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
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func needsNetworkSetup(conf *image.Configuration) bool {
	return conf.Network.CustomScript != "" || conf.Network.ConfigDir != ""
}

func (m *Manager) configureNetworkOnFirstboot(conf *image.Configuration, output Output) error {
	if !needsNetworkSetup(conf) {
		m.system.Logger().Info("Network configuration not provided, skipping.")
		return nil
	}

	netDir := filepath.Join(output.CatalystConfigDir(), "network")
	if err := vfs.MkdirAll(m.system.FS(), netDir, vfs.DirPerm); err != nil {
		return fmt.Errorf("creating network directory in overlays: %w", err)
	}

	if conf.Network.CustomScript != "" {
		if err := vfs.CopyFile(m.system.FS(), conf.Network.CustomScript, netDir); err != nil {
			return fmt.Errorf("copying custom network script: %w", err)
		}
	} else if err := vfs.CopyDir(m.system.FS(), conf.Network.ConfigDir, netDir, false, nil); err != nil {
		return fmt.Errorf("copying network config: %w", err)
	}
	return nil
}
