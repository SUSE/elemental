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

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type ModuleCache struct {
	KernelDir string
	FS        vfs.FS
	Runner    sys.Runner
}

func (c *ModuleCache) Build(ctx context.Context, outputDir string) error {
	if err := depmod(ctx, c.Runner, outputDir); err != nil {
		return fmt.Errorf("generating modules.dep and map files: %w", err)
	}

	modulesKernelDir := filepath.Join(outputDir, c.KernelDir)
	entries, err := c.FS.ReadDir(modulesKernelDir)
	if err != nil {
		return fmt.Errorf("reading kernel path: %w", err)
	}

	// Flatten the hierarchy to allow for overlaying on top of the kernel directory.
	for _, entry := range entries {
		source := filepath.Join(modulesKernelDir, entry.Name())
		target := filepath.Join(outputDir, entry.Name())

		if err = c.FS.Rename(source, target); err != nil {
			return fmt.Errorf("flattening entry %s: %w", entry.Name(), err)
		}
	}

	// Drop the full path as it is a target for symlink later.
	if err = c.FS.RemoveAll(modulesKernelDir); err != nil {
		return fmt.Errorf("removing kernel path: %w", err)
	}

	return nil
}

func depmod(ctx context.Context, runner sys.Runner, outputDir string) error {
	_, err := runner.RunContext(ctx, "depmod", "--all", "-o", outputDir)
	return err
}
