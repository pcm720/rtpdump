package codecs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/david-biro/rtpdump/log"
	"github.com/david-biro/rtpdump/rtp"
)

const AMR_NB_MAGIC string = "#!AMR\n"
const AMR_WB_MAGIC string = "#!AMR-WB\n"

var AMR_NB_FRAME_SIZE []int = []int{12, 13, 15, 17, 19, 20, 26, 31, 5, 0, 0, 0, 0, 0, 0, 0}
var AMR_WB_FRAME_SIZE []int = []int{17, 23, 32, 36, 40, 46, 50, 58, 60, 5, 5, 0, 0, 0, 0, 0}

const AMR_NB_SAMPLE_RATE = 8000
const AMR_WB_SAMPLE_RATE = 16000

type Amr struct {
	started       bool
	configured    bool
	sampleRate    int
	octetAligned  bool
	alignmentSet  bool
	sampleRateSet bool
	timestamp     uint32

	lastSeq uint16
}

func NewAmr() Codec {
	return &Amr{started: false, configured: false, timestamp: 0}
}

func (amr *Amr) Init() {
}

func (amr *Amr) Reset() {
	amr.started = false
	amr.alignmentSet = false
	amr.sampleRateSet = false
	amr.sampleRate = 0
	amr.timestamp = 0
	amr.lastSeq = 0
}

func (amr *Amr) isWideBand() bool {
	return amr.sampleRate == AMR_WB_SAMPLE_RATE
}

func (amr Amr) GetFormatMagic() ([]byte, error) {
	switch amr.sampleRate {
	case AMR_WB_SAMPLE_RATE:
		return []byte(AMR_WB_MAGIC), nil
	case AMR_NB_SAMPLE_RATE:
		return []byte(AMR_NB_MAGIC), nil
	default:
		return nil, amr.invalidState()
	}
}

func (amr *Amr) invalidState() error {
	return errors.New("invalid state")
}

func (amr *Amr) SetOptions(options map[string]string) error {
	if amr.started {
		return amr.invalidState()
	}

	switch options["octet-aligned"] {
	case "0":
		amr.octetAligned, amr.alignmentSet = false, true
	case "1":
		amr.octetAligned, amr.alignmentSet = true, true
	default:
	}

	switch options["sample-rate"] {
	case "nb":
		amr.sampleRate, amr.sampleRateSet = AMR_NB_SAMPLE_RATE, true
	case "wb":
		amr.sampleRate, amr.sampleRateSet = AMR_WB_SAMPLE_RATE, true
	}

	amr.configured = true
	return nil
}

func (amr *Amr) HandleRtpPacket(packet *rtp.RtpPacket) (result []byte, err error) {
	if !amr.configured {
		return nil, amr.invalidState()
	}

	log.Sdebug("decoding packet with sequence number %d", packet.SequenceNumber)
	// detect sequence number wrap-around and treat it as stream continuation while accounting for possible packet losses (±100 packets)
	if !((amr.lastSeq > 65435) && (packet.SequenceNumber < 100)) && (packet.SequenceNumber <= amr.lastSeq) {
		return nil, errors.New("ignore out of sequence")
	}

	if !amr.alignmentSet || !amr.sampleRateSet {
		if err := amr.detectParameters(packet); err != nil {
			return nil, err
		}
	}

	result = append(result, amr.handleMissingSamples(packet.Timestamp)...)

	var speechFrame []byte
	if amr.octetAligned {
		speechFrame, err = amr.handleOaMode(packet.Timestamp, packet.Payload)
	} else {
		speechFrame, err = amr.handleBeMode(packet.Timestamp, packet.Payload)
	}

	if err != nil {
		return nil, err
	}

	result = append(result, speechFrame...)
	return result, nil
}

func (amr *Amr) detectParameters(packet *rtp.RtpPacket) error {
	if len(packet.Payload) == 2 { // can't detect codec parameters using packet that contains only the AMR header, probably a "no data" packet
		return errors.New("payload is too short")
	}

	oaFrameType := (packet.Payload[1] & 0x78) >> 3
	beFrameType := (packet.Payload[0]&0x07)<<1 | (packet.Payload[1]&0x80)>>7
	log.Sdebug(fmt.Sprintf("expected OA type %d NB frame size: %d, WB: %d, actual: %d", oaFrameType, AMR_NB_FRAME_SIZE[oaFrameType], AMR_WB_FRAME_SIZE[oaFrameType], len(packet.Payload)))
	log.Sdebug(fmt.Sprintf("expected BE type %d NB frame size: %d, WB: %d, actual: %d", beFrameType, AMR_NB_FRAME_SIZE[beFrameType], AMR_WB_FRAME_SIZE[beFrameType], len(packet.Payload)))

	checkFrameSize := func(frameSize int) bool {
		if frameSize > 0 {
			if d := (len(packet.Payload) - frameSize); d >= 0 && d < 3 {
				return true
			}
		}
		return false
	}
	setAMRParameters := func(octetAligned bool, sampleRate int) {
		amr.alignmentSet, amr.sampleRateSet = true, true
		amr.octetAligned, amr.sampleRate = octetAligned, sampleRate
	}

	if (packet.Payload[0] << 4) == 0x0 { // check for octet-aligned only if the first byte has reserved bits after 4-bit CMR
		if checkFrameSize(AMR_NB_FRAME_SIZE[oaFrameType]) {
			log.Info("detected AMR-NB octet-aligned frame")
			setAMRParameters(true, AMR_NB_SAMPLE_RATE)
			return nil
		}
		if checkFrameSize(AMR_WB_FRAME_SIZE[oaFrameType]) {
			log.Info("detected AMR-WB octet-aligned frame")
			setAMRParameters(true, AMR_WB_SAMPLE_RATE)
			return nil
		}
	}

	if checkFrameSize(AMR_NB_FRAME_SIZE[beFrameType]) {
		log.Info("detected AMR-NB bandwidth-efficient frame")
		setAMRParameters(false, AMR_NB_SAMPLE_RATE)
		return nil
	}
	if checkFrameSize(AMR_WB_FRAME_SIZE[beFrameType]) {
		log.Info("detected AMR-WB bandwidth-efficient frame")
		setAMRParameters(false, AMR_WB_SAMPLE_RATE)
		return nil
	}
	return errors.New("unable to detect codec parameters")
}

func (amr *Amr) handleMissingSamples(timestamp uint32) (result []byte) {
	if amr.timestamp != 0 {
		lostSamplesFromPrevious := ((timestamp - amr.timestamp) / (uint32(amr.sampleRate) / 50)) - 1
		log.Sdebug("lostSamplesFromPrevious: %d, time: %d", lostSamplesFromPrevious, lostSamplesFromPrevious*20)
		if lostSamplesFromPrevious == math.MaxUint32 { // something caused a wrap around
			return result
		}
		for i := lostSamplesFromPrevious; i > 0; i-- {
			if amr.isWideBand() {
				result = append(result, 0xFC)
			} else {
				result = append(result, 0x7C)
			}
		}
	}
	return result
}

func (amr *Amr) getSpeechFrameByteSize(frameType int) (size int) {
	if amr.isWideBand() {
		return AMR_WB_FRAME_SIZE[frameType]
	}
	return AMR_NB_FRAME_SIZE[frameType]
}

func (amr *Amr) handleOaMode(timestamp uint32, payload []byte) ([]byte, error) {
	var result []byte
	var currentTimestamp uint32

	frame := 0
	rtpFrameHeader := payload[0:]
	// payload header := [CMR(4bit)[R(4bit)][ILL(4bit)(opt)][ILP(4bit)(opt)]
	// TOC := [F][FT(4bit)][Q][P][P]
	// storage := [0][FT(4bit)][Q][0][0]
	cmr := (rtpFrameHeader[0] & 0xF0) >> 4

	if len(rtpFrameHeader) == 4 { // RFC 2833 RTP Event has a length of 4 bytes
		printRTPEvent(rtpFrameHeader)
		return nil, nil
	}

	isLastFrame := (rtpFrameHeader[1]&0x80)&0x80 == 0x00
	frameType := (rtpFrameHeader[1] & 0x78) >> 3
	quality := (rtpFrameHeader[1]&0x04)&0x04 == 0x04

	log.Sdebug("octet-aligned, lastFrame:%t, cmr:%d, frameType:%d, quality:%t",
		isLastFrame, cmr, frameType, quality)

	speechFrameHeader := frameType << 3
	speechFrameHeader = speechFrameHeader | (rtpFrameHeader[1] & 0x04)

	speechFrameSize := amr.getSpeechFrameByteSize(int(frameType))

	currentTimestamp = timestamp + uint32((amr.sampleRate/50)*frame)

	if !isLastFrame {
		log.Warn("Amr does not suport more than one frame per payload - discarted")
		return nil, errors.New("Amr does not suport more than one frame per payload")
	}

	result = append(result, speechFrameHeader)

	if speechFrameSize != 0 {
		speechPayload := rtpFrameHeader[2 : 2+speechFrameSize]
		result = append(result, speechPayload...)
	}
	amr.timestamp = currentTimestamp
	return result, nil
}

func (amr *Amr) handleBeMode(timestamp uint32, payload []byte) ([]byte, error) {
	var result []byte
	var currentTimestamp uint32

	frame := 0
	rtpFrameHeader := payload[0:]
	// packing frame with TOC: frame type and quality bit
	// RTP=[CMR(4bit)[F][FT(4bit)][Q][..speechFrame]] -> storage=[0][FT(4bit)][Q][0][0]
	cmr := (rtpFrameHeader[0] & 0xF0) >> 4

	if len(rtpFrameHeader) == 4 { // RFC 2833 RTP Event has a length of 4 bytes
		printRTPEvent(rtpFrameHeader)
		return nil, nil
	}

	isLastFrame := (rtpFrameHeader[0]&0x08)>>3 == 0x00
	frameType := (rtpFrameHeader[0]&0x07)<<1 | (rtpFrameHeader[1]&0x80)>>7
	quality := (rtpFrameHeader[1] & 0x40) == 0x40

	log.Sdebug("bandwidth-efficient, lastFrame:%t, cmr:%d, frameType:%d, quality:%t",
		isLastFrame, cmr, frameType, quality)

	speechFrameHeader := (rtpFrameHeader[0]&0x07)<<4 | (rtpFrameHeader[1]&0x80)>>4
	speechFrameHeader = speechFrameHeader | (rtpFrameHeader[1]&0x40)>>4

	speechFrameSize := amr.getSpeechFrameByteSize(int(frameType))

	currentTimestamp = timestamp + uint32((amr.sampleRate/50)*frame)

	if !isLastFrame {
		log.Warn("Amr does not suport more than one frame per payload - discarted")
		return nil, errors.New("Amr does not suport more than one frame per payload")
	}

	result = append(result, speechFrameHeader)

	if speechFrameSize != 0 {
		speechPayload := rtpFrameHeader[1:]
		speechFrame := make([]byte, speechFrameSize)
		// shift 2 bits left in speechFrame
		for k := 0; k < speechFrameSize; k++ {
			speechFrame[k] = (speechPayload[k] & 0x3F) << 2
			if k+1 < speechFrameSize {
				speechFrame[k] = speechFrame[k] | (speechPayload[k+1]&0xC0)>>6
			}
		}
		result = append(result, speechFrame...)
	}
	amr.timestamp = currentTimestamp
	return result, nil
}

func printRTPEvent(frameHeader []byte) {
	rtpEvent := frameHeader[0]
	endOfEvent := (frameHeader[1] & 0x80) != 0
	volume := (frameHeader[1] & 0x3F)
	eventDuration := binary.BigEndian.Uint16([]byte{frameHeader[2], frameHeader[3]})
	log.Info(fmt.Sprintf("possible RTP event: 0x%0x, end of event: %t, volume: %d, duration: %d", rtpEvent, endOfEvent, volume, eventDuration))
}

var AmrMetadata = CodecMetadata{
	Name:     "amr",
	LongName: "Adaptive Multi Rate",
	Options: []CodecOption{
		amrSampleRateOption,
		amrOctetAlignedOption,
	},
	Init: NewAmr,
}

var amrOctetAlignedOption = CodecOption{
	Required:         true,
	Name:             "octet-aligned",
	Description:      "whether this payload is octet-aligned or bandwidth-efficient",
	ValidValues:      []string{"0", "1", "auto"},
	ValueDescription: []string{"bandwidth-efficient", "octet-aligned", "auto"},
	RestrictValues:   true,
}

var amrSampleRateOption = CodecOption{
	Required:         true,
	Name:             "sample-rate",
	Description:      "whether this payload is narrow or wide band",
	ValidValues:      []string{"nb", "wb", "auto"},
	ValueDescription: []string{"Narrow Band (8000)", "Wide Band (16000)", "Detect automatically"},
	RestrictValues:   true,
}
