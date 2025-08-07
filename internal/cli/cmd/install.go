/*
Copyright © 2025 SUSE LLC
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

type InstallFlags struct {
	OperatingSystemImage string
	Target               string
	Description          string
	ConfigScript         string
	Overlay              string
	CreateBootEntry      bool
	Bootloader           string
	KernelCmdline        string
	Verify               bool
	Local                bool
}

var InstallArgs InstallFlags

func NewInstallCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "Install an OCI image on a target system",
		UsageText: fmt.Sprintf("%s install [OPTIONS]", appName),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Usage:       "Path to OS image post-commit script",
				Destination: &InstallArgs.ConfigScript,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "Description file to read installation details",
				Destination: &InstallArgs.Description,
			},
			&cli.StringFlag{
				Name:        "os-image",
				Usage:       "URI to the image containing the operating system",
				Destination: &InstallArgs.OperatingSystemImage,
			},
			&cli.StringFlag{
				Name:        "overlay",
				Usage:       "URI of the overlay content for the OS image",
				Destination: &InstallArgs.Overlay,
			},
			&cli.StringFlag{
				Name:        "target",
				Aliases:     []string{"t"},
				Usage:       "Target device for the installation process",
				Destination: &InstallArgs.Target,
			},
			&cli.BoolFlag{
				Name:        "create-boot-entry",
				Usage:       "Create EFI boot entry",
				Destination: &InstallArgs.CreateBootEntry,
			},
			&cli.StringFlag{
				Name:        "bootloader",
				Aliases:     []string{"b"},
				Value:       "grub",
				Usage:       "Bundled bootloader to install to ESP",
				Destination: &InstallArgs.Bootloader,
			},
			&cli.StringFlag{
				Name:        "cmdline",
				Value:       "",
				Usage:       "Kernel cmdline for installed system",
				Destination: &InstallArgs.KernelCmdline,
			},
			&cli.BoolFlag{
				Name:        "verify",
				Value:       true,
				Usage:       "Verify OCI ssl",
				Destination: &InstallArgs.Verify,
			},
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Load OCI images from the local container storage instead of a remote registry",
				Destination: &InstallArgs.Local,
			},
		},
	}
}
