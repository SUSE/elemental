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

type InstallerFlags struct {
	InstallSpec          InstallFlags
	OperatingSystemImage string
	ConfigScript         string
	Local                bool
	Verify               bool
	Name                 string
	OutputDir            string
	Overlay              string
	Label                string
	KernelCmdLine        string
}

var InstallerArgs InstallerFlags

func NewBuildISOCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "build-iso",
		Usage:     "Build an installer ISO",
		UsageText: fmt.Sprintf("%s build-iso [OPTIONS]", appName),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "install-config",
				Usage:       "Path to OS image post-commit script",
				Destination: &InstallerArgs.InstallSpec.ConfigScript,
			},
			&cli.StringFlag{
				Name:        "install-description",
				Usage:       "Description file to read installation details",
				Destination: &InstallerArgs.InstallSpec.Description,
			},
			&cli.StringFlag{
				Name:        "install-overlay",
				Usage:       "URI of the overlay content for the OS image",
				Destination: &InstallerArgs.InstallSpec.Overlay,
			},
			&cli.StringFlag{
				Name:        "install-target",
				Usage:       "Target device for the installation process",
				Destination: &InstallerArgs.InstallSpec.Target,
			},
			&cli.StringFlag{
				Name:        "install-cmdline",
				Value:       "",
				Usage:       "Kernel cmdline for installed system",
				Destination: &InstallerArgs.InstallSpec.KernelCmdline,
			},
			&cli.StringFlag{
				Name:        "config",
				Usage:       "Path to installer media config script",
				Destination: &InstallerArgs.ConfigScript,
			},
			&cli.BoolFlag{
				Name:        "verify",
				Value:       true,
				Usage:       "Verify OCI ssl",
				Destination: &InstallerArgs.Verify,
			},
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Load OCI images from the local container storage instead of a remote registry",
				Destination: &InstallerArgs.Local,
			},
			&cli.StringFlag{
				Name:        "output",
				Usage:       "Location for the temporary builtime files and the resulting image",
				Destination: &InstallerArgs.OutputDir,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "name",
				Usage:       "Name of the resulting image file",
				Destination: &InstallerArgs.Name,
			},
			&cli.StringFlag{
				Name:        "os-image",
				Usage:       "URI to the image containing the operating system",
				Destination: &InstallerArgs.OperatingSystemImage,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "overlay",
				Usage:       "URI of the data to include in installer media",
				Destination: &InstallerArgs.Overlay,
			},
			&cli.StringFlag{
				Name:        "label",
				Usage:       "Label of the installer media filesystem",
				Destination: &InstallerArgs.Label,
			},
			&cli.StringFlag{
				Name:        "cmdline",
				Usage:       "Kernel command line to boot the installer media",
				Destination: &InstallerArgs.KernelCmdLine,
			},
		},
	}
}
