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
	"path/filepath"
	"syscall"

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/internal/kmod"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/urfave/cli/v2"
)

func ManageKernelModules(c *cli.Context) error {
	args := &cmd.KernelModulesArgs

	if c.App.Metadata == nil || c.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	system := c.App.Metadata["system"].(*sys.System)

	ctx, cancelFunc := signal.NotifyContext(c.Context, syscall.SIGTERM, syscall.SIGINT)
	defer cancelFunc()

	logger := system.Logger()

	kernelModules, err := kmod.ListKernelModules(system)
	if err != nil {
		logger.Info("Listing kernel modules failed, unable to proceed")
		return err
	} else if len(kernelModules) == 0 {
		logger.Info("No additional kernel modules found, skipping operation")
		return nil
	}

	config := kmod.NewConfig()

	if args.Unload {
		unloader := &kmod.Unloader{
			System: system,
			Config: config,
		}

		return unloader.Unload(ctx, kernelModules)
	}

	kernel, _, err := vfs.FindKernel(system.FS(), "/")
	if err != nil {
		logger.Error("Finding kernel directory failed, unable to proceed")
		return err
	}
	kernelDir := filepath.Dir(kernel)

	reloader := &kmod.Reloader{
		System: system,
		Config: config,
		ModuleCache: &kmod.ModuleCache{
			FS:        system.FS(),
			Runner:    system.Runner(),
			KernelDir: kernelDir,
		},
		KernelDir: kernelDir,
	}

	return reloader.Reload(ctx, kernelModules)
}
