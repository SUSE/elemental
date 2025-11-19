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

type UnpackFlags struct {
	Image     string
	TargetDir string
	Platform  string
	Local     bool
	Verify    bool
}

var UnpackArgs UnpackFlags

func NewUnpackImageCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "unpack-image",
		Usage:     "Unpacks an image to the specified location",
		UsageText: fmt.Sprintf("%s unpack-image [OPTIONS]", appName),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "image",
				Usage:       "OCI image to unpack",
				Destination: &UnpackArgs.Image,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "target",
				Aliases:     []string{"t"},
				Usage:       "Target directory",
				Destination: &UnpackArgs.TargetDir,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "platform",
				Usage:       "OCI Image platform",
				Destination: &UnpackArgs.Platform,
				Value:       fmt.Sprintf("linux/%s", runtime.GOARCH),
			},
			&cli.BoolFlag{
				Name:        "verify",
				Value:       true,
				Usage:       "Verify ssl",
				Destination: &UnpackArgs.Verify,
			},
			&cli.BoolFlag{
				Name:        "local",
				Usage:       "Use local image",
				Destination: &UnpackArgs.Local,
			},
		},
	}
}
