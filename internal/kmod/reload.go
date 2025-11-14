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
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/overlay"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type moduleCacheBuilder interface {
	Build(ctx context.Context, outputDir string) error
}

type Reloader struct {
	*sys.System

	Config      *Config
	ModuleCache moduleCacheBuilder
	KernelDir   string
}

func (r *Reloader) Reload(ctx context.Context, kernelModules []string) (err error) {
	logger := r.Logger()

	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	logger.Info("Generating module files")
	modulesDir, err := r.generateModules(ctx, cleanup)
	if err != nil {
		logger.Error("Generating module files failed")
		return err
	}

	logger.Info("Activating kernel modules")
	if err = manageModules(ctx, r.System, modulesDir, kernelModules, false); err != nil {
		logger.Error("Activating one or more kernel modules failed")
		return err
	}

	logger.Info("Reloading kernel modules completed")

	return nil
}

func (r *Reloader) generateModules(ctx context.Context, cleanup *cleanstack.CleanStack) (string, error) {
	if err := prepareEnvironment(r.System, r.Config, cleanup); err != nil {
		return "", fmt.Errorf("preparing environment: %w", err)
	}

	moduleCacheDir := r.Config.ModuleCacheDir()

	if err := r.ModuleCache.Build(ctx, moduleCacheDir); err != nil {
		return "", fmt.Errorf("building modules cache: %w", err)
	}

	o, err := overlay.New(
		r.KernelDir,
		moduleCacheDir,
		r.Mounter(),
		r.FS(),
		overlay.WithWorkDir(r.Config.OverlayWorkDir()),
		overlay.WithTarget(r.Config.MountPoint),
	)
	if err != nil {
		return "", fmt.Errorf("setting up overlay: %w", err)
	}

	mergedDir, err := o.Mount()
	if err != nil {
		return "", fmt.Errorf("mounting overlay filesystem: %w", err)
	}
	cleanup.PushErrorOnly(o.Unmount)

	// Ensure the merged directory is usable by modprobe.
	if err = linkKernelModuleTree(r.FS(), mergedDir, r.KernelDir); err != nil {
		return "", fmt.Errorf("linking kernel module tree: %w", err)
	}

	return mergedDir, nil
}

func linkKernelModuleTree(fs vfs.FS, targetDir, kernelDir string) error {
	modulesPath := filepath.Dir(kernelDir)
	kernelVersion := filepath.Base(kernelDir)

	symlinkParentDir := filepath.Join(targetDir, modulesPath)
	symlinkPath := filepath.Join(symlinkParentDir, kernelVersion)

	relativePath, err := filepath.Rel(symlinkParentDir, targetDir)
	if err != nil {
		return fmt.Errorf("identifying relative modules path: %w", err)
	}

	if err = fs.Symlink(relativePath, symlinkPath); err != nil {
		return fmt.Errorf("creating symlink at %s: %w", symlinkPath, err)
	}

	return nil
}
