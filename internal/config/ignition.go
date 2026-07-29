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

package config

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/cpio"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/extensions"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	ensureSysextUnitName        = "ensure-sysext.service"
	reloadKernelModulesUnitName = "reload-kernel-modules.service"
	updateLinkerCacheUnitName   = "update-linker-cache.service"
	ignitionFileName            = "10-elemental.ign"
	ignitionFromButaneFileName  = "90-butane.ign"
)

var (
	//go:embed templates/ensure-sysext.service
	ensureSysextUnit string

	//go:embed templates/reload-kernel-modules.service
	reloadKernelModulesUnit string

	//go:embed templates/update-linker-cache.service
	updateLinkerCacheUnit string
)

// configureSystem writes the Ignition configuration file including:
// * Predefined Butane configuration
// * Kubernetes configuration and deployment files
// * Systemd extensions
// * Kubernetes distribution installation service
//
// it builds a CPIO file containing the Ignition configuration at /usr/lib/ignition/base.d, then the CPIO file
// is used as an initrd extension allowing the user to provide additional user configuration to be merged on top.
func (m *Manager) configureSystem(ctx context.Context, conf *image.Configuration, output Output, manifest *resolver.ResolvedManifest, ext []api.SystemdExtension) error {
	const (
		variant = "fcos"
		version = "1.6.0"
	)
	var butaneCfg butane.Config

	butaneCfg.Variant = variant
	butaneCfg.Version = version

	k8sEnabled := isKubernetesEnabled(conf)

	if len(conf.ButaneConfig) == 0 &&
		!k8sEnabled &&
		len(ext) == 0 {
		m.system.Logger().Info("No ignition configuration required")
		return nil
	}

	if k8sEnabled {
		err := m.unpackKubernetesArtifacts(ctx, manifest, output)
		if err != nil {
			return fmt.Errorf("unpacking k8s: %w", err)
		}

		err = m.configureKubernetes(ctx, conf, manifest, &butaneCfg)
		if err != nil {
			return err
		}
	}

	if len(ext) > 0 {
		data, err := extensions.Serialize(ext)
		if err != nil {
			return fmt.Errorf("serializing extensions: %w", err)
		}

		butaneCfg.AddFileInline(extensions.File, &data, 0o644)

		butaneCfg.AddSystemdUnit(ensureSysextUnitName, ensureSysextUnit, true)
		butaneCfg.AddSystemdUnit(reloadKernelModulesUnitName, reloadKernelModulesUnit, true)
		butaneCfg.AddSystemdUnit(updateLinkerCacheUnitName, updateLinkerCacheUnit, true)
	}

	return m.writeBaseIgnitionConfig(output, butaneCfg, conf.ButaneConfig)
}

// writeBaseIgnitionConfig renders the generated Ignition configuration including the user provided butane configuration
// as part of a CPIO file, which can be used to extend the OS initrd and include ignition base configuration the expected
// /usr/lib/ignition/base.d path
func (m *Manager) writeBaseIgnitionConfig(output Output, config butane.Config, butaneConfing map[string]any) (err error) {
	tmpDir, err := vfs.TempDir(m.system.FS(), output.RootPath, "initrd")
	if err != nil {
		return fmt.Errorf("creating temporary directory for ignition initrd extension: %w", err)
	}
	defer func() {
		e := m.system.FS().RemoveAll(tmpDir)
		if err == nil && e != nil {
			err = e
		}
	}()

	ignitionFile := filepath.Join(tmpDir, image.IgnitionBaseConfigPath(), ignitionFileName)
	err = butane.WriteIgnitionFile(m.system, config, ignitionFile)
	if err != nil {
		return fmt.Errorf("writing ignition file %q: %w", ignitionFile, err)
	}

	if len(butaneConfing) > 0 {
		m.system.Logger().Info("Translating butane configuration to Ignition syntax")

		butaneFile := filepath.Join(tmpDir, image.IgnitionBaseConfigPath(), ignitionFromButaneFileName)
		err = butane.WriteIgnitionFile(m.system, butaneConfing, butaneFile)
		if err != nil {
			return fmt.Errorf("writing ignition file %q: %w", butaneFile, err)
		}
	}

	return cpio.CreateCPIO(context.Background(), m.system, tmpDir, output.InitrdExtensionFile())
}
