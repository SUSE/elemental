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

package build

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func (b *Builder) downloadSystemExtensions(ctx context.Context, def *image.Definition, rm *resolver.ResolvedManifest, buildDir image.BuildDir) error {
	extensions, err := enabledExtensions(rm, def)
	if err != nil {
		return fmt.Errorf("filtering enabled systemd extensions: %w", err)
	} else if len(extensions) == 0 {
		return nil
	}

	fs := b.System.FS()
	extensionsDir := filepath.Join(buildDir.OverlaysDir(), image.ExtensionsPath())

	if err = vfs.MkdirAll(fs, extensionsDir, 0o700); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}

	for _, extension := range extensions {
		b.System.Logger().Info("Downloading extension %s from %s...",
			extension.Name, extension.Image)

		extensionPath := filepath.Join(extensionsDir, filepath.Base(extension.Image))

		if err = b.DownloadFile(ctx, fs, extension.Image, extensionPath); err != nil {
			return fmt.Errorf("downloading systemd extension %s: %w", extension.Name, err)
		}
	}

	return nil
}

func isExtensionExplicitlyEnabled(name string, def *image.Definition) bool {
	return slices.ContainsFunc(def.Release.Components.SystemdExtensions, func(e release.SystemdExtension) bool {
		return e.Name == name
	})
}

func enabledExtensions(rm *resolver.ResolvedManifest, def *image.Definition) ([]api.SystemdExtension, error) {
	charts, _, err := enabledHelmCharts(rm, def.Release.Components.HelmCharts, nil)
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

	var all, enabled []api.SystemdExtension

	all = append(all, rm.CorePlatform.Components.Systemd.Extensions...)
	if rm.ProductExtension != nil {
		all = append(all, rm.ProductExtension.Components.Systemd.Extensions...)
	}

	for _, ext := range all {
		if ext.Required ||
			isExtensionExplicitlyEnabled(ext.Name, def) ||
			(ext.Name == k8sExtension && isKubernetesEnabled(def)) ||
			isDependency(ext.Name) {
			enabled = append(enabled, ext)
		}
	}

	return enabled, nil
}
