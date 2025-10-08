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
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/template"
)

const (
	ensureSysextUnitName = "systemd-sysext.service"
	k8sResourcesUnitName = "k8s-resource-installer.service"
)

//go:embed templates/systemd-sysext.service
var ensureSysextUnit string

//go:embed templates/k8s-resource-installer.service.tpl
var k8sResourceUnitTpl string

// configureIngition writes the ignition configuration file based on the provided butane configuration
// and the given kubernetes configuration
func (b *Builder) configureIgnition(def *image.Definition, buildDir image.BuildDir, k8sScript string) error {
	if len(def.ButaneConfig) == 0 && k8sScript == "" {
		b.System.Logger().Info("No ignition configuration required")
		return nil
	}

	const (
		variant = "fcos"
		version = "1.6.0"
	)
	var config butane.Config

	config.Variant = variant
	config.Version = version

	if len(def.ButaneConfig) > 0 {
		b.System.Logger().Info("Translating butane configuration to Ignition syntax")

		ignitionBytes, err := butane.TranslateBytes(b.System, def.ButaneConfig)
		if err != nil {
			return fmt.Errorf("failed translating butane configuration: %w", err)
		}
		config.MergeInlineIgnition(string(ignitionBytes))
	} else {
		b.System.Logger().Info("No butane configuration to translate into Ignition syntax")
	}

	if k8sScript != "" {
		k8sResourcesUnit, err := generateK8sResourcesUnit(k8sScript)
		if err != nil {
			return err
		}

		config.AddSystemdUnit(ensureSysextUnitName, ensureSysextUnit, true)
		config.AddSystemdUnit(k8sResourcesUnitName, k8sResourcesUnit, true)
	}

	ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())
	return butane.WriteIgnitionFile(b.System, config, ignitionFile)
}

func generateK8sResourcesUnit(deployScript string) (string, error) {
	values := struct {
		KubernetesDir        string
		ManifestDeployScript string
	}{
		KubernetesDir:        filepath.Dir(deployScript),
		ManifestDeployScript: deployScript,
	}

	data, err := template.Parse(k8sResourcesUnitName, k8sResourceUnitTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing config script template: %w", err)
	}
	return data, nil
}
