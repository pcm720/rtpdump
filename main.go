package main

import (
	"fmt"
	"os"

	"github.com/david-biro/rtpdump/esp"
	"github.com/david-biro/rtpdump/log"
	"github.com/david-biro/rtpdump/rtp"
	"github.com/urfave/cli"
)

func loadKeyFile(c *cli.Context) error {
	return esp.LoadKeyFile(c.GlobalString("key-file"))
}

func main() {
	log.SetLevel(log.INFO)

	app := cli.NewApp()
	app.Name = "rtpdump"
	app.Version = "0.9.1"
	cli.AppHelpTemplate += `
     /\_/\
    ( o.o )
     > ^ <
    `

	app.Commands = []cli.Command{
		{
			Name:      "streams",
			Aliases:   []string{"s"},
			Usage:     "display rtp streams in pcap file",
			ArgsUsage: "[pcap-file]",
			Action:    streamsCmd,
		},
		{
			Name:      "interactive-dump",
			Aliases:   []string{"id"},
			Usage:     "dumps rtp payload to file",
			ArgsUsage: "[pcap-file]",
			Action:    interactiveDumpCmd,
		},
		{
			Name:      "dump",
			Aliases:   []string{"d"},
			Usage:     "dumps rtp payload to file using parameters from arguments",
			ArgsUsage: "[pcap-file]",
			Action:    dumpCmd,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "codec, c",
					Value: "amr",
					Usage: "Codec to use for stream decoding",
				},
				cli.StringFlag{
					Name:  "flags, f",
					Value: "sample-rate:nb,octet-aligned:0",
					Usage: "Codec options in \"option:value\" format, separated by comma",
				},
				cli.StringFlag{
					Name:  "output, o",
					Value: "out.amr",
					Usage: "Output filename",
				},
				cli.IntFlag{
					Name:  "stream, s",
					Value: -1,
					Usage: "Stream index to decode. By default dumps all streams using output filename as a base name",
				},
			},
		},
		{
			Name:      "play",
			Aliases:   []string{"p"},
			Usage:     "replays the selected rtp stream ;)",
			ArgsUsage: "[pcap-file]",
			Action:    playCmd,
			Flags: []cli.Flag{
				cli.StringFlag{Name: "host", Value: "localhost", Usage: "destination host for replayed RTP packets"},
				cli.IntFlag{Name: "port", Value: 1234, Usage: "destination port for replayed RTP packets"},
			},
		},
		{
			Name:    "codecs",
			Aliases: []string{"c"},
			Usage:   "lists supported codecs information",
			Subcommands: cli.Commands{
				cli.Command{
					Name:      "list",
					Action:    codecsList,
					ArgsUsage: "[codec name or empty for all]",
				},
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "key-file, k",
			Value: "esp-keys.txt",
			Usage: "Load ipsec keys from `FILE`",
		},
	}

	app.Run(os.Args)
}

var streamsCmd = func(c *cli.Context) error {
	loadKeyFile(c)

	inputFile := c.Args().First()

	if len(c.Args()) <= 0 {
		cli.ShowCommandHelp(c, "streams")
		return cli.NewExitError("wrong usage for streams", 1)
	}

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

	for _, v := range rtpStreams {
		fmt.Printf("%s\n", v)
	}

	return nil
}
