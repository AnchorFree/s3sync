package main

import (
	"fmt"
	"os"
	// "log"
	"github.com/urfave/cli"
)

type CmdHandler func(*Config, *cli.Context) error

func CmdNotImplemented(*Config, *cli.Context) error {
	return fmt.Errorf("Command not implemented")
}

func main() {
	// Now the setup for the application

	cliapp := cli.NewApp()
	cliapp.Name = "s3-cli"
	// cliapp.Usage = ""
	cliapp.Version = "0.2.0"

	cli.VersionFlag = cli.BoolFlag{
		Name:  "version, V",
		Usage: "print version number",
	}

	cliapp.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name:  "config, c",
			Value: &cli.StringSlice{"$HOME/.s3cfg"},
			Usage: "Config `FILE` name.",
		},
		cli.StringFlag{
			Name:   "access-key",
			Usage:  "AWS Access Key `ACCESS_KEY`",
			EnvVar: "AWS_ACCESS_KEY_ID,AWS_ACCESS_KEY",
		},
		cli.StringFlag{
			Name:   "secret-key",
			Usage:  "AWS Secret Key `SECRET_KEY`",
			EnvVar: "AWS_SECRET_ACCESS_KEY,AWS_SECRET_KEY",
		},
		cli.StringFlag{
			Name:   "exec-on-change",
			Usage:  "run a command in case of changes after sync",
			EnvVar: "EXEC_ON_CHANGE,ON_CHANGE_EXEC,ON_CHANGE",
		},

		cli.BoolFlag{
			Name:  "recursive,r",
			Usage: "Recursive upload, download or removal",
		},
		cli.BoolFlag{
			Name:  "force",
			Usage: "Force overwrite and other dangerous operations.",
		},
		cli.BoolFlag{
			Name:  "skip-existing",
			Usage: "Skip over files that exist at the destination (only for [get] and [sync] commands).",
		},

		cli.BoolFlag{
			Name:  "verbose,v",
			Usage: "Verbose output (e.g. debugging)",
		},
		cli.BoolFlag{
			Name:  "dry-run,n",
			Usage: "Only show what should be uploaded or downloaded but don't actually do it. May still perform S3 requests to get bucket listings and other information though (only for file transfer commands)",
		},
		cli.BoolFlag{
			Name:  "check-md5",
			Usage: "Check MD5 sums when comparing files. (default)",
		},
		cli.BoolFlag{
			Name:  "no-check-md5",
			Usage: "Do not check MD5 sums when comparing files.",
		},
	}

	// The wrapper to launch a command -- take care of standard setup
	//  before we get going
	launch := func(handler CmdHandler) func(*cli.Context) error {
		return func(c *cli.Context) error {
			err := handler(NewConfig(c), c)
			if err != nil {
				fmt.Println(err.Error())
			}
			return err
		}
	}

	cliapp.Commands = []cli.Command{
		{
			Name:   "sync",
			Usage:  "Synchronize a directory tree to S3 -- LOCAL_DIR s3://BUCKET[/PREFIX] or s3://BUCKET[/PREFIX] LOCAL_DIR",
			Action: launch(CmdSync),
			Flags:  cliapp.Flags,
		},
	}

	cliapp.Run(os.Args)
}
