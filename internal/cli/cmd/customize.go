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
	"runtime"

	"github.com/suse/elemental/v3/pkg/installer"
	"github.com/urfave/cli/v2"
)

type CustomizeFlags struct {
	ConfigDir       string
	OutputName      string
	Platform        string
	CustomizeOutput string
	MediaType       string
	Local           bool
}

var CustomizeArgs CustomizeFlags

func NewCustomizeCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "customize",
		Usage:     "Customize an image based on a release",
		UsageText: fmt.Sprintf("%s customize", appName),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "type",
				Usage:       "Type of the installer media, 'iso' or 'raw' (default: iso)",
				Destination: &CustomizeArgs.MediaType,
				Value:       installer.ISO.String(),
			},
			&cli.StringFlag{
				Name:        "config-dir",
				Usage:       "Full path to the image configuration directory",
				Destination: &CustomizeArgs.ConfigDir,
			},
			&cli.StringFlag{
				Name:        "output",
				Usage:       "Filename for the output image without extension, this is added based on type",
				Destination: &CustomizeArgs.OutputName,
				DefaultText: "image-<timestamp>",
			},
			&cli.StringFlag{
				Name:        "platform",
				Usage:       "Target platform",
				Destination: &CustomizeArgs.Platform,
				Value:       fmt.Sprintf("linux/%s", runtime.GOARCH),
			},
			&cli.StringFlag{
				Name:        "customize-output",
				Usage:       "Full path to the directory to store customize artifacts",
				Destination: &CustomizeArgs.CustomizeOutput,
				Value:       "_customize",
			},
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Load OCI images from the local container storage instead of a remote registry",
				Destination: &CustomizeArgs.Local,
			},
		},
	}
}
