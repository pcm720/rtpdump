package main

import (
	"fmt"
	"os"

	"github.com/david-biro/rtpdump/codecs"
	"github.com/david-biro/rtpdump/console"
	"github.com/david-biro/rtpdump/rtp"
	"github.com/urfave/cli"
)

var interactiveDumpCmd = func(c *cli.Context) error {
	loadKeyFile(c)

	inputFile := c.Args().First()

	if inputFile == "" {
		cli.ShowCommandHelp(c, "dump")
		return cli.NewExitError("wrong usage for dump", 1)
	}

	rtpReader, err := rtp.NewRtpReader(inputFile)

	if err != nil {
		return cli.NewMultiError(cli.NewExitError("failed to open file", 1), err)
	}

	defer rtpReader.Close()

	return doInteractiveDump(c, rtpReader)
}

//var rtpnum = 0
func doInteractiveDump(c *cli.Context, rtpReader *rtp.RtpReader) error {
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
		1,
		len(rtpStreams),
		console.ListPrompt("Choose RTP Stream", rtpStreamsOptions...))

	if err != nil {
		return cli.NewMultiError(cli.NewExitError("invalid input", 1), err)
	}
	fmt.Printf("(%-3d) %s\n\n", streamIndex, rtpStreams[streamIndex-1])

	var codecList []string
	for _, v := range codecs.CodecList {
		codecList = append(codecList, v.Name)
	}
	codecIndex, err := console.ExpectIntRange(
		1,
		len(codecs.CodecList),
		console.ListPrompt("Choose codec:", codecList...))

	if err != nil {
		return cli.NewMultiError(cli.NewExitError("invalid input", 1), err)
	}
	fmt.Printf("(%-3d) %s\n\n", codecIndex, codecs.CodecList[codecIndex-1].Name)

	codecMetadata := codecs.CodecList[codecIndex-1]

	optionsMap := make(map[string]string)
	for _, v := range codecMetadata.Options {
		var optionValue string
		if v.RestrictValues {
			optionValue, err = console.ExpectRestrictedString(
				v.ValidValues,
				console.KeyValuePrompt(fmt.Sprintf("%s - %s", v.Name, v.Description),
					v.ValidValues, v.ValueDescription))
		} else {
			optionValue, err = console.ExpectAnyString(
				console.Prompt(fmt.Sprintf("%s - %s: ", v.Name, v.Description)))
		}

		if err != nil {
			return cli.NewMultiError(cli.NewExitError("invalid input", 1), err)
		}
		optionsMap[v.Name] = optionValue
	}

	outputFile, err := console.ExpectAnyString(console.Prompt("Output file: "))

	if err != nil {
		return cli.NewMultiError(cli.NewExitError("invalid input", 1), err)
	}

	fmt.Printf("%s\n", outputFile)

	codec := codecMetadata.Init()
	err = codec.SetOptions(optionsMap)
	//fmt.Println(optionsMap)

	if err != nil {
		return err
	}

	codec.Init()

	f, err := os.Create(outputFile)
	if err != nil {
		return cli.NewMultiError(cli.NewExitError("failed to create output file", 1), err)
	}
	defer f.Close()

	magic, err := codec.GetFormatMagic()
	if err != nil {
		return cli.NewMultiError(cli.NewExitError("failed to get format magic", 1), err)
	}
	f.Write(magic)
	for _, r := range rtpStreams[streamIndex-1].RtpPackets {
		frames, err := codec.HandleRtpPacket(r)
		if err == nil {
			f.Write(frames)
			// rtpnum = rtpnum + 1
			// fmt.Println(rtpnum)
		}
	}
	f.Sync()

	return nil
}

func codecsList(c *cli.Context) error {
	codec := c.Args().First()
	found := codec == ""

	for _, v := range codecs.CodecList {
		if found || codec == v.Name {
			fmt.Printf("%s\n", v.Describe())
			found = true
		}
	}

	if !found {
		fmt.Printf("Codec %s not available\n", codec)
	}
	return nil
}
