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

	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func needsManifestsSetup(k *kubernetes.Kubernetes) bool {
	return len(k.RemoteManifests) > 0 || len(k.LocalManifests) > 0
}

func setupManifests(ctx context.Context, fs vfs.FS, k *kubernetes.Kubernetes, manifestsDir string) error {
	if err := vfs.MkdirAll(fs, manifestsDir, vfs.DirPerm); err != nil {
		return fmt.Errorf("setting up manifests directory '%s': %w", manifestsDir, err)
	}

	for _, manifest := range k.RemoteManifests {
		if err := downloadExtension(ctx, fs, manifest, manifestsDir); err != nil {
			return fmt.Errorf("downloading remote Kubernetes manfiest '%s': %w", manifest, err)
		}
	}

	for _, manifest := range k.LocalManifests {
		overlayPath := filepath.Join(manifestsDir, filepath.Base(manifest))
		if err := vfs.CopyFile(fs, manifest, overlayPath); err != nil {
			return fmt.Errorf("copying local manifest '%s' to '%s': %w", manifest, overlayPath, err)
		}
	}

	return nil
}

func writeK8sResDeployScript(fs vfs.FS, dest, runtimeManifestsDir string, runtimeHelmCharts []string) (path string, err error) {
	const k8sResDeployScriptName = "k8s_res_deploy.sh"

	values := struct {
		HelmCharts   []string
		ManifestsDir string
	}{
		HelmCharts:   runtimeHelmCharts,
		ManifestsDir: runtimeManifestsDir,
	}

	data, err := template.Parse(k8sResDeployScriptName, k8sResDeployScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing template for %s: %w", k8sResDeployScriptName, err)
	}

	filename := filepath.Join(dest, k8sResDeployScriptName)
	if err = fs.WriteFile(filename, []byte(data), 0o744); err != nil {
		return "", fmt.Errorf("writing %s: %w", filename, err)
	}
	return filename, nil
}
