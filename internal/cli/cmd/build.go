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

	"github.com/urfave/cli/v2"
)

type BuildFlags struct {
	ImageType  string
	Platform   string
	ConfigDir  string
	BuildDir   string
	OutputPath string
	Local      bool
}

var BuildArgs BuildFlags

func NewBuildCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "build",
		Usage:     "[DEPRECATED] Build new image",
		UsageText: fmt.Sprintf("[DEPRECATED] %s build [OPTIONS]", appName),
		Action:    action,
		Hidden:    true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "image-type",
				Usage:       "Type of image artifact to build (RAW or ISO)",
				Destination: &BuildArgs.ImageType,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "platform",
				Usage:       "Target platform",
				Destination: &BuildArgs.Platform,
				Value:       fmt.Sprintf("linux/%s", runtime.GOARCH),
			},
			&cli.StringFlag{
				Name:        "config-dir",
				Usage:       "Full path to the image configuration directory",
				Destination: &BuildArgs.ConfigDir,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "build-dir",
				Usage:       "Full path to the directory to store build artifacts",
				Destination: &BuildArgs.BuildDir,
				Value:       "_build",
			},
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "Filepath for the output image",
				Destination: &BuildArgs.OutputPath,
				DefaultText: "image-<timestamp>.<image-type>",
			},
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Load OCI images from the local container storage instead of a remote registry",
				Destination: &BuildArgs.Local,
			},
		},
	}
}
