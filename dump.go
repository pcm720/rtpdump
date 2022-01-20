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
	if err := codec.SetOptions(options.options); err != nil {
		return err
	}
	codec.Init()

	if options.streamIndex != -1 { // dump single stream
		if err := dumpStream(codec, options.outputFile, options.rtpStreams[options.streamIndex-1]); err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to decode stream: %s", err), 1)
		}
		return nil
	}

	extension := filepath.Ext(options.outputFile) // dump all streams
	baseName := options.outputFile[:len(options.outputFile)-len(extension)] + "_s"
	log.Info(fmt.Sprintf("dumping %d streams", len(options.rtpStreams)))
	for streamIndex, stream := range options.rtpStreams {
		log.Info(fmt.Sprintf("dumping %d", streamIndex+1))
		fileName := baseName + strconv.Itoa(streamIndex+1) + extension

		if err := dumpStream(codec, fileName, stream); err != nil {
			log.Error("failed to decode stream " + strconv.Itoa(streamIndex+1) + ": " + err.Error())
		}
	}
	return nil

}

func dumpStream(codec codecs.Codec, fileName string, stream *rtp.RtpStream) (err error) {
	defer func() {
		codec.Reset()
		if p := recover(); p != nil {
			err = fmt.Errorf("%s", p)
		}
		if err != nil {
			os.Remove(fileName)
		}
	}()

	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0655)
	if err != nil {
		return cli.NewMultiError(cli.NewExitError("failed to create file", 1), err)
	}
	defer f.Close()

	gotFormatMagic := false
	if magic, err := codec.GetFormatMagic(); err == nil {
		gotFormatMagic = true
		f.Write(magic)
	}
	for _, r := range stream.RtpPackets {
		frames, err := codec.HandleRtpPacket(r)
		if err != nil {
			if (err.Error() == "ignore out of sequence") || (err.Error() == "payload is too short") {
				continue
			}
			return cli.NewMultiError(cli.NewExitError("failed to handle RTP packet", 1), err)
		}

		if !gotFormatMagic {
			magic, err := codec.GetFormatMagic()
			if err != nil {
				return cli.NewMultiError(cli.NewExitError("failed to get format magic", 1), err)
			}
			f.Write(magic)
			gotFormatMagic = true
		}
		f.Write(frames)
	}
	f.Sync()
	return nil
}
