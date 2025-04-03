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

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func Install(ctx *cli.Context) error {
	var s *sys.System
	args := &cmd.InstallArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting install action with args: %+v", args)

	d, err := digestInstallSetup(s, args)
	if err != nil {
		return err
	}

	s.Logger().Info("Checked configuration, running installation process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	installer := install.New(ctxCancel, s)
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
	if flags.ConfigFile != "" {
		if ok, _ := vfs.Exists(s.FS(), flags.ConfigFile); !ok {
			return nil, fmt.Errorf("config file '%s' not found", flags.ConfigFile)
		}
		data, err := s.FS().ReadFile(flags.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("could not read config file '%s': %w", flags.ConfigFile, err)
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

	err := d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}
	return d, nil
}
