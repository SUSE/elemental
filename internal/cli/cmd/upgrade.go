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

type UpgradeFlags struct {
	OperatingSystemImage string
}

var UpgradeArgs UpgradeFlags

func NewUpgradeCommand(action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "upgrade",
		Usage:     "Upgrade system from an OS image",
		UsageText: fmt.Sprintf("%s upgrade [OPTIONS]", appName()),
		Action:    action,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "os-image",
				Usage:       "image containing the operating system",
				Destination: &UpgradeArgs.OperatingSystemImage,
				Required:    true,
			},
		},
	}
}
