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
