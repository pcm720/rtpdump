package rtp

import (
	"fmt"
	"time"

	"github.com/david-biro/rtpdump/log"
	"github.com/david-biro/rtpdump/util"
)

type RtpStream struct {

	// Public
	Ssrc               uint32
	PayloadType        int
	SrcIP, DstIP       string
	SrcPort, DstPort   uint
	StartTime, EndTime time.Time

	// Internal - improve
	FirstTimestamp uint32
	FirstSeq       uint16
	Cycle          uint
	CurSeq         uint16

	// Calculated
	TotalExpectedPackets uint
	LostPackets          uint
	MeanJitter           float32
	MeanBandwidth        float32

	RtpPackets []*RtpPacket
}

func (r RtpStream) String() string {
	return fmt.Sprintf("%s - %s   0x%08X   %3d   %5d   %s:%d -> %s:%d",
		util.TimeToStr(r.StartTime),
		util.TimeToStr(r.EndTime),
		r.Ssrc,
		r.PayloadType,
		len(r.RtpPackets),
		r.SrcIP,
		r.SrcPort,
		r.DstIP,
		r.DstPort,
	)
}

func (r *RtpStream) AddPacket(rtp *RtpPacket) {
	var lostPackets int32 = 0

	// skip calculating number of lost packets if this is the first packet of the stream
	if r.TotalExpectedPackets != 0 {
		lostPackets = int32(rtp.SequenceNumber) - int32(r.CurSeq) - 1 // account for the difference between last and current packet
	}

	if lostPackets < 0 { // if number of lost packets is negative,
		// detect sequence number wrap-around and treat it as stream continuation while accounting for possible packet losses (±100 packets)
		if !((r.CurSeq > 65435) && (rtp.SequenceNumber < 100)) { // else, ignore out-of-sequence packets
			return
		}

		// handle sequence number overflow and lost packets by
		// adding number of packets lost before wrap-around and number of packets lost after wrap-around
		lostPackets = int32(0xFFFF-r.CurSeq) + int32(rtp.SequenceNumber)
		log.Sinfo("sequence number wrap-around detected, lost %d packets", lostPackets)
	}
	if lostPackets != 0 {
		log.Sdebug("%d packets lost between packets %d and %d", lostPackets, r.CurSeq, rtp.SequenceNumber)
	}

	r.EndTime = rtp.ReceivedAt
	r.CurSeq = rtp.SequenceNumber
	r.TotalExpectedPackets += 1 + uint(lostPackets) // count current packet and all lost packets
	r.LostPackets += uint(lostPackets)

	r.RtpPackets = append(r.RtpPackets, rtp)
}
