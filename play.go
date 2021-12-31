package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/david-biro/rtpdump/console"
	"github.com/david-biro/rtpdump/rtp"
	"github.com/urfave/cli"
)

var playCmd = func(c *cli.Context) error {

	loadKeyFile(c)

	inputFile := c.Args().First()

	if inputFile == "" {
		cli.ShowCommandHelp(c, "play")
		return cli.NewExitError("wrong usage for play", 1)
	}

	host := c.String("host")
	port := c.Int("port")

	rtpReader, err := rtp.NewRtpReader(inputFile)

	if err != nil {
		return cli.NewMultiError(cli.NewExitError("failed to open file", 1), err)
	}

	defer rtpReader.Close()

	rtpStreams := rtpReader.GetStreams()

	if len(rtpStreams) <= 0 {
		fmt.Println("No streams found")
		return nil
	}

	var rtpStreamsOptions []string
	for _, v := range rtpStreams {
		rtpStreamsOptions = append(rtpStreamsOptions, v.String())
	}

	streamIndex, err := console.ExpectIntRange(
		0,
		len(rtpStreams),
		console.ListPrompt("Choose RTP Stream", rtpStreamsOptions...))

	if err != nil {
		return cli.NewMultiError(cli.NewExitError("invalid input", 1), err)
	}
	if streamIndex == 0 {
		fmt.Print("Playing all streams\n\n")
		// Locate the start time of first stream
		var baseTime time.Time = rtpStreams[0].StartTime
		for _, v := range rtpStreams {
			if v.StartTime.Before(baseTime) {
				baseTime = v.StartTime
			}
		}
		var waitGroup sync.WaitGroup
		for i, stream := range rtpStreams {
			// Compute delay start w/respect to start of initial stream
			first := stream.RtpPackets[0]
			delay := first.ReceivedAt.Sub(baseTime)
			waitGroup.Add(1)
			go playStream(&waitGroup, i+1, stream, host, port+(2*i), delay)
		}
		// wait for all streams players to complete
		waitGroup.Wait()
		fmt.Printf("All streams completed\n\n")
	} else {
		stream := rtpStreams[streamIndex-1]
		return playStream(nil, streamIndex, stream, host, port, 0)
	}

	return nil
}

func playStream(wg *sync.WaitGroup, streamIndex int, stream *rtp.RtpStream, host string, port int, delay time.Duration) error {

	// if run as part of a waitgroup, notify done at the completion
	if wg != nil {
		defer wg.Done()
	}

	fmt.Printf("(%-3d) %s -> Streaming to: %s:%d\n", streamIndex, stream, host, port)

	RemoteAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
	conn, err := net.DialUDP("udp", nil, RemoteAddr)
	defer conn.Close()
	if err != nil {
		fmt.Printf("Some error: %v\n", err)
		return err
	}

	if delay > 0 {
		fmt.Printf("(%-3d) Delaying of (%d) ns\n", streamIndex, delay.Nanoseconds())
		time.Sleep(delay)
	}

	len := len(stream.RtpPackets)
	for i, v := range stream.RtpPackets {
		fmt.Printf("(%-3d) ", streamIndex)
		fmt.Println(v)
		conn.Write(v.Data)

		if i < len-1 {
			next := stream.RtpPackets[i+1]
			wait := next.ReceivedAt.Sub(v.ReceivedAt)
			time.Sleep(wait)
		}
	}

	fmt.Printf("(%-3d) Completed\n", streamIndex)
	return nil
}
