package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

type InstallFlags struct {
	OperatingSystemImage string
	Target               string
}

var InstallArgs InstallFlags

func NewInstallCommand(action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "Install an OCI image on a target system",
		UsageText: fmt.Sprintf("%s install [OPTIONS]", appName()),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "os-image",
				Usage:       "OCI image containing the operating system",
				Destination: &InstallArgs.OperatingSystemImage,
			},
			&cli.StringFlag{
				Name:        "target",
				Aliases:     []string{"t"},
				Usage:       "Target device for the installation process",
				Destination: &InstallArgs.Target,
			},
		},
	}
}
