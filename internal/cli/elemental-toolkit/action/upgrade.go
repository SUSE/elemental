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

package action

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/suse/elemental/v3/internal/cli/elemental-toolkit/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func Upgrade(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.UpgradeArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting upgrade action with args: %+v", args)

	d, err := digestUpgradeSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect upgrade setup")
		return err
	}

	s.Logger().Info("Checked configuration, running upgrade process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	bootloader, err := bootloader.New(d.BootConfig.Bootloader, s)
	if err != nil {
		s.Logger().Error("Parsing boot config failed")
		return err
	}

	upgrader := upgrade.New(ctxCancel, s, upgrade.WithBootloader(bootloader), upgrade.WithUnpackOpts(unpack.WithVerify(args.Verify)))

	err = upgrader.Upgrade(d)
	if err != nil {
		s.Logger().Error("Upgrade failed")
		return err
	}

	s.Logger().Info("Upgrade completed")

	return nil
}

func digestUpgradeSetup(s *sys.System, flags *cmd.UpgradeFlags) (*deployment.Deployment, error) {
	d, err := deployment.Parse(s, "/")
	if err != nil {
		return nil, fmt.Errorf("parsing deployment: %w", err)
	} else if d == nil {
		return nil, fmt.Errorf("deployment not found")
	}

	srcOS, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
	if err != nil {
		return nil, fmt.Errorf("failed parsing OS source URI ('%s'): %w", flags.OperatingSystemImage, err)
	}
	d.SourceOS = srcOS

	if flags.Overlay != "" {
		overlay, err := deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return nil, fmt.Errorf("failed parsing overlay source URI ('%s'): %w", flags.Overlay, err)
		}
		d.OverlayTree = overlay
	}

	if flags.ConfigScript != "" {
		d.CfgScript = flags.ConfigScript
	}

	if flags.CreateBootEntry {
		if d.Firmware == nil {
			d.Firmware = &deployment.FirmwareConfig{}
		}
		d.Firmware.BootEntries = []*firmware.EfiBootEntry{
			firmware.DefaultBootEntry(s.Platform(), d.Disks[0].Device),
		}
	}

	err = d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, nil
}
