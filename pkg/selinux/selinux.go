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

package selinux

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	SelinuxTargetedContextFile = selinuxTargetedPath + "/contexts/files/file_contexts"
	SelinuxTargetedPolicyType  = "targeted"

	defaultStoreRoot    = "/var/lib/selinux"
	migrateMarkerPrefix = "/etc/selinux/elemental-selinux_modules_migrated-"
	selinuxTargetedPath = "/etc/selinux/" + SelinuxTargetedPolicyType
	selinuxAutoRelabel  = "/etc/selinux/.autorelabel"
	debugLines          = 10
)

// SystemRelabel applies the SE Linux labels based on the targeted policy found within the given
// root path. It force applies the labels under the given root except for the given shared RW paths.
// This is to prevent runtime changes during the upgrades as RW paths are potentially in use for current
// processes. For snapshotted RW paths it applies SE Linux labels without force flag as it might include
// customized content merged with stock OS content.
// If at least one shared RW path is provided it also sets the .autorelabel file to trigger
// relabelling at boot and relabel the excluded paths.
func SystemRelabel(ctx context.Context, s *sys.System, rootDir string, snapshotted []string, shared []string) error {
	contextFile := filepath.Join(rootDir, SelinuxTargetedContextFile)
	contextExists, _ := vfs.Exists(s.FS(), contextFile)

	if contextExists {
		var err error

		baseArgs := []string{"-i"}

		// We only keep last 10 lines of the stdout and stderr for debugging purposes
		stdOut := ring.New(debugLines)
		stdErr := ring.New(debugLines)

		if rootDir == "/" || rootDir == "" {
			rootDir = "/"
		} else {
			baseArgs = append(baseArgs, "-r", rootDir)
		}

		args := []string{"-F"}
		if len(snapshotted) > 0 {
			for _, path := range snapshotted {
				args = append(args, "-e", path)
			}
		}
		if len(shared) > 0 {
			for _, path := range shared {
				args = append(args, "-e", path)
			}
			err = s.FS().WriteFile(filepath.Join(rootDir, selinuxAutoRelabel), []byte{}, vfs.FilePerm)
			if err != nil {
				return fmt.Errorf("creating .autorelabel file: %w", err)
			}
		}
		args = append(args, contextFile, rootDir)

		s.Logger().Info("Applying SE Linux labels to the read-only root tree, forced relabelling")
		err = s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), "setfiles", slices.Concat(baseArgs, args)...)
		logOutput(s, stdOut, stdErr)

		if len(snapshotted) > 0 {
			s.Logger().Info("Applying SE Linux labels to snapshotted RW volumes")
			for _, path := range snapshotted {
				stdOut = ring.New(debugLines)
				stdErr = ring.New(debugLines)
				err = s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), "setfiles", append(baseArgs, contextFile, path)...)
				logOutput(s, stdOut, stdErr)
			}
		}

		return err
	}

	s.Logger().Warn("Not relabelling SE Linux, no context found")
	return nil
}

// ChrootedSystemRelabel applies the SE Linux labels based on the targeted policy found within the given
// root path. Runs the same logic as RelabelSystem method but running inside a chroot environment.
func ChrootedSystemRelabel(ctx context.Context, s *sys.System, rootDir string, snapshotted []string, shared []string) error {
	callback := func() error { return SystemRelabel(ctx, s, "/", snapshotted, shared) }
	err := chroot.ChrootedCallback(s, rootDir, nil, callback, chroot.WithoutDefaultBinds())
	if err != nil {
		return fmt.Errorf("chrooted system relabel: %w", err)
	}
	return nil
}

// RefreshPolicy recompiles the policy if its modules changed and otherwise regenerates
// the installed policy from the linked policy and local customizations.
func RefreshPolicy(ctx context.Context, s *sys.System, policyType string) error {
	const cmd = "semodule"
	args := []string{"-n", "--refresh", "-s", policyType}

	stdOut := ring.New(debugLines)
	stdErr := ring.New(debugLines)

	s.Logger().Info("Refreshing SE Linux policy from store '%s'", policyType)
	err := s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), cmd, args...)
	logOutput(s, stdOut, stdErr)

	return err
}

// ChrootedRefreshPolicy runs RefreshPolicy for the specified policy in a chroot of the given
// root path. The refresh is skipped if the store root is the default policy store
// (/var/lib/selinux), or the custom store root has no active policy store.
func ChrootedRefreshPolicy(ctx context.Context, s *sys.System, rootDir, policyType string) error {
	callback := func() error {
		storeRoot, err := getStoreRoot(s)
		if err != nil {
			return fmt.Errorf("parsing store-root from policy configuration: %w", err)
		}

		// Do not refresh when the store is under /var, as it still holds old base modules and
		// rebuilding it would compile them into the new snapshot.
		if storeRoot == defaultStoreRoot {
			s.Logger().Info("SE Linux policy store is in the default %q, skipping policy refresh", storeRoot)
			return nil
		}

		// Refresh only on active policy store entry.
		store := filepath.Join(storeRoot, policyType, "active")
		if exists, _ := vfs.Exists(s.FS(), store); !exists {
			s.Logger().Info("No SE Linux policy store found at %q, skipping policy refresh", store)
			return nil
		}

		return RefreshPolicy(ctx, s, policyType)
	}
	err := chroot.ChrootedCallback(s, rootDir, nil, callback, chroot.WithoutDefaultBinds())
	if err != nil {
		return fmt.Errorf("chrooted policy refresh: %w", err)
	}
	return nil
}

// ChrootedDefaultPolicyMigration attempts to migrate any existing default (/var/lib/selinux) policy store
// of the given type to the store specified under /etc/selinux/semanage.conf in a chroot environment. During migration all
// *.local files are migrated, as well as all modules with priority > 200 that do not already exist in the new store.
//
// Migration is skipped when there is no active default store, the default store has already been migrated,
// the semanage.conf does not define a custom store, or when the new custom store has no 'active' entry.
func ChrootedDefaultPolicyMigration(s *sys.System, rootDir, policyType string) error {
	callback := func() error {
		defaultPolicy := filepath.Join(defaultStoreRoot, policyType, "active")
		if exists, _ := vfs.Exists(s.FS(), defaultPolicy); !exists {
			// The default store is removed by 'cleanoldsepoldir.service' once there are no more
			// snapshots containing it. At that point, there is no need for the elemental migration
			// marker as well.
			if err := s.FS().Remove(migrateMarkerPrefix + policyType); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("removing migration marker: %w", err)
			}
			return nil
		}

		storeRoot, err := getStoreRoot(s)
		if err != nil {
			return fmt.Errorf("parsing store-root from policy configuration: %w", err)
		}

		if !shouldMigrateDefaultPolicy(s, storeRoot, policyType) {
			return nil
		}

		destPolicy := filepath.Join(storeRoot, policyType, "active")
		if err := migratePolicy(s, defaultPolicy, destPolicy); err != nil {
			return fmt.Errorf("migrating policy: %w", err)
		}

		return s.FS().WriteFile(migrateMarkerPrefix+policyType, []byte{}, 0o644)
	}

	if err := chroot.ChrootedCallback(s, rootDir, nil, callback, chroot.WithoutDefaultBinds()); err != nil {
		return fmt.Errorf("chrooted default SE Linux policy migration: %w", err)
	}

	return nil
}

// shouldMigrateDefaultPolicy determines whether the default (/var/lib/selinux) policy store
// of a specific type can be migrated to the specified store root.
func shouldMigrateDefaultPolicy(s *sys.System, storeRoot, policyType string) bool {
	// An existing migration marker means the default policy store has already been migrated,
	// but there are still snapshots that hold the old store.
	if exists, _ := vfs.Exists(s.FS(), migrateMarkerPrefix+policyType); exists {
		return false
	}

	// Migrate only if the given store is different from the default store path.
	if storeRoot == defaultStoreRoot {
		return false
	}

	// Migration requires an existing active policy store for the expected policy type.
	store := filepath.Join(storeRoot, policyType, "active")
	if exists, _ := vfs.Exists(s.FS(), store); !exists {
		s.Logger().Warn("Missing %q policy store. Local policy migration will not be performed.", store)
		return false
	}

	return true
}

// getStoreRoot parses the '/etc/selinux/semanage.conf' and returns the value of the
// 'store-root' property. In the event of a missing 'store-root' returns the default
// policy store - /var/lib/selinux.
func getStoreRoot(s *sys.System) (string, error) {
	const (
		semanageConf = "/etc/selinux/semanage.conf"
	)

	policyStoreRoot := defaultStoreRoot
	data, err := s.FS().ReadFile(semanageConf)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return policyStoreRoot, nil
		}
		return "", fmt.Errorf("reading policy store configuration: %w", err)
	}

	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "store-root" {
			continue
		}
		policyStoreRoot = filepath.Clean(strings.Trim(strings.TrimSpace(value), `"`))
	}

	return policyStoreRoot, nil
}

// migratePolicy migrates a source policy store to the given destination.
// Migration is done for all *.local files and for all modules that are with
// priority > 200 and do not already exist in the destination store.
func migratePolicy(s *sys.System, src, dst string) error {
	if err := copyLocalFiles(s, src, dst); err != nil {
		return fmt.Errorf("copying local config files from source policy %q: %w", src, err)
	}

	if err := copyLocalModules(s, src, dst); err != nil {
		return fmt.Errorf("copying modules from source policy %q: %w", src, err)
	}

	return nil
}

// copyLocalModules copies modules from src to dst. Modules with priority <= 200
// are skipped.
func copyLocalModules(s *sys.System, src, dst string) error {
	shouldCopyModule := func(name string) bool {
		if name == "disabled" {
			// To ensure disabled modules stay disabled, the
			// 'disabled' modules directory needs to be copied.
			return true
		}

		priority, err := strconv.Atoi(name)
		// 100 - 200 range should be coming from the image
		// itself. Only copy additions above the default range.
		return err == nil && priority > 200
	}

	srcModules := filepath.Join(src, "modules")
	entries, err := s.FS().ReadDir(srcModules)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("listing local modules %q: %w", srcModules, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || !shouldCopyModule(entry.Name()) {
			continue
		}

		src := filepath.Join(srcModules, entry.Name())
		dst := filepath.Join(dst, "modules", entry.Name())
		if err := copyMissingModules(s, src, dst); err != nil {
			return fmt.Errorf("copying modules from %q to %q: %w", src, dst, err)
		}
	}
	return nil
}

func copyMissingModules(s *sys.System, src, dst string) error {
	entries, err := s.FS().ReadDir(src)
	if err != nil {
		return fmt.Errorf("listing source module dir %q: %w", src, err)
	}

	if err := vfs.MkdirAll(s.FS(), dst, 0o700); err != nil {
		return fmt.Errorf("creating target module dir %q: %w", dst, err)
	}

	for _, entry := range entries {
		source := filepath.Join(src, entry.Name())
		target := filepath.Join(dst, entry.Name())
		if ok, _ := vfs.Exists(s.FS(), target); ok {
			// Overwriting existing modules is not desirable as it may cause
			// incorrect image state.
			continue
		}

		if entry.IsDir() {
			if err := vfs.MkdirAll(s.FS(), target, 0o700); err != nil {
				return fmt.Errorf("creating module directory %q: %w", target, err)
			}

			if err := vfs.CopyDir(s.FS(), source, target, false, nil); err != nil {
				return fmt.Errorf("copying module %q to %q: %w", source, target, err)
			}
		} else {
			if err := vfs.CopyFile(s.FS(), source, target); err != nil {
				return fmt.Errorf("copying file %q to %q: %w", source, target, err)
			}
		}
	}
	return nil
}

// copyLocalFiles copies all *.local files from a source policy store to the
// given destination store.
func copyLocalFiles(s *sys.System, src, dst string) error {
	files, err := vfs.FindFiles(s.FS(), src, "*.local")
	if err != nil {
		return fmt.Errorf("listing local config files at %q: %w", src, err)
	}
	for _, source := range files {
		target := filepath.Join(dst, filepath.Base(source))
		// Files that already exist in dst are replaced so that user-defined
		// configurations are not lost.
		if replaced, _ := vfs.Exists(s.FS(), target); replaced {
			s.Logger().Info("Replacing %q with locally defined %q", target, source)
		}

		if err = vfs.CopyFile(s.FS(), source, target); err != nil {
			return fmt.Errorf("copying local config file %q: %w", source, err)
		}
	}
	return nil
}

func stdHander(r *ring.Ring) func(string) {
	return func(line string) {
		r.Value = line
		r = r.Next()
	}
}

func logOutput(s *sys.System, stdOut, stdErr *ring.Ring) {
	output := "\n------- stdOut -------\n"
	stdOut.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "------- stdErr -------\n"
	stdErr.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "----------------------\n"
	s.Logger().Debug("SE Linux command output: %s", output)
}
