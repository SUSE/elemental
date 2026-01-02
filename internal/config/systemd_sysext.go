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

package config

import (
	"context"
	"fmt"
	iofs "io/fs"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

func (m *Manager) downloadSystemExtensions(ctx context.Context, extensions []api.SystemdExtension, output Output) error {
	logger := m.system.Logger()
	fs := m.system.FS()
	extensionsDir := filepath.Join(output.OverlaysDir(), image.ExtensionsPath())

	if err := vfs.MkdirAll(fs, extensionsDir, 0o700); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}

	for _, extension := range extensions {
		logger.Info("Pulling extension %s from %s...",
			extension.Name, extension.Image)

		if isRemoteURL(extension.Image) {
			extensionPath := filepath.Join(extensionsDir, filepath.Base(extension.Image))
			if err := m.downloadFile(ctx, fs, extension.Image, extensionPath); err != nil {
				return fmt.Errorf("downloading systemd extension %s: %w", extension.Name, err)
			}

			continue
		}

		if err := m.unpackExtension(ctx, extension, extensionsDir); err != nil {
			return fmt.Errorf("unpacking systemd extension %s: %w", extension.Name, err)
		}
	}

	return nil
}

func isRemoteURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

func (m *Manager) unpackExtension(ctx context.Context, extension api.SystemdExtension, extensionsDir string) error {
	fs := m.system.FS()

	tempDir, err := vfs.TempDir(fs, "", fmt.Sprintf("%s-", extension.Name))
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		_ = fs.RemoveAll(tempDir)
	}()

	unpacker := unpack.NewOCIUnpacker(m.system, extension.Image, unpack.WithLocalOCI(m.local))
	if _, err = unpacker.Unpack(ctx, tempDir); err != nil {
		return fmt.Errorf("unpacking extension: %w", err)
	}

	entries, err := fs.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("reading unpacked directory: %w", err)
	}

	if len(entries) == 1 {
		entry := entries[0]
		if !entry.IsDir() {
			file := filepath.Join(tempDir, entry.Name())
			if err = vfs.CopyFile(fs, file, extensionsDir); err != nil {
				return fmt.Errorf("copying extension file %s: %w", file, err)
			}

			return nil
		}
	}

	if !slices.ContainsFunc(entries, func(entry iofs.DirEntry) bool {
		return entry.Name() == "usr" && entry.IsDir()
	}) {
		return fmt.Errorf("invalid extension: either a single image file or a /usr directory is required")
	}

	sync := rsync.NewRsync(m.system, rsync.WithContext(ctx))
	syncDirectory := func(dirName string) error {
		sourcePath := filepath.Join(tempDir, dirName)
		if exists, _ := vfs.Exists(fs, sourcePath); !exists {
			return nil
		}

		targetPath := filepath.Join(extensionsDir, extension.Name, dirName)
		if err = vfs.MkdirAll(fs, targetPath, 0755); err != nil {
			return fmt.Errorf("creating extension directory /%s: %w", dirName, err)
		}

		if err = sync.SyncData(sourcePath, targetPath); err != nil {
			return fmt.Errorf("syncing extension directory /%s: %w", dirName, err)
		}

		return nil
	}

	if err = syncDirectory("usr"); err != nil {
		return err
	}

	return syncDirectory("opt")
}

func isExtensionExplicitlyEnabled(name string, conf *image.Configuration) bool {
	return slices.ContainsFunc(conf.Release.Components.SystemdExtensions, func(e release.SystemdExtension) bool {
		return e.Name == name
	})
}

func enabledExtensions(rm *resolver.ResolvedManifest, conf *image.Configuration, logger log.Logger) ([]api.SystemdExtension, error) {
	var all, enabled []api.SystemdExtension

	all = append(all, rm.CorePlatform.Components.Systemd.Extensions...)
	if rm.ProductExtension != nil {
		all = append(all, rm.ProductExtension.Components.Systemd.Extensions...)
	}

	var notFound []string
	for _, selected := range conf.Release.Components.SystemdExtensions {
		if !slices.ContainsFunc(all, func(e api.SystemdExtension) bool {
			return e.Name == selected.Name
		}) {
			notFound = append(notFound, selected.Name)
		}
	}

	if len(notFound) > 0 {
		return nil, fmt.Errorf("requested systemd extension(s) not found: %q", notFound)
	}

	charts, _, err := enabledHelmCharts(rm, conf.Release.Components.HelmCharts, nil)
	if err != nil {
		return nil, fmt.Errorf("filtering enabled helm charts: %w", err)
	}

	isDependency := func(extension string) bool {
		return slices.ContainsFunc(charts, func(c *api.HelmChart) bool {
			return slices.ContainsFunc(c.ExtensionDependencies(), func(dependency string) bool {
				return dependency == extension
			})
		})
	}

	for _, ext := range all {
		if ext.Required ||
			isExtensionExplicitlyEnabled(ext.Name, conf) ||
			(ext.Name == k8sExtension && isKubernetesEnabled(conf)) ||
			isDependency(ext.Name) {
			enabled = append(enabled, ext)
		} else {
			logger.Debug("Extension %q not enabled", ext.Name)
		}
	}

	return enabled, nil
}
