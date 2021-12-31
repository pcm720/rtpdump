package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/david-biro/rtpdump/codecs"
	"github.com/david-biro/rtpdump/log"
	"github.com/david-biro/rtpdump/rtp"
	"github.com/urfave/cli"
)

type dumpOptions struct {
	codecMetadata codecs.CodecMetadata
	rtpStreams    []*rtp.RtpStream
	streamIndex   int
	options       map[string]string
	outputFile    string
}

var dumpCmd = func(c *cli.Context) error {
	loadKeyFile(c)

	inputFile := c.Args().First()
	if inputFile == "" {
		cli.ShowCommandHelp(c, "dump")
		return cli.NewExitError("wrong usage for dump", 1)
	}

	// find codec in codecs.CodecList and get CodecMetadata
	codecIndex := -1
	codecName := c.String("codec")
	for index, codec := range codecs.CodecList {
		if codec.Name == codecName {
			codecIndex = index
			break
		}
	}
	if codecIndex == -1 {
		return cli.NewExitError("invalid codec name, see available codecs using \"codecs list\" command", 1)
	}
	codecMetadata := codecs.CodecList[codecIndex]

	// create a map of codec options
	codecOptions := make(map[string][]string)
	for _, availableOptions := range codecMetadata.Options {
		codecOptions[availableOptions.Name] = availableOptions.ValidValues
	}

	// parse codec options
	flags := c.String("flags")
	options := strings.Split(flags, ",")
	optionsMap := make(map[string]string, len(options))
	for _, option := range options {
		values := strings.Split(option, ":")
		if len(values) != 2 {
			cli.ShowCommandHelp(c, "dump")
			return cli.NewExitError("invalid flag value", 1)
		}
		if validValues, ok := codecOptions[values[0]]; ok {
			for _, validValue := range validValues { // validate option value
				if validValue == values[1] {
					optionsMap[values[0]] = values[1]
					goto next
				}
			}
			return cli.NewExitError("invalid value '"+values[1]+"' for option '"+values[0]+"', valid values: ["+strings.Join(validValues, ", ")+"]", 1)
		}
		return cli.NewExitError("unknown option '"+values[0]+"', see available options and valid values using \"codecs list\" command", 1)
	next:
	}

	// get and validate stream index
	streamIndex := c.Int("stream")
	if streamIndex < 1 && streamIndex != -1 {
		return cli.NewExitError("invalid stream index", 1)
	}

	// read RTP packets and find streams
	rtpReader, err := rtp.NewRtpReader(inputFile)
	if err != nil {
		return cli.NewMultiError(cli.NewExitError("failed to open file", 1), err)
	}
	defer rtpReader.Close()

	rtpStreams := rtpReader.GetStreams()
	if len(rtpStreams) <= 0 {
		fmt.Println("no streams found")
		return nil
	}
	if streamIndex > len(rtpStreams) {
		return cli.NewExitError("stream with specified index doesn't exist", 1)
	}

	return doDump(dumpOptions{
		codecMetadata: codecMetadata,
		rtpStreams:    rtpStreams,
		streamIndex:   streamIndex,
		options:       optionsMap,
		outputFile:    c.String("output"),
	})
}

func doDump(options dumpOptions) error {
	codec := options.codecMetadata.Init()

	err := codec.SetOptions(options.options)
	if err != nil {
		return err
	}

	codec.Init()

	dumpStream := func(fileName string, stream *rtp.RtpStream) (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("%s", p)
			}
		}()

		f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0655)
		if err != nil {
			return cli.NewMultiError(cli.NewExitError("failed to create file", 1), err)
		}
		defer f.Close()

		f.Write(codec.GetFormatMagic())
		for _, r := range stream.RtpPackets {
			if frames, err := codec.HandleRtpPacket(r); err == nil {
				f.Write(frames)
			}
		}
		f.Sync()
		return nil
	}

	if options.streamIndex != -1 { // dump single stream
		if err = dumpStream(options.outputFile, options.rtpStreams[options.streamIndex-1]); err != nil {
			os.Remove(options.outputFile)
			return cli.NewExitError(fmt.Sprintf("failed to decode stream: %s", err), 1)
		}
		return nil
	}

	extension := filepath.Ext(options.outputFile) // dump all streams
	baseName := options.outputFile[:len(options.outputFile)-len(extension)] + "_s"
	for streamIndex, stream := range options.rtpStreams {
		fileName := baseName + strconv.Itoa(streamIndex+1) + extension
		err := dumpStream(fileName, stream)
		if err != nil {
			log.Error("failed to decode stream " + strconv.Itoa(streamIndex+1) + ": " + err.Error())
			os.Remove(fileName)
		}
	}
	return nil

}
