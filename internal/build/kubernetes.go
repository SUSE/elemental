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
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const k8sExtension = "rke2"

//go:embed templates/k8s_res_deploy.sh.tpl
var k8sResDeployScriptTpl string

func needsManifestsSetup(def *image.Definition) bool {
	return len(def.Kubernetes.RemoteManifests) > 0 || len(def.Kubernetes.LocalManifests) > 0
}

func needsHelmChartsSetup(def *image.Definition) bool {
	return (len(def.Release.Components.HelmCharts) > 0) || def.Kubernetes.Helm != nil
}

func isKubernetesEnabled(def *image.Definition) bool {
	return isExtensionExplicitlyEnabled(k8sExtension, def) || needsHelmChartsSetup(def) || needsManifestsSetup(def)
}

func (b *Builder) configureKubernetes(
	ctx context.Context,
	def *image.Definition,
	manifest *resolver.ResolvedManifest,
	buildDir image.BuildDir,
) (k8sResourceScript string, err error) {
	if !isKubernetesEnabled(def) {
		b.System.Logger().Info("Kubernetes is not enabled, skipping configuration")

		return "", nil
	}

	var runtimeHelmCharts []string
	if needsHelmChartsSetup(def) {
		b.System.Logger().Info("Configuring Helm charts")

		runtimeHelmCharts, err = b.Helm.Configure(def, manifest)
		if err != nil {
			return "", fmt.Errorf("configuring helm charts: %w", err)
		}
	}

	var runtimeManifestsDir string
	if needsManifestsSetup(def) {
		b.System.Logger().Info("Configuring Kubernetes manifests")

		runtimeManifestsDir, err = b.setupManifests(ctx, &def.Kubernetes, buildDir)
		if err != nil {
			return "", fmt.Errorf("configuring kubernetes manifests: %w", err)
		}
	}

	if len(runtimeHelmCharts) > 0 || runtimeManifestsDir != "" {
		k8sResourceScript, err = writeK8sResDeployScript(b.System.FS(), buildDir, runtimeManifestsDir, runtimeHelmCharts)
		if err != nil {
			return "", fmt.Errorf("writing kubernetes resource deployment script: %w", err)
		}
	}

	return k8sResourceScript, nil
}

func (b *Builder) setupManifests(ctx context.Context, k *kubernetes.Kubernetes, buildDir image.BuildDir) (string, error) {
	fs := b.System.FS()

	relativeManifestsPath := filepath.Join("/", image.KubernetesManifestsPath())
	manifestsDir := filepath.Join(buildDir.OverlaysDir(), relativeManifestsPath)

	if err := vfs.MkdirAll(fs, manifestsDir, vfs.DirPerm); err != nil {
		return "", fmt.Errorf("setting up manifests directory '%s': %w", manifestsDir, err)
	}

	for _, manifest := range k.RemoteManifests {
		path := filepath.Join(manifestsDir, filepath.Base(manifest))

		if err := b.DownloadFile(ctx, fs, manifest, path); err != nil {
			return "", fmt.Errorf("downloading remote Kubernetes manifest '%s': %w", manifest, err)
		}
	}

	for _, manifest := range k.LocalManifests {
		overlayPath := filepath.Join(manifestsDir, filepath.Base(manifest))
		if err := vfs.CopyFile(fs, manifest, overlayPath); err != nil {
			return "", fmt.Errorf("copying local manifest '%s' to '%s': %w", manifest, overlayPath, err)
		}
	}

	return relativeManifestsPath, nil
}

func writeK8sResDeployScript(fs vfs.FS, buildDir image.BuildDir, runtimeManifestsDir string, runtimeHelmCharts []string) (string, error) {
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
		return "", fmt.Errorf("parsing deployment template: %w", err)
	}

	relativeK8sPath := filepath.Join("/", image.KubernetesPath())
	destDir := filepath.Join(buildDir.OverlaysDir(), relativeK8sPath)

	if err = vfs.MkdirAll(fs, destDir, vfs.DirPerm); err != nil {
		return "", fmt.Errorf("creating destination directory: %w", err)
	}

	fullPath := filepath.Join(destDir, k8sResDeployScriptName)
	relativePath := filepath.Join(relativeK8sPath, k8sResDeployScriptName)

	if err = fs.WriteFile(fullPath, []byte(data), 0o744); err != nil {
		return "", fmt.Errorf("writing deployment script %q: %w", fullPath, err)
	}

	return relativePath, nil
}
