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

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	k8sResDeployScriptName  = "k8s_res_deploy.sh"
	k8sConfDeployScriptName = "k8s_conf_deploy.sh"
	k8sResourcesUnitName    = "k8s-resource-installer.service"
	k8sConfigUnitName       = "k8s-config-installer.service"
	k8sInstallSh            = "install.sh"
)

var (
	//go:embed templates/k8s-resource-installer.service.tpl
	k8sResourceUnitTpl string

	//go:embed templates/k8s-config-installer.service.tpl
	k8sConfigUnitTpl string

	//go:embed templates/k8s-vip.yaml.tpl
	k8sVIPManifestTpl string

	//go:embed templates/k8s_res_deploy.sh.tpl
	k8sResDeployScriptTpl string

	//go:embed templates/k8s_conf_deploy.sh.tpl
	k8sConfDeployScriptTpl string
)

func needsManifestsSetup(conf *image.Configuration) bool {
	return len(conf.Kubernetes.RemoteManifests) > 0 || len(conf.Kubernetes.LocalManifests) > 0 || conf.Kubernetes.Network.IsManagedInternally()
}

func needsHelmChartsSetup(conf *image.Configuration) bool {
	return (len(conf.Release.Components.HelmCharts) > 0) || conf.Kubernetes.Helm != nil
}

func isKubernetesEnabled(conf *image.Configuration) bool {
	return conf.Release.Components.Kubernetes != nil || needsHelmChartsSetup(conf) || needsManifestsSetup(conf)
}

func (m *Manager) configureKubernetes(
	ctx context.Context,
	conf *image.Configuration,
	manifest *resolver.ResolvedManifest,
	butaneCfg *butane.Config,
) (err error) {
	if manifest == nil ||
		manifest.CorePlatform == nil ||
		manifest.CorePlatform.Components.Kubernetes == nil {
		m.system.Logger().Error("Kubernetes is enabled, but not part of the release")
		return fmt.Errorf("kubernetes release not found")
	}

	var runtimeHelmCharts []string

	if needsHelmChartsSetup(conf) {
		m.system.Logger().Info("Configuring Helm charts")

		runtimeHelmCharts, err = m.helm.Configure(conf, manifest, butaneCfg)
		if err != nil {
			return fmt.Errorf("configuring helm charts: %w", err)
		}
	}

	var runtimeManifestsDir string
	if needsManifestsSetup(conf) {
		m.system.Logger().Info("Configuring Kubernetes manifests")

		runtimeManifestsDir, err = m.setupManifests(ctx, &conf.Kubernetes, butaneCfg)
		if err != nil {
			return fmt.Errorf("configuring kubernetes manifests: %w", err)
		}
	}

	if len(runtimeHelmCharts) > 0 || runtimeManifestsDir != "" {
		err = appendK8sResDeployScript(butaneCfg, runtimeManifestsDir, runtimeHelmCharts)
		if err != nil {
			return fmt.Errorf("generating kubernetes resource deployment script: %w", err)
		}

		err = appendK8sResUnit(conf, butaneCfg)
		if err != nil {
			return fmt.Errorf("generating kubernetes resource deployment unit: %w", err)
		}
	}

	err = appendK8sConfigDeployScript(butaneCfg, conf.Kubernetes)
	if err != nil {
		return fmt.Errorf("generating kubernetes config deployment script: %w", err)
	}

	err = appendRke2Configuration(m.system, butaneCfg, &conf.Kubernetes)
	if err != nil {
		return fmt.Errorf("generating RKE2 configuration: %w", err)
	}

	return nil
}

func (m *Manager) setupManifests(ctx context.Context, k *kubernetes.Kubernetes, butaneCfg *butane.Config) (string, error) {
	fs := m.system.FS()

	relativeManifestsPath := filepath.Join("/", image.KubernetesManifestsPath())

	for _, manifest := range k.RemoteManifests {
		targetPath := filepath.Join(relativeManifestsPath, filepath.Base(manifest))

		rc, err := m.downloader.URLBody(ctx, manifest)
		if err != nil {
			return "", fmt.Errorf("downloading remote Kubernetes manifest '%s': %w", manifest, err)
		}

		err = butaneCfg.AddFileInlineFromReader(targetPath, rc, 0o644)
		if err != nil {
			return "", fmt.Errorf("reading contents for manifest %q: %w", manifest, err)
		}
	}

	for _, manifest := range k.LocalManifests {
		targetPath := filepath.Join(relativeManifestsPath, filepath.Base(manifest))
		mfst, err := fs.Open(manifest)
		if err != nil {
			return "", fmt.Errorf("opening local manifest %q: %w", manifest, err)
		}
		err = butaneCfg.AddFileInlineFromReader(targetPath, mfst, 0o644)
		if err != nil {
			return "", fmt.Errorf("reading local manifest %q: %w", manifest, err)
		}
	}

	return relativeManifestsPath, nil
}

func appendK8sResDeployScript(butaneCfg *butane.Config, runtimeManifestsDir string, runtimeHelmCharts []string) error {
	values := struct {
		HelmCharts   []string
		ManifestsDir string
	}{
		HelmCharts:   runtimeHelmCharts,
		ManifestsDir: runtimeManifestsDir,
	}

	data, err := template.Parse(k8sResDeployScriptName, k8sResDeployScriptTpl, &values)
	if err != nil {
		return fmt.Errorf("parsing deployment template: %w", err)
	}

	relativePath := filepath.Join("/", image.KubernetesPath(), k8sResDeployScriptName)
	butaneCfg.AddFileInline(relativePath, &data, 0o744)
	return nil
}

func appendK8sResUnit(conf *image.Configuration, butaneCfg *butane.Config) error {
	k8sScript := filepath.Join("/", image.KubernetesPath(), k8sResDeployScriptName)
	initHostname := "*"

	if len(conf.Kubernetes.Nodes) > 0 {
		initNode, err := kubernetes.FindInitNode(conf.Kubernetes.Nodes)
		if err != nil {
			return err
		}

		if initNode != nil {
			initHostname = initNode.Hostname
		}
	}

	k8sResourcesUnit, err := generateK8sResourcesUnit(k8sScript, initHostname)
	if err != nil {
		return err
	}

	butaneCfg.AddSystemdUnit(k8sResourcesUnitName, k8sResourcesUnit, true)
	return nil
}

func appendK8sConfigDeployScript(butaneCfg *butane.Config, k kubernetes.Kubernetes) error {
	relativeK8sPath := filepath.Join("/", image.KubernetesPath())
	k8sInstallPath := filepath.Join("/", image.KubernetesInstallPath())

	var (
		initNode *kubernetes.Node
		err      error
	)

	if len(k.Nodes) > 1 {
		initNode, err = kubernetes.FindInitNode(k.Nodes)
		if err != nil {
			return fmt.Errorf("finding init node: %w", err)
		}
	}

	values := struct {
		Nodes         kubernetes.Nodes
		APIVIP4       string
		APIVIP6       string
		APIHost       string
		KubernetesDir string
		InitNode      kubernetes.Node
		InstallPath   string
		InstallScript string
	}{
		Nodes:         k.Nodes,
		APIVIP4:       k.Network.APIVIP4,
		APIVIP6:       k.Network.APIVIP6,
		APIHost:       k.Network.APIHost,
		KubernetesDir: relativeK8sPath,
		InitNode:      kubernetes.Node{},
		InstallPath:   k8sInstallPath,
		InstallScript: filepath.Join(k8sInstallPath, k8sInstallSh),
	}

	if initNode != nil {
		values.InitNode = *initNode
	}

	data, err := template.Parse(k8sConfDeployScriptName, k8sConfDeployScriptTpl, &values)
	if err != nil {
		return fmt.Errorf("parsing deployment template: %w", err)
	}

	relativePath := filepath.Join(relativeK8sPath, k8sConfDeployScriptName)
	butaneCfg.AddFileInline(relativePath, &data, 0o744)
	return nil
}

func generateK8sResourcesUnit(deployScript, initHostname string) (string, error) {
	values := struct {
		KubernetesDir        string
		ManifestDeployScript string
		InitHostname         string
	}{
		KubernetesDir:        filepath.Dir(deployScript),
		ManifestDeployScript: deployScript,
		InitHostname:         initHostname,
	}

	data, err := template.Parse(k8sResourcesUnitName, k8sResourceUnitTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing resources unit template: %w", err)
	}
	return data, nil
}

func generateK8sConfigUnit(deployScript string) (string, error) {
	values := struct {
		ConfigDeployScript string
	}{
		ConfigDeployScript: deployScript,
	}

	data, err := template.Parse(k8sConfigUnitName, k8sConfigUnitTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing config unit template: %w", err)
	}
	return data, nil
}

func kubernetesVIPManifest(k *kubernetes.Kubernetes) (string, error) {
	vars := struct {
		APIAddress4 string
		APIAddress6 string
	}{
		APIAddress4: k.Network.APIVIP4,
		APIAddress6: k.Network.APIVIP6,
	}

	return template.Parse("k8s-vip", k8sVIPManifestTpl, &vars)
}

func appendRke2Configuration(s *sys.System, butaneCfg *butane.Config, k *kubernetes.Kubernetes) error {
	configScript := filepath.Join("/", image.KubernetesPath(), k8sConfDeployScriptName)

	c, err := kubernetes.NewCluster(s, k)
	if err != nil {
		return fmt.Errorf("failed parsing cluster: %w", err)
	}

	k8sConfigUnit, err := generateK8sConfigUnit(configScript)
	if err != nil {
		return fmt.Errorf("failed generating k8s config unit: %w", err)
	}

	butaneCfg.AddSystemdUnit(k8sConfigUnitName, k8sConfigUnit, true)

	k8sPath := filepath.Join("/", image.KubernetesPath())

	serverBytes, err := marshalConfig(c.ServerConfig)
	if err != nil {
		return fmt.Errorf("failed marshaling server config: %w", err)
	}

	butaneCfg.AddFileInline(filepath.Join(k8sPath, "server.yaml"), new(string(serverBytes)), 0o644)

	if c.InitServerConfig != nil {
		initServerBytes, err := marshalConfig(c.InitServerConfig)
		if err != nil {
			return fmt.Errorf("failed marshaling init-server config: %w", err)
		}

		butaneCfg.AddFileInline(filepath.Join(k8sPath, "init.yaml"), new(string(initServerBytes)), 0o644)
	}

	if c.AgentConfig != nil {
		agentBytes, err := marshalConfig(c.AgentConfig)
		if err != nil {
			return fmt.Errorf("failed marshaling agent config: %w", err)
		}

		butaneCfg.AddFileInline(filepath.Join(k8sPath, "agent.yaml"), new(string(agentBytes)), 0o644)
	}

	if c.RegistriesConfig != nil {
		registriesBytes, err := marshalConfig(c.RegistriesConfig)
		if err != nil {
			return fmt.Errorf("failed marshaling agent config: %w", err)
		}

		butaneCfg.AddFileInline(filepath.Join(k8sPath, "registries.yaml"), new(string(registriesBytes)), 0o644)
	}

	if k.Network.IsManagedInternally() {
		manifestsPath := filepath.Join("/", image.KubernetesManifestsPath())

		vip, err := kubernetesVIPManifest(k)
		if err != nil {
			return fmt.Errorf("failed marshaling agent config: %w", err)
		}

		butaneCfg.AddFileInline(filepath.Join(manifestsPath, "k8s-vip.yaml"), new(string(vip)), 0o644)
	}

	return nil
}

func marshalConfig(config map[string]any) ([]byte, error) {
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("serializing kubernetes config: %w", err)
	}

	return data, nil
}

// unpackKubernetesArtifacts extracts Kubernetes distribution artifacts from an OCI image for installation at firstboot.
func (m *Manager) unpackKubernetesArtifacts(ctx context.Context, manifest *resolver.ResolvedManifest, output Output) error {
	k8s := manifest.CorePlatform.Components.Kubernetes
	fs := m.system.FS()

	overlaysDir := filepath.Join(output.OverlaysDir(), image.KubernetesInstallPath())
	installScript := filepath.Join(overlaysDir, k8sInstallSh)

	if err := vfs.MkdirAll(fs, overlaysDir, vfs.DirPerm); err != nil {
		return fmt.Errorf("creating kubernetes artifacts directory: %w", err)
	}

	m.system.Logger().Info("Extracting Kubernetes artifacts from OCI image: %s", k8s.Image)
	if err := m.unpackImage(ctx, k8s.Image, overlaysDir); err != nil {
		return err
	}

	exists, _ := vfs.Exists(fs, installScript)
	if !exists {
		return fmt.Errorf("kubernetes install script %q not found", installScript)
	}

	return nil
}
