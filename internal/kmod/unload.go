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

package kmod

import (
	"context"

	"github.com/suse/elemental/v3/pkg/sys"
)

type Unloader struct {
	*sys.System

	Config *Config
}

func (u *Unloader) Unload(ctx context.Context, kernelModules []string) error {
	logger := u.Logger()

	isMountPoint, _ := u.Mounter().IsMountPoint(u.Config.MountPoint)
	if !isMountPoint {
		logger.Warn("Kernel modules mount point not found, skipping unload")
		return nil
	}

	if err := manageModules(ctx, u.System, u.Config.MountPoint, kernelModules, true); err != nil {
		logger.Error("Deactivating one or more kernel modules failed")
		return err
	}

	if err := cleanupEnvironment(u.System, u.Config); err != nil {
		logger.Error("Cleaning up environment failed")
		return err
	}

	logger.Info("Unloading kernel modules completed")

	return nil
}
