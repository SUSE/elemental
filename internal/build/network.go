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

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

//go:embed templates/network.sh.tpl
var configureNetworkScript string

const networkCustomScriptName = "configure-network.sh"

// configureNetwork configures the network component if enabled.
//
// If a custom network configuration script is provided, it will be copied over.
// Otherwise, network configurations will be generated from static definitions
// alongside a configuration script that enables these on first boot.
//
// Example result file layout:
//
//	overlays
//	└── var
//	    └── lib
//	        └── elemental
//	            ├── configure-network.sh
//	            └── network
//	                ├── node1.example.com
//	                │   ├── eth0.nmconnection
//	                │   └── eth1.nmconnection
//	                ├── node2.example.com
//	                │   └── eth0.nmconnection
//	                ├── node3.example.com
//	                │   ├── bond0.nmconnection
//	                │   └── eth1.nmconnection
//	                └── host_config.yaml
func (b *Builder) configureNetwork(def *image.Definition, buildDir image.BuildDir) (string, error) {
	if def.Network.CustomScript == "" && def.Network.ConfigDir == "" {
		b.System.Logger().Info("Network configuration not provided, skipping.")
		return "", nil
	}

	if err := vfs.MkdirAll(b.System.FS(), filepath.Join(buildDir.OverlaysDir(), image.ElementalPath()), vfs.DirPerm); err != nil {
		return "", fmt.Errorf("creating elemental directory in overlays: %w", err)
	}

	relativeScriptPath := filepath.Join("/", image.ElementalPath(), networkCustomScriptName)
	fullScriptPath := filepath.Join(buildDir.OverlaysDir(), relativeScriptPath)

	if def.Network.CustomScript != "" {
		if err := vfs.CopyFile(b.System.FS(), def.Network.CustomScript, fullScriptPath); err != nil {
			return "", fmt.Errorf("copying custom network script: %w", err)
		}

		return relativeScriptPath, nil
	}

	relativeConfigDir := filepath.Join("/", image.NetworkPath())
	fullConfigDir := filepath.Join(buildDir.OverlaysDir(), relativeConfigDir)

	if err := b.generateNetworkConfig(def.Network.ConfigDir, fullConfigDir); err != nil {
		return "", fmt.Errorf("generating network config: %w", err)
	}

	if err := b.writeNetworkConfigurationScript(fullScriptPath, relativeConfigDir); err != nil {
		return "", fmt.Errorf("writing network configuration script: %w", err)
	}

	return relativeScriptPath, nil
}

func (b *Builder) generateNetworkConfig(configDir, outputDir string) error {
	_, err := b.System.Runner().Run("nmc", "generate",
		"--config-dir", configDir,
		"--output-dir", outputDir)

	return err
}

func (b *Builder) writeNetworkConfigurationScript(scriptPath, configDir string) error {
	values := struct {
		ConfigDir string
	}{
		ConfigDir: configDir,
	}

	data, err := template.Parse("network", configureNetworkScript, &values)
	if err != nil {
		return fmt.Errorf("parsing network template: %w", err)
	}

	if err = b.System.FS().WriteFile(scriptPath, []byte(data), 0o744); err != nil {
		return fmt.Errorf("writing network script: %w", err)
	}

	return nil
}
