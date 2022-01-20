package codecs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/david-biro/rtpdump/rtp"
)

//  0 = EVS, 1 = EVS AMR-WB IO, 2 = special case (Table A.1 in 3GPP TS 26.445)
var EVS_IO_MODES []int = []int{0, 2, 1, 0, 0, 1, 0, 1, 0, 1, 1, 0, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0}

// payload size is in bytes, only for Compact Header!
var EVS_PAYLOAD_SIZES []int = []int{6, 7, 17, 18, 20, 23, 24, 32, 33, 36, 40, 41, 46, 50, 58, 60, 61, 80, 120, 160, 240, 320}

const EVS_MAGIC_1 string = "#!EVS_MC1.0\n"
const EVS_MAGIC_2 string = "\x00\x00\x00\x01"

var evs_payload_sizes = []int{6, 7, 17, 18, 20, 23, 24, 32, 33, 36, 40, 41, 46, 50, 58, 60, 61, 80, 120, 160, 240, 320}
var toc = []byte{0x0C, 0x00, 0, 0x01, 0x02, 1, 0x03, 0x32, 0x04, 3, 4, 0x05, 5, 6, 7, 8, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B}

type Evs struct {
	started    bool
	configured bool // for menu options
	FullHeader bool
	timestamp  uint32
}

func NewEvs() Codec {
	return &Evs{configured: false, FullHeader: false, timestamp: 0}
}

func (evs *Evs) Init() {
}

func (evs *Evs) Reset() {
	evs.started = false
	evs.timestamp = 0
}

func (evs Evs) GetFormatMagic() ([]byte, error) {
	return append([]byte(EVS_MAGIC_1), []byte(EVS_MAGIC_2)...), nil
}

func (evs *Evs) invalidState() error {
	return errors.New("invalid state")
}

func (evs *Evs) SetOptions(options map[string]string) error {
	if evs.started {
		return evs.invalidState()
	}

	v, ok := options["header-format"]
	if !ok {
		return errors.New("required option not present")
	}

	evs.FullHeader = v == "1"

	evs.configured = true
	return nil
}

//check whether the payload is AMRWB IO mode or not-> 1: AMRWBIO, 0: EVS PRIMARY, -1: SPECIAL CASE
func IsIoMode(slice []int, payloadsize int) int {
	for i := range slice {
		if slice[i] == payloadsize {
			return EVS_IO_MODES[i]
		}
	}
	return -1
}

func getIndex(slice []int, item int) int {
	for i := range slice {
		if slice[i] == item {
			return i
		}
	}
	return -1
}

func realignOctet(octet []byte) []byte {
	for index, _ := range octet {
		if index != 0 {
			octet[index-1] |= (octet[index] >> 5)
		}
		octet[index] = (octet[index] << 3)
	}
	return octet
}

//https://play.golang.org/p/CldAWVp2PVA
func realignOctet2(octet []byte) []byte {
	var lastbyte byte = octet[len(octet)-1]
	var lastbit = lastbyte & 1
	octet[len(octet)-1] &= 0xFE

	for index, _ := range octet {
		if index != 0 {
			octet[index-1] |= (octet[index] >> 5)
		}
		octet[index] = (octet[index] << 3)
	}
	if lastbit == 1 {
		octet[0] &= 0x7F
		octet[0] |= 0x80
	}
	if lastbit == 0 {
		octet[0] &= 0x7F
	}
	return octet
}

//section A.2.1 of 3GPP TS26.445 and section A.2.2 of 3GPP TS26.445
//Iu Framing is not supported for the EVS codec
func (evs *Evs) HandleRtpPacket(packet *rtp.RtpPacket) (result []byte, err error) {

	fmt.Println("HandleRtpPacket func started.. ")

	if !evs.configured {
		return nil, evs.invalidState()
	}

	if !evs.FullHeader {
		fmt.Println(evs.FullHeader)
		switch iomode := IsIoMode(evs_payload_sizes, len(packet.Payload)); iomode {
		case 0: // EVS Primary
			fmt.Println("EVS")
			result = append(result, toc[getIndex(evs_payload_sizes, len(packet.Payload))])
			result = append(result, packet.Payload...)

			//Debug
			//fmt.Println("payload length: ", len(packet.Payload))

		case 1: // EVS-AMRWB IO
			fmt.Println("EVS AMR-WB IO")
			fmt.Println("payload length: ", len(packet.Payload))

			buf := new(bytes.Buffer)

			// might be changed to this: https://pkg.go.dev/github.com/go-restruct/restruct#Unpack

			_ = binary.Write(buf, binary.BigEndian, toc[getIndex(evs_payload_sizes, len(packet.Payload))])

			result = append(result, buf.Bytes()...)
			result = append(result, realignOctet2(packet.Payload)...)

			//Debug
			//fmt.Println("eredeti:", (packet.Payload))
			//fmt.Println("payload length: ", len(packet.Payload))

			const AMR_WB_MAGIC string = "#!AMR-WB\n"
			//fmt.Println("dumped to file: ")

			for _, s := range result {
				fmt.Printf("%x", s)
				fmt.Print(" ")
			}
			fmt.Println()

		case 2: //Ambigous case
			fmt.Println("Ambigous case")
			bit0 := (packet.Payload[0] & 0xff) >> 7
			if bit0 == 0 { //EVS Primary 2.8kbps
				fmt.Println("EVS Primary 2.8kbps")
				fmt.Println("payload length: ", len(packet.Payload))
				result = append(result, toc[1])
				result = append(result, packet.Payload...)
			} else { //EVS AMRWB IO SID
				fmt.Println("EVS AMRWB IO SID Silence Insertion Descriptor in header-full with one CMR byte")

				buf := new(bytes.Buffer)

				result = append(result, 0x39)
				_ = binary.Write(buf, binary.BigEndian, packet.Payload[2:])
				result = append(result, buf.Bytes()...)

				for _, s := range result {
					fmt.Printf("%x", s)
					fmt.Print(" ")
				}
				fmt.Println()
			}
		default:
			fmt.Println("other payload size found than EVS or it is in Header-Full Format")
		}
	} else { // Header-Full Format, ToC + Single Speech Frame
		bit0 := (packet.Payload[0] & 0xff) >> 7 //CMR?  >0<|1|2|3|4|5|6|7|
		bit1 := (packet.Payload[0] & 0x40) >> 6 //next speech frame?  0|>1<|2|3|4|5|6|7|
		buf := new(bytes.Buffer)
		if bit0 == 1 { //CMR
			fmt.Println("CMR+ToC single or multiframe, not supported yet")
			//if nextbyte.bit1 == 1 { //more ToC, more speech frame
			//} else {
			//}
		} else { //ToC
			if bit1 == 0 { // no more speechframe (ToC+singleframe)
				fmt.Println("ToC+single frame")
				result = append(result, packet.Payload[0]&0xf)
				_ = binary.Write(buf, binary.BigEndian, packet.Payload[1:])
				result = append(result, buf.Bytes()...)
			}
		}
	}

	// result (evs-mime) can be decoded using 3GPP 26.443 EVS_dec binary
	return result, nil
}

// Options
var EvsMetadata = CodecMetadata{
	Name:     "evs",
	LongName: "Enhanced Voice Services",
	Options: []CodecOption{
		EvsHeaderFormatOption,
	},
	Init: NewEvs,
}

var EvsHeaderFormatOption = CodecOption{
	Required:         true,
	Name:             "header-format",
	Description:      "whether the RTP payload consists of exactly one coded frame (Compact Format) or the payload consists of one or more coded frame(s) with EVS RTP payload header(s)",
	ValidValues:      []string{"0", "1"},
	ValueDescription: []string{"Compact Format", "Header-Full Format (only single ToC + single frame supported yet)"},
	RestrictValues:   true,
}
