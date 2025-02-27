package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

type BuildFlags struct {
	OperatingSystemImage string
	ConfigDir            string
}

var BuildArgs BuildFlags

func NewBuildCommand(action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "build",
		Usage:     "Build new image",
		UsageText: fmt.Sprintf("%s build [OPTIONS]", appName()),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "os-image",
				Usage:       "OCI image containing the operating system",
				Destination: &BuildArgs.OperatingSystemImage,
			},
			&cli.StringFlag{
				Name:        "config-dir",
				Usage:       "Full path to the image configuration directory",
				Destination: &BuildArgs.ConfigDir,
			},
		},
	}
}
