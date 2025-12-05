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
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	k8sExtension            = "rke2"
	k8sResDeployScriptName  = "k8s_res_deploy.sh"
	k8sConfDeployScriptName = "k8s_conf_deploy.sh"
)

//go:embed templates/k8s_res_deploy.sh.tpl
var k8sResDeployScriptTpl string

//go:embed templates/k8s_conf_deploy.sh.tpl
var k8sConfDeployScriptTpl string

func needsManifestsSetup(conf *image.Configuration) bool {
	return len(conf.Kubernetes.RemoteManifests) > 0 || len(conf.Kubernetes.LocalManifests) > 0 || conf.Kubernetes.Network.IsHA()
}

func needsHelmChartsSetup(conf *image.Configuration) bool {
	return (len(conf.Release.Components.HelmCharts) > 0) || conf.Kubernetes.Helm != nil
}

func isKubernetesEnabled(conf *image.Configuration) bool {
	return isExtensionExplicitlyEnabled(k8sExtension, conf) || needsHelmChartsSetup(conf) || needsManifestsSetup(conf)
}

func (m *Manager) configureKubernetes(
	ctx context.Context,
	conf *image.Configuration,
	manifest *resolver.ResolvedManifest,
	outputDir OutputDir,
) (k8sResourceScript, k8sConfScript string, err error) {
	if !isKubernetesEnabled(conf) {
		m.system.Logger().Info("Kubernetes is not enabled, skipping configuration")

		return "", "", nil
	}

	var runtimeHelmCharts []string
	if needsHelmChartsSetup(conf) {
		m.system.Logger().Info("Configuring Helm charts")

		runtimeHelmCharts, err = m.helm.Configure(conf, manifest)
		if err != nil {
			return "", "", fmt.Errorf("configuring helm charts: %w", err)
		}
	}

	var runtimeManifestsDir string
	if needsManifestsSetup(conf) {
		m.system.Logger().Info("Configuring Kubernetes manifests")

		runtimeManifestsDir, err = m.setupManifests(ctx, &conf.Kubernetes, outputDir)
		if err != nil {
			return "", "", fmt.Errorf("configuring kubernetes manifests: %w", err)
		}
	}

	if len(runtimeHelmCharts) > 0 || runtimeManifestsDir != "" {
		k8sResourceScript, err = writeK8sResDeployScript(m.system.FS(), outputDir, runtimeManifestsDir, runtimeHelmCharts)
		if err != nil {
			return "", "", fmt.Errorf("writing kubernetes resource deployment script: %w", err)
		}
	}

	k8sConfScript, err = writeK8sConfigDeployScript(m.system.FS(), outputDir, conf.Kubernetes)
	if err != nil {
		return "", "", fmt.Errorf("writing kubernetes resource deployment script: %w", err)
	}

	return k8sResourceScript, k8sConfScript, nil
}

func (m *Manager) setupManifests(ctx context.Context, k *kubernetes.Kubernetes, outputDir OutputDir) (string, error) {
	fs := m.system.FS()

	relativeManifestsPath := filepath.Join("/", image.KubernetesManifestsPath())
	manifestsDir := filepath.Join(outputDir.OverlaysDir(), relativeManifestsPath)

	if err := vfs.MkdirAll(fs, manifestsDir, vfs.DirPerm); err != nil {
		return "", fmt.Errorf("setting up manifests directory '%s': %w", manifestsDir, err)
	}

	for _, manifest := range k.RemoteManifests {
		path := filepath.Join(manifestsDir, filepath.Base(manifest))

		if err := m.downloadFile(ctx, fs, manifest, path); err != nil {
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

func writeK8sResDeployScript(fs vfs.FS, outputDir OutputDir, runtimeManifestsDir string, runtimeHelmCharts []string) (string, error) {

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
	destDir := filepath.Join(outputDir.OverlaysDir(), relativeK8sPath)

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

func writeK8sConfigDeployScript(fs vfs.FS, outputDir OutputDir, k kubernetes.Kubernetes) (string, error) {
	relativeK8sPath := filepath.Join("/", image.KubernetesPath())

	var (
		initNode *kubernetes.Node
		err      error
	)

	if len(k.Nodes) > 0 {
		initNode, err = kubernetes.FindInitNode(k.Nodes)
		if err != nil {
			return "", fmt.Errorf("finding init node: %w", err)
		}
	}

	values := struct {
		Nodes         kubernetes.Nodes
		APIVIP4       string
		APIVIP6       string
		APIHost       string
		KubernetesDir string
		InitNode      kubernetes.Node
	}{
		Nodes:         k.Nodes,
		APIVIP4:       k.Network.APIVIP4,
		APIVIP6:       k.Network.APIVIP6,
		APIHost:       k.Network.APIHost,
		KubernetesDir: relativeK8sPath,
		InitNode:      kubernetes.Node{},
	}

	if initNode != nil {
		values.InitNode = *initNode
	}

	data, err := template.Parse(k8sConfDeployScriptName, k8sConfDeployScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing deployment template: %w", err)
	}

	destDir := filepath.Join(outputDir.OverlaysDir(), relativeK8sPath)

	if err = vfs.MkdirAll(fs, destDir, vfs.DirPerm); err != nil {
		return "", fmt.Errorf("creating destination directory: %w", err)
	}

	fullPath := filepath.Join(destDir, k8sConfDeployScriptName)
	relativePath := filepath.Join(relativeK8sPath, k8sConfDeployScriptName)

	if err = fs.WriteFile(fullPath, []byte(data), 0o744); err != nil {
		return "", fmt.Errorf("writing deployment script %q: %w", fullPath, err)
	}

	return relativePath, nil
}
