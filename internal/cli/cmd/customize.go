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

type CustomizeFlags struct {
	InstallSpec   InstallFlags
	InputFile     string
	OutputDir     string
	Name          string
	ConfigScript  string
	Overlay       string
	Label         string
	KernelCmdline string
}

var CustomizeArgs CustomizeFlags

func NewCustomizeCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "customize",
		Usage:     "Customize installer artifact",
		UsageText: fmt.Sprintf("%s customize", appName),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "install-config",
				Usage:       "Path to OS image post-commit script",
				Destination: &CustomizeArgs.InstallSpec.ConfigScript,
			},
			&cli.StringFlag{
				Name:        "install-description",
				Usage:       "Description file to read installation details",
				Destination: &CustomizeArgs.InstallSpec.Description,
			},
			&cli.StringFlag{
				Name:        "install-overlay",
				Usage:       "URI of the overlay content for the OS image",
				Destination: &CustomizeArgs.InstallSpec.Overlay,
			},
			&cli.StringFlag{
				Name:        "install-target",
				Usage:       "Target device for the installation process",
				Destination: &CustomizeArgs.InstallSpec.Target,
			},
			&cli.StringFlag{
				Name:        "install-cmdline",
				Value:       "",
				Usage:       "Kernel cmdline for installed system",
				Destination: &CustomizeArgs.InstallSpec.KernelCmdline,
			},
			&cli.StringFlag{
				Name:        "input",
				Usage:       "Path to local image to customize",
				Destination: &CustomizeArgs.InputFile,
			},
			&cli.StringFlag{
				Name:        "config",
				Usage:       "Path to installer media config script",
				Destination: &CustomizeArgs.ConfigScript,
			},
			&cli.StringFlag{
				Name:        "output",
				Usage:       "Location for the temporary builtime files and the resulting image",
				Destination: &CustomizeArgs.OutputDir,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "name",
				Usage:       "Name of the resulting image file",
				Destination: &CustomizeArgs.Name,
			},
			&cli.StringFlag{
				Name:        "overlay",
				Usage:       "URI of the data to include in installer media",
				Destination: &CustomizeArgs.Overlay,
			},
			&cli.StringFlag{
				Name:        "cmdline",
				Usage:       "Kernel command line to boot the installer media",
				Destination: &CustomizeArgs.KernelCmdline,
			},
		},
	}
}
