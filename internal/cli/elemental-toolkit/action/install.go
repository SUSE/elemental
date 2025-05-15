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
	"sigs.k8s.io/yaml"

	"github.com/suse/elemental/v3/internal/cli/elemental-toolkit/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func Install(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.InstallArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting install action with args: %+v", args)

	d, err := digestInstallSetup(s, args)
	if err != nil {
		s.Logger().Error("failed to collect installation setup: %v", err)
		return err
	}

	s.Logger().Info("Checked configuration, running installation process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	manager := firmware.NewEfiBootManager(s)

	boot, err := bootloader.New(d.BootConfig.Bootloader, s)
	if err != nil {
		s.Logger().Error("Failed to parse boot config: %s", err.Error())
		return err
	}

	installer := install.New(ctxCancel, s, install.WithBootloader(boot), install.WithBootManager(manager))
	err = installer.Install(d)
	if err != nil {
		s.Logger().Error("installation failed: %v", err)
		return err
	}

	s.Logger().Info("Installation complete")

	return nil
}

func digestInstallSetup(s *sys.System, flags *cmd.InstallFlags) (*deployment.Deployment, error) {
	d := deployment.DefaultDeployment()
	if flags.Description != "" {
		if ok, _ := vfs.Exists(s.FS(), flags.Description); !ok {
			return nil, fmt.Errorf("config file '%s' not found", flags.Description)
		}
		data, err := s.FS().ReadFile(flags.Description)
		if err != nil {
			return nil, fmt.Errorf("could not read description file '%s': %w", flags.Description, err)
		}
		err = yaml.Unmarshal(data, d)
		if err != nil {
			return nil, fmt.Errorf("could not unmarshal config file: %w", err)
		}
	}
	if flags.Target != "" && len(d.Disks) > 0 {
		d.Disks[0].Device = flags.Target
	}

	if flags.OperatingSystemImage != "" {
		srcOS, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
		if err != nil {
			return nil, fmt.Errorf("failed parsing OS source URI ('%s'): %w", flags.OperatingSystemImage, err)
		}
		d.SourceOS = srcOS
	}

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
		d.Firmware.BootEntries = []*firmware.EfiBootEntry{
			firmware.DefaultBootEntry(s.Platform(), d.Disks[0].Device),
		}
	}

	if flags.Bootloader != "none" {
		d.BootConfig.Bootloader = flags.Bootloader
	}

	err := d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, nil
}
