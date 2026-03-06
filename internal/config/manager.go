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
	iofs "io/fs"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/coreos/butane/base/v0_6"
	"github.com/coreos/ignition/v2/config/util"
	"gopkg.in/yaml.v3"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/internal/template"

	"github.com/suse/elemental/v3/pkg/extensions"
	"github.com/suse/elemental/v3/pkg/extractor"
	"github.com/suse/elemental/v3/pkg/http"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

type downloadFunc func(ctx context.Context, fs vfs.FS, url, path string) error

type helmConfigurator interface {
	Configure(conf *image.Configuration, manifest *resolver.ResolvedManifest) ([]string, error)
}

type releaseManifestResolver interface {
	Resolve(uri string) (*resolver.ResolvedManifest, error)
}

type Manager struct {
	system *sys.System
	local  bool

	rmResolver   releaseManifestResolver
	downloadFile downloadFunc
	helm         helmConfigurator
}

type Opts func(m *Manager)

func WithManifestResolver(r releaseManifestResolver) Opts {
	return func(m *Manager) {
		m.rmResolver = r
	}
}

func WithDownloadFunc(d downloadFunc) Opts {
	return func(m *Manager) {
		m.downloadFile = d
	}
}

func WithLocal(local bool) Opts {
	return func(m *Manager) {
		m.local = local
	}
}

func NewManager(sys *sys.System, helm helmConfigurator, opts ...Opts) *Manager {
	m := &Manager{
		system: sys,
		helm:   helm,
	}

	for _, o := range opts {
		o(m)
	}

	if m.downloadFile == nil {
		m.downloadFile = http.DownloadFile
	}

	return m
}

// ConfigureComponents configures the components defined in the provided configuration
// and returns the resolved release manifest from said configuration.
func (m *Manager) ConfigureComponents(ctx context.Context, conf *image.Configuration, output image.Output) (rm *resolver.ResolvedManifest, err error) {
	if m.rmResolver == nil {
		defaultResolver, err := defaultManifestResolver(m.system.FS(), output, m.local)
		if err != nil {
			return nil, fmt.Errorf("using default release manifest resolver: %w", err)
		}
		m.rmResolver = defaultResolver
	}

	rm, err = m.rmResolver.Resolve(conf.Release.ManifestURI)
	if err != nil {
		return nil, fmt.Errorf("resolving release manifest at uri '%s': %w", conf.Release.ManifestURI, err)
	}

	if err = m.configureNetworkOnFirstboot(conf, output); err != nil {
		return nil, fmt.Errorf("configuring network: %w", err)
	}

	if err = m.ConfigureCustomScripts(conf, output); err != nil {
		return nil, fmt.Errorf("configuring custom scripts: %w", err)
	}

	k8sScript, k8sConfScript, err := m.ConfigureKubernetes(ctx, conf, rm, output)
	if err != nil {
		return nil, fmt.Errorf("configuring kubernetes: %w", err)
	}

	extensions, err := EnabledExtensions(rm, conf, m.system.Logger())
	if err != nil {
		return nil, fmt.Errorf("filtering enabled systemd extensions: %w", err)
	}

	if len(extensions) != 0 {
		if err = m.downloadSystemExtensions(ctx, extensions, output); err != nil {
			return nil, fmt.Errorf("downloading system extensions: %w", err)
		}
	}

	if err = m.ConfigureIgnition(conf, output, k8sScript, k8sConfScript, extensions); err != nil {
		return nil, fmt.Errorf("configuring ignition: %w", err)
	}

	return rm, nil
}

func defaultManifestResolver(fs vfs.FS, out image.Output, local bool) (res *resolver.Resolver, err error) {
	const (
		globPattern = "release_manifest*.yaml"
	)

	searchPaths := []string{
		globPattern,
		filepath.Join("etc", "release-manifest", globPattern),
	}

	manifestsDir := out.ReleaseManifestsStoreDir()
	if err := vfs.MkdirAll(fs, manifestsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating release manifest store '%s': %w", manifestsDir, err)
	}

	extr, err := extractor.New(searchPaths, extractor.WithStore(manifestsDir), extractor.WithLocal(local))
	if err != nil {
		return nil, fmt.Errorf("initialising OCI release manifest extractor: %w", err)
	}

	return resolver.New(source.NewReader(extr)), nil
}

func needsNetworkSetup(conf *image.Configuration) bool {
	return conf.Network.CustomScript != "" || conf.Network.ConfigDir != ""
}

func (m *Manager) configureNetworkOnFirstboot(conf *image.Configuration, output image.Output) error {
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

var (
	//go:embed templates/catalyst-script.sh.tpl
	catalystScript string
)

func (m *Manager) ConfigureCustomScripts(conf *image.Configuration, output image.Output) error {
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

const (
	ensureSysextUnitName        = "ensure-sysext.service"
	reloadKernelModulesUnitName = "reload-kernel-modules.service"
	updateLinkerCacheUnitName   = "update-linker-cache.service"
	k8sResourcesUnitName        = "k8s-resource-installer.service"
	k8sConfigUnitName           = "k8s-config-installer.service"
)

var (
	//go:embed templates/ensure-sysext.service
	ensureSysextUnit string

	//go:embed templates/reload-kernel-modules.service
	reloadKernelModulesUnit string

	//go:embed templates/update-linker-cache.service
	updateLinkerCacheUnit string

	//go:embed templates/k8s-resource-installer.service.tpl
	k8sResourceUnitTpl string

	//go:embed templates/k8s-config-installer.service.tpl
	k8sConfigUnitTpl string

	//go:embed templates/k8s-vip.yaml.tpl
	k8sVIPManifestTpl string
)

// ConfigureIgnition writes the Ignition configuration file including:
// * Predefined Butane configuration
// * Kubernetes configuration and deployment files
// * Systemd extensions
func (m *Manager) ConfigureIgnition(conf *image.Configuration, output image.Output, k8sScript, k8sConfScript string, ext []api.SystemdExtension) error {
	if len(conf.ButaneConfig) == 0 &&
		k8sScript == "" &&
		k8sConfScript == "" &&
		len(ext) == 0 {
		m.system.Logger().Info("No ignition configuration required")
		return nil
	}

	const (
		variant = "fcos"
		version = "1.6.0"
	)
	var config butane.Config

	config.Variant = variant
	config.Version = version

	if len(conf.ButaneConfig) > 0 {
		m.system.Logger().Info("Translating butane configuration to Ignition syntax")

		ignitionBytes, err := butane.TranslateBytes(m.system, conf.ButaneConfig)
		if err != nil {
			return fmt.Errorf("failed translating butane configuration: %w", err)
		}
		config.MergeInlineIgnition(string(ignitionBytes))
	} else {
		m.system.Logger().Info("No butane configuration to translate into Ignition syntax")
	}

	if k8sScript != "" {
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

		config.AddSystemdUnit(k8sResourcesUnitName, k8sResourcesUnit, true)
	}

	if k8sConfScript != "" {
		err := appendRke2Configuration(m.system, &config, &conf.Kubernetes, k8sConfScript)
		if err != nil {
			return fmt.Errorf("failed appending rke2 configuration: %w", err)
		}
	}

	if len(ext) > 0 {
		data, err := extensions.Serialize(ext)
		if err != nil {
			return fmt.Errorf("serializing extensions: %w", err)
		}

		config.Storage.Files = append(config.Storage.Files, v0_6.File{
			Path:     extensions.File,
			Contents: v0_6.Resource{Inline: util.StrToPtr(data)},
		})

		config.AddSystemdUnit(ensureSysextUnitName, ensureSysextUnit, true)
		config.AddSystemdUnit(reloadKernelModulesUnitName, reloadKernelModulesUnit, true)
		config.AddSystemdUnit(updateLinkerCacheUnitName, updateLinkerCacheUnit, true)
	}

	ignitionFile := filepath.Join(output.FirstbootConfigDir(), image.IgnitionFilePath())
	return butane.WriteIgnitionFile(m.system, config, ignitionFile)
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
		return "", fmt.Errorf("parsing config script template: %w", err)
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
		return "", fmt.Errorf("parsing config script template: %w", err)
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

func appendRke2Configuration(s *sys.System, config *butane.Config, k *kubernetes.Kubernetes, configScript string) error {
	c, err := kubernetes.NewCluster(s, k)
	if err != nil {
		return fmt.Errorf("failed parsing cluster: %w", err)
	}

	k8sConfigUnit, err := generateK8sConfigUnit(configScript)
	if err != nil {
		return fmt.Errorf("failed generating k8s config unit: %w", err)
	}

	config.AddSystemdUnit(k8sConfigUnitName, k8sConfigUnit, true)

	k8sPath := filepath.Join("/", image.KubernetesPath())

	serverBytes, err := marshalConfig(c.ServerConfig)
	if err != nil {
		return fmt.Errorf("failed marshaling server config: %w", err)
	}

	config.Storage.Files = append(config.Storage.Files, v0_6.File{
		Path:     filepath.Join(k8sPath, "server.yaml"),
		Contents: v0_6.Resource{Inline: util.StrToPtr(string(serverBytes))},
	})

	if c.InitServerConfig != nil {
		initServerBytes, err := marshalConfig(c.InitServerConfig)
		if err != nil {
			return fmt.Errorf("failed marshaling init-server config: %w", err)
		}

		config.Storage.Files = append(config.Storage.Files, v0_6.File{
			Path:     filepath.Join(k8sPath, "init.yaml"),
			Contents: v0_6.Resource{Inline: util.StrToPtr(string(initServerBytes))},
		})
	}

	if c.AgentConfig != nil {
		agentBytes, err := marshalConfig(c.AgentConfig)
		if err != nil {
			return fmt.Errorf("failed marshaling agent config: %w", err)
		}

		config.Storage.Files = append(config.Storage.Files, v0_6.File{
			Path:     filepath.Join(k8sPath, "agent.yaml"),
			Contents: v0_6.Resource{Inline: util.StrToPtr(string(agentBytes))},
		})
	}

	if k.Network.APIVIP4 != "" || k.Network.APIVIP6 != "" {
		manifestsPath := filepath.Join("/", image.KubernetesManifestsPath())

		vip, err := kubernetesVIPManifest(k)
		if err != nil {
			return fmt.Errorf("failed marshaling agent config: %w", err)
		}

		config.Storage.Files = append(config.Storage.Files, v0_6.File{
			Path:     filepath.Join(manifestsPath, "k8s-vip.yaml"),
			Contents: v0_6.Resource{Inline: util.StrToPtr(string(vip))},
		})
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

const (
	k8sExtension            = "rke2"
	k8sResDeployScriptName  = "k8s_res_deploy.sh"
	k8sConfDeployScriptName = "k8s_conf_deploy.sh"
)

//go:embed templates/k8s_res_deploy.sh.tpl
var k8sResDeployScriptTpl string

//go:embed templates/k8s_conf_deploy.sh.tpl
var k8sConfDeployScriptTpl string

func NeedsManifestsSetup(conf *image.Configuration) bool {
	return len(conf.Kubernetes.RemoteManifests) > 0 || len(conf.Kubernetes.LocalManifests) > 0 || conf.Kubernetes.Network.IsHA()
}

func NeedsHelmChartsSetup(conf *image.Configuration) bool {
	return (len(conf.Release.Components.HelmCharts) > 0) || conf.Kubernetes.Helm != nil
}

func IsKubernetesEnabled(conf *image.Configuration) bool {
	return isExtensionExplicitlyEnabled(k8sExtension, conf) || NeedsHelmChartsSetup(conf) || NeedsManifestsSetup(conf)
}

func (m *Manager) ConfigureKubernetes(
	ctx context.Context,
	conf *image.Configuration,
	manifest *resolver.ResolvedManifest,
	output image.Output,
) (k8sResourceScript, k8sConfScript string, err error) {
	if !IsKubernetesEnabled(conf) {
		m.system.Logger().Info("Kubernetes is not enabled, skipping configuration")

		return "", "", nil
	}

	var runtimeHelmCharts []string
	if NeedsHelmChartsSetup(conf) {
		m.system.Logger().Info("Configuring Helm charts")

		runtimeHelmCharts, err = m.helm.Configure(conf, manifest)
		if err != nil {
			return "", "", fmt.Errorf("configuring helm charts: %w", err)
		}
	}

	var runtimeManifestsDir string
	if NeedsManifestsSetup(conf) {
		m.system.Logger().Info("Configuring Kubernetes manifests")

		runtimeManifestsDir, err = m.setupManifests(ctx, &conf.Kubernetes, output)
		if err != nil {
			return "", "", fmt.Errorf("configuring kubernetes manifests: %w", err)
		}
	}

	if len(runtimeHelmCharts) > 0 || runtimeManifestsDir != "" {
		k8sResourceScript, err = writeK8sResDeployScript(m.system.FS(), output, runtimeManifestsDir, runtimeHelmCharts)
		if err != nil {
			return "", "", fmt.Errorf("writing kubernetes resource deployment script: %w", err)
		}
	}

	k8sConfScript, err = writeK8sConfigDeployScript(m.system.FS(), output, conf.Kubernetes)
	if err != nil {
		return "", "", fmt.Errorf("writing kubernetes resource deployment script: %w", err)
	}

	return k8sResourceScript, k8sConfScript, nil
}

func (m *Manager) setupManifests(ctx context.Context, k *kubernetes.Kubernetes, output image.Output) (string, error) {
	fs := m.system.FS()

	relativeManifestsPath := filepath.Join("/", image.KubernetesManifestsPath())
	manifestsDir := filepath.Join(output.OverlaysDir(), relativeManifestsPath)

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

func writeK8sResDeployScript(fs vfs.FS, output image.Output, runtimeManifestsDir string, runtimeHelmCharts []string) (string, error) {

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
	destDir := filepath.Join(output.OverlaysDir(), relativeK8sPath)

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

func writeK8sConfigDeployScript(fs vfs.FS, output image.Output, k kubernetes.Kubernetes) (string, error) {
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

	destDir := filepath.Join(output.OverlaysDir(), relativeK8sPath)

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

func (m *Manager) downloadSystemExtensions(ctx context.Context, extensions []api.SystemdExtension, output image.Output) error {
	logger := m.system.Logger()
	fs := m.system.FS()
	extensionsDir := filepath.Join(output.OverlaysDir(), image.ExtensionsPath())

	if err := vfs.MkdirAll(fs, extensionsDir, 0o700); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}

	for _, extension := range extensions {
		logger.Info("Pulling extension %s from %s...",
			extension.Name, extension.Image)

		if IsRemoteURL(extension.Image) {
			extensionPath := filepath.Join(extensionsDir, filepath.Base(extension.Image))
			if err := m.downloadFile(ctx, fs, extension.Image, extensionPath); err != nil {
				return fmt.Errorf("downloading systemd extension %s: %w", extension.Name, err)
			}

			continue
		}

		if err := m.unpackExtension(ctx, extension, extensionsDir); err != nil {
			return fmt.Errorf("unpacking systemd extension %s: %w", extension.Name, err)
		}
	}

	return nil
}

func IsRemoteURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

func (m *Manager) unpackExtension(ctx context.Context, extension api.SystemdExtension, extensionsDir string) error {
	fs := m.system.FS()

	tempDir, err := vfs.TempDir(fs, "", fmt.Sprintf("%s-", extension.Name))
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		_ = fs.RemoveAll(tempDir)
	}()

	unpacker := unpack.NewOCIUnpacker(m.system, extension.Image, unpack.WithLocalOCI(m.local))
	if _, err = unpacker.Unpack(ctx, tempDir); err != nil {
		return fmt.Errorf("unpacking extension: %w", err)
	}

	entries, err := fs.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("reading unpacked directory: %w", err)
	}

	if len(entries) == 1 {
		entry := entries[0]
		if !entry.IsDir() {
			file := filepath.Join(tempDir, entry.Name())
			if err = vfs.CopyFile(fs, file, extensionsDir); err != nil {
				return fmt.Errorf("copying extension file %s: %w", file, err)
			}

			return nil
		}
	}

	if !slices.ContainsFunc(entries, func(entry iofs.DirEntry) bool {
		return entry.Name() == "usr" && entry.IsDir()
	}) {
		return fmt.Errorf("invalid extension: either a single image file or a /usr directory is required")
	}

	sync := rsync.NewRsync(m.system, rsync.WithContext(ctx))
	syncDirectory := func(dirName string) error {
		sourcePath := filepath.Join(tempDir, dirName)
		if exists, _ := vfs.Exists(fs, sourcePath); !exists {
			return nil
		}

		targetPath := filepath.Join(extensionsDir, extension.Name, dirName)
		if err = vfs.MkdirAll(fs, targetPath, 0755); err != nil {
			return fmt.Errorf("creating extension directory /%s: %w", dirName, err)
		}

		if err = sync.SyncData(sourcePath, targetPath); err != nil {
			return fmt.Errorf("syncing extension directory /%s: %w", dirName, err)
		}

		return nil
	}

	if err = syncDirectory("usr"); err != nil {
		return err
	}

	return syncDirectory("opt")
}

func isExtensionExplicitlyEnabled(name string, conf *image.Configuration) bool {
	return slices.ContainsFunc(conf.Release.Components.SystemdExtensions, func(e release.SystemdExtension) bool {
		return e.Name == name
	})
}

func EnabledExtensions(rm *resolver.ResolvedManifest, conf *image.Configuration, logger log.Logger) ([]api.SystemdExtension, error) {
	var all, enabled []api.SystemdExtension

	all = append(all, rm.CorePlatform.Components.Systemd.Extensions...)
	if rm.ProductExtension != nil {
		all = append(all, rm.ProductExtension.Components.Systemd.Extensions...)
	}

	var notFound []string
	for _, selected := range conf.Release.Components.SystemdExtensions {
		if !slices.ContainsFunc(all, func(e api.SystemdExtension) bool {
			return e.Name == selected.Name
		}) {
			notFound = append(notFound, selected.Name)
		}
	}

	if len(notFound) > 0 {
		return nil, fmt.Errorf("requested systemd extension(s) not found: %q", notFound)
	}

	charts, _, err := enabledHelmCharts(rm, conf.Release.Components.HelmCharts)
	if err != nil {
		return nil, fmt.Errorf("filtering enabled helm charts: %w", err)
	}

	isDependency := func(extension string) bool {
		return slices.ContainsFunc(charts, func(c *api.HelmChart) bool {
			return slices.ContainsFunc(c.ExtensionDependencies(), func(dependency string) bool {
				return dependency == extension
			})
		})
	}

	for _, ext := range all {
		if ext.Required ||
			isExtensionExplicitlyEnabled(ext.Name, conf) ||
			(ext.Name == k8sExtension && IsKubernetesEnabled(conf)) ||
			isDependency(ext.Name) {
			enabled = append(enabled, ext)
		} else {
			logger.Debug("Extension '%s' not enabled", ext.Name)
		}
	}

	return enabled, nil
}
