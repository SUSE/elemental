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
	"github.com/suse/elemental/v3/internal/image/os"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

//go:embed templates/config.sh.tpl
var configScriptTpl string

func writeConfigScript(fs vfs.FS, def *image.Definition, destDir, networkScript, k8sResourceDeployScript string) (string, error) {
	const configScriptName = "config.sh"

	values := struct {
		Users                []os.User
		KubernetesDir        string
		ManifestDeployScript string
		NetworkScript        string
	}{
		Users:         def.OperatingSystem.Users,
		NetworkScript: networkScript,
	}

	if k8sResourceDeployScript != "" {
		// TODO: Ensure passing a plain filename is not allowed as the KubernetesDir is used for a cleanup
		values.KubernetesDir = filepath.Dir(k8sResourceDeployScript)
		values.ManifestDeployScript = k8sResourceDeployScript
	}

	data, err := template.Parse(configScriptName, configScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing config script template: %w", err)
	}

	filename := filepath.Join(destDir, configScriptName)
	if err = fs.WriteFile(filename, []byte(data), 0o744); err != nil {
		return "", fmt.Errorf("writing config script: %w", err)
	}
	return filename, nil
}
