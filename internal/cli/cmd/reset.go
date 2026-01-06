/*
Copyright © 2025-2026 SUSE LLC
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

func NewResetCommand(appName string, action func(*cli.Context) error) *cli.Command {
	return &cli.Command{
		Name:      "reset",
		Usage:     "Factory resets the current host",
		UsageText: fmt.Sprintf("%s reset [OPTIONS]", appName),
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
				Usage:       "Description file to read reset details",
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
			&cli.BoolFlag{
				Name:        "create-boot-entry",
				Usage:       "Create EFI boot entry",
				Destination: &InstallArgs.CreateBootEntry,
				Value:       true,
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
			&cli.StringFlag{
				Name:        "crypto-policy",
				Usage:       "Set the crypto policy of the installed system [default, fips]",
				Destination: &InstallArgs.CryptoPolicy,
			},
			&cli.StringFlag{
				Name:        "snapshotter",
				Usage:       "Snapshotter [snapper, overwrite]",
				Value:       "snapper",
				Destination: &InstallArgs.Snapshotter,
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
