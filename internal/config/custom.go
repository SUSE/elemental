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
	_ "embed"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var (
	//go:embed templates/catalyst-script.sh.tpl
	catalystScript string
)

func (m *Manager) configureCustomScripts(conf *image.Configuration, output Output) error {
	if conf.Custom.ScriptsDir == "" {
		m.system.Logger().Info("Custom configuration scripts not provided, skipping.")
		return nil
	}

	fs := m.system.FS()

	catalystDir := output.CatalystConfigDir()
	if err := vfs.MkdirAll(fs, catalystDir, vfs.DirPerm); err != nil {
		return fmt.Errorf("creating catalyst directory in overlays: %w", err)
	}

	var scripts []string

	appendScript := func(destPath string) error {
		if err := fs.Chmod(destPath, 0o744); err != nil {
			return fmt.Errorf("setting executable permissions to %q: %w", destPath, err)
		}

		scripts = append(scripts, filepath.Base(destPath))
		return nil
	}

	if err := vfs.CopyDir(fs, conf.Custom.ScriptsDir, catalystDir, false, appendScript); err != nil {
		return err
	}

	if err := vfs.CopyDir(fs, conf.Custom.FilesDir, catalystDir, true, nil); err != nil {
		return err
	}

	return m.writeCatalystScript(catalystDir, scripts)
}

func (m *Manager) writeCatalystScript(catalystDir string, scripts []string) error {
	slices.Sort(scripts)

	values := struct {
		Scripts []string
	}{
		Scripts: scripts,
	}

	script, err := template.Parse("catalyst-script", catalystScript, values)
	if err != nil {
		return fmt.Errorf("assembling script: %w", err)
	}

	filename := filepath.Join(catalystDir, "script")
	if err = m.system.FS().WriteFile(filename, []byte(script), 0o744); err != nil {
		return fmt.Errorf("writing script: %w", err)
	}

	m.system.Logger().Info("Catalyst script written")

	return nil
}
