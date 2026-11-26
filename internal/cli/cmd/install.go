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
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
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
	CryptoPolicy         string
	Snapshotter          string
	HardeningProfile     string
	HardeningDatastream  string
	HardeningTailoring   string
}

var InstallArgs InstallFlags

func NewInstallCommand(appName string, action func(context.Context, *cli.Command) error) *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "Install an OCI image on a target system",
		UsageText: fmt.Sprintf("%s install [OPTIONS]", appName),
		Action:    action,
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:        configFlg,
				Usage:       configDesc,
				Destination: &InstallArgs.ConfigScript,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "Description file to read installation details",
				Destination: &InstallArgs.Description,
			},
			&cli.StringFlag{
				Name:        osImgFlg,
				Usage:       osImgDesc,
				Destination: &InstallArgs.OperatingSystemImage,
			},
			&cli.StringFlag{
				Name:        overlayFlg,
				Usage:       overlayDesc,
				Destination: &InstallArgs.Overlay,
			},
			&cli.StringFlag{
				Name:        "target",
				Aliases:     []string{"t"},
				Usage:       "Target device for the installation process",
				Destination: &InstallArgs.Target,
			},
			&cli.BoolFlag{
				Name:        createBootFlg,
				Usage:       createBootDesc,
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
				Name:        cmdlineFlg,
				Value:       "",
				Usage:       cmdlineDesc,
				Destination: &InstallArgs.KernelCmdline,
			},
			&cli.BoolFlag{
				Name:        verifyFlg,
				Value:       true,
				Usage:       verifyDesc,
				Destination: &InstallArgs.Verify,
			},
			&cli.BoolFlag{
				Name:        localFlg,
				Usage:       localDesc,
				Destination: &InstallArgs.Local,
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
		}, hardeningFlags()...),
	}
}

// hardeningFlags returns the hardening profile flags shared by install and reset.
func hardeningFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "hardening-profile",
			Usage:       "SCAP profile to harden the installed system with, e.g. stig, anssi_bp28_high, or a profile from the tailoring file",
			Destination: &InstallArgs.HardeningProfile,
		},
		&cli.StringFlag{
			Name:        "hardening-datastream",
			Usage:       "SCAP datastream path inside the OS image (defaults to the only one under /usr/share/xml/scap/ssg/content)",
			Destination: &InstallArgs.HardeningDatastream,
		},
		&cli.StringFlag{
			Name:        "hardening-tailoring",
			Usage:       "XCCDF tailoring file customizing the hardening profile",
			Destination: &InstallArgs.HardeningTailoring,
		},
	}
}
