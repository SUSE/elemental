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
	"errors"
	"fmt"

	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func ListKernelModules(s *sys.System) ([]string, error) {
	d, err := deployment.Parse(s, "/")
	if err != nil {
		return nil, fmt.Errorf("parsing deployment information: %w", err)
	} else if d == nil {
		return nil, fmt.Errorf("deployment information not found")
	}

	var kernelModules []string

	if d.Extensions != nil {
		kernelModules = d.Extensions.KernelModules
	}

	// TODO (atanasdinov): Merge modules from deployment.yaml with ones coming from CLI args?

	return kernelModules, nil
}

func prepareEnvironment(s *sys.System, c *Config, cleanup *cleanstack.CleanStack) error {
	if err := cleanupEnvironment(s, c); err != nil {
		return fmt.Errorf("cleaning up environment: %w", err)
	}

	cleanup.PushErrorOnly(func() error {
		return s.FS().RemoveAll(c.BaseDir)
	})

	for _, dir := range []string{c.MountPoint, c.BaseDir, c.ModuleCacheDir(), c.OverlayWorkDir()} {
		if err := vfs.MkdirAll(s.FS(), dir, vfs.DirPerm); err != nil {
			return err
		}
	}

	return nil
}

func cleanupEnvironment(s *sys.System, c *Config) error {
	isMountPoint, _ := s.Mounter().IsMountPoint(c.MountPoint)
	if isMountPoint {
		if err := s.Mounter().Unmount(c.MountPoint); err != nil {
			return fmt.Errorf("unmounting %s: %w", c.MountPoint, err)
		}
	}

	return s.FS().RemoveAll(c.BaseDir)
}

func manageModules(ctx context.Context, s *sys.System, modulesPath string, modules []string, remove bool) error {
	var err error

	operation := "activating"
	if remove {
		operation = "deactivating"
	}

	for _, module := range modules {
		if mErr := modprobe(ctx, s.Runner(), modulesPath, module, remove); mErr != nil {
			err = errors.Join(err, fmt.Errorf("%s module %s: %w", operation, module, mErr))
			continue
		}

		s.Logger().Info("Module %s %s completed", module, operation)
	}

	return err
}

func modprobe(ctx context.Context, runner sys.Runner, modulesPath, module string, remove bool) error {
	args := []string{"-d", modulesPath, module}
	if remove {
		args = append(args, "-r")
	}

	_, err := runner.RunContext(ctx, "modprobe", args...)
	return err
}
