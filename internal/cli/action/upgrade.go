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

package action

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	cmdpkg "github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func Upgrade(ctx context.Context, cmd *cli.Command) error {
	var s *sys.System
	args := &cmdpkg.UpgradeArgs
	if cmd.Root().Metadata == nil || cmd.Root().Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = cmd.Root().Metadata["system"].(*sys.System)

	s.Logger().Info("Starting upgrade action with args: %+v", args)

	d, err := digestUpgradeSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect upgrade setup")
		return err
	}

	s.Logger().Info("Checked configuration, running upgrade process")

	ctxCancel, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
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

	manager := firmware.NewEfiBootManager(s)
	upgrader := upgrade.New(
		ctxCancel, s, upgrade.WithBootloader(bootloader), upgrade.WithBootManager(manager),
		upgrade.WithUnpackOpts(unpack.WithVerify(args.Verify), unpack.WithLocal(args.Local)),
	)

	err = upgrader.Upgrade(d)
	if err != nil {
		s.Logger().Error("Upgrade failed")
		return err
	}

	s.Logger().Info("Upgrade completed")

	return nil
}

func digestUpgradeSetup(s *sys.System, flags *cmdpkg.UpgradeFlags) (*deployment.Deployment, error) {
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

	err = d.Sanitize(s, deployment.CheckDiskDevice)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, nil
}
