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

package v0

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/crypto"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestV0ConfigurationSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configuration V0 test suite")
}

var installYAML = `
schema: v0
bootloader: grub
kernelCmdLine: "console=ttyS0 quiet loglevel=3"
cryptoPolicy: fips
raw:
  diskSize: 35G
iso:
  device: /dev/sda
`

var butaneYAML = `
version: 1.6.0
variant: fcos
`

var kubernetesClusterYAML = `
manifests:
  - https://foo.bar/bar.yaml
helm:
  charts:
    - name: "foo"
      version: "0.0.0"
      targetNamespace: "foo-system"
      repositoryName: "foo-charts"
  repositories:
    - name: "foo-charts"
      url: "https://charts.foo.bar"
      credentials:
        username: cluster-user
        password: cluster-pass
nodes:
  - hostname: node1.foo.bar
    type: server
network:
  apiHost: 192.168.120.100
  apiVIP: 192.168.120.100.sslip.io
`

var releaseYAML = `
name: foo
manifestURI: oci://registry.foo.bar/release-manifest:0.0.1
components:
  systemd:
    - extension: bar
  helm:
    - chart: foo
      valuesFile: foo.yaml
      credentials:
        username: release-user
        password: release-pass
`

var _ = Describe("Configuration", Label("configuration"), func() {
	var configDir Dir = "/tmp/config-dir"
	var fs vfs.FS
	var cleanup func()
	var err error

	BeforeEach(func() {
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			fmt.Sprintf("%s/install.yaml", configDir):                      installYAML,
			fmt.Sprintf("%s/butane.yaml", configDir):                       butaneYAML,
			fmt.Sprintf("%s/kubernetes/cluster.yaml", configDir):           kubernetesClusterYAML,
			fmt.Sprintf("%s/release.yaml", configDir):                      releaseYAML,
			fmt.Sprintf("%s/foo.yaml", configDir.HelmValuesDir()):          "",
			fmt.Sprintf("%s/bar.yaml", configDir.KubernetesManifestsDir()): "",
			fmt.Sprintf("%s/agent.yaml", configDir.KubernetesConfigDir()):  "",
			fmt.Sprintf("%s/server.yaml", configDir.KubernetesConfigDir()): "",
			fmt.Sprintf("%s/node1.foo.yaml", configDir.NetworkDir()):       "",
			fmt.Sprintf("%s/scripts/foo.sh", configDir.CustomDir()):        "",
			fmt.Sprintf("%s/files/foo", configDir.CustomDir()):             "",
		})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Is fully parsed", func() {
		conf, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(conf.Installation.Bootloader).To(Equal("grub"))
		Expect(conf.Installation.KernelCmdLine).To(Equal("console=ttyS0 quiet loglevel=3"))
		Expect(conf.Installation.RAW.DiskSize).To(Equal(install.DiskSize("35G")))
		Expect(conf.Installation.ISO.Device).To(Equal("/dev/sda"))
		Expect(conf.Installation.CryptoPolicy).To(Equal(crypto.FIPSPolicy))

		Expect(conf.Kubernetes.Config.AgentFilePath).To(Equal(configDir.KubernetesAgentFilepath()))
		Expect(conf.Kubernetes.Config.ServerFilePath).To(Equal(configDir.KubernetesServerFilepath()))
		Expect(conf.Kubernetes.Helm).ToNot(BeNil())
		Expect(conf.Kubernetes.Helm.Charts).ToNot(BeNil())
		Expect(conf.Kubernetes.Helm.Charts[0].Name).To(Equal("foo"))
		Expect(conf.Kubernetes.Helm.Charts[0].RepositoryName).To(Equal("foo-charts"))
		Expect(conf.Kubernetes.Helm.Charts[0].TargetNamespace).To(Equal("foo-system"))
		Expect(conf.Kubernetes.Helm.Charts[0].ValuesFile).To(BeEmpty())
		Expect(conf.Kubernetes.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(conf.Kubernetes.Helm.Repositories[0].Name).To(Equal("foo-charts"))
		Expect(conf.Kubernetes.Helm.Repositories[0].URL).To(Equal("https://charts.foo.bar"))
		Expect(conf.Kubernetes.Helm.Repositories[0].Credentials.Username).To(Equal("cluster-user"))
		Expect(conf.Kubernetes.Helm.Repositories[0].Credentials.Password).To(Equal("cluster-pass"))
		Expect(conf.Kubernetes.Nodes[0].Hostname).To(Equal("node1.foo.bar"))
		Expect(conf.Kubernetes.Nodes[0].Type).To(Equal("server"))
		Expect(conf.Kubernetes.Network.APIHost).To(Equal("192.168.120.100"))
		Expect(conf.Kubernetes.Network.APIVIP4).To(Equal("192.168.120.100.sslip.io"))
		Expect(conf.Kubernetes.Network.APIVIP6).To(BeEmpty())

		Expect(conf.Network.ConfigDir).To(Equal(configDir.NetworkDir()))
		Expect(conf.Network.CustomScript).To(BeEmpty())

		Expect(conf.Custom.ScriptsDir).To(Equal(filepath.Join(configDir.CustomDir(), "scripts")))
		Expect(conf.Custom.FilesDir).To(Equal(filepath.Join(configDir.CustomDir(), "files")))

		Expect(conf.Release.Components.SystemdExtensions).ToNot(BeEmpty())
		Expect(conf.Release.Components.SystemdExtensions[0].Name).To(Equal("bar"))
		Expect(len(conf.Release.Components.HelmCharts)).To(Equal(3))
		Expect(conf.Release.Components.HelmCharts[0].Name).To(Equal("foo"))
		Expect(conf.Release.Components.HelmCharts[0].ValuesFile).To(Equal("foo.yaml"))
		Expect(conf.Release.Components.HelmCharts[0].Credentials.Username).To(Equal("release-user"))
		Expect(conf.Release.Components.HelmCharts[0].Credentials.Password).To(Equal("release-pass"))
		Expect(containsChart("metallb", conf.Release.Components.HelmCharts)).To(BeTrue())
		Expect(containsChart("endpoint-copier-operator", conf.Release.Components.HelmCharts)).To(BeTrue())
		Expect(conf.Release.ManifestURI).To(Equal("oci://registry.foo.bar/release-manifest:0.0.1"))
		Expect(conf.Release.Name).To(Equal("foo"))

		Expect(conf.ButaneConfig).NotTo(BeEmpty())
		Expect(conf.ButaneConfig).To(Equal(map[string]any{
			"version": "1.6.0",
			"variant": "fcos",
		}))
	})

	It("Successfully parses relative release manifest URI", func() {
		releaseFile := filepath.Join(string(configDir), "release.yaml")
		releaseYAML := `manifestURI: file://./release-manifest.yaml`

		Expect(fs.Remove(releaseFile)).To(Succeed())
		Expect(fs.WriteFile(releaseFile, []byte(releaseYAML), 0644)).To(Succeed())

		conf, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.Release.ManifestURI).To(Equal("file:/tmp/config-dir/release-manifest.yaml"))
	})

	It("Successfully parses network script", func() {
		Expect(fs.Remove(filepath.Join(configDir.NetworkDir(), "node1.foo.yaml"))).To(Succeed())

		scriptPath := filepath.Join(configDir.NetworkDir(), "configure-network.sh")
		_, err := fs.Create(scriptPath)
		Expect(err).ToNot(HaveOccurred())

		conf, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.Network.ConfigDir).To(BeEmpty())
		Expect(conf.Network.CustomScript).To(Equal(scriptPath))
	})

	It("Fails to parse an empty network directory", func() {
		Expect(fs.Remove(filepath.Join(configDir.NetworkDir(), "node1.foo.yaml"))).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("parsing network directory: network directory is empty"))
	})

	It("Skips custom scripts if custom directory is not present", func() {
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir()))).To(Succeed())

		conf, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.Custom.ScriptsDir).To(BeEmpty())
		Expect(conf.Custom.FilesDir).To(BeEmpty())
	})

	It("Doesn't set custom files directory only if not present", func() {
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir(), "files"))).To(Succeed())

		conf, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.Custom.ScriptsDir).To(Equal(filepath.Join(configDir.CustomDir(), "scripts")))
		Expect(conf.Custom.FilesDir).To(BeEmpty())
	})

	It("Fails to parse an empty custom directory", func() {
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir(), "scripts"))).To(Succeed())
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir(), "files"))).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("parsing custom directory: directory \"/tmp/config-dir/custom\" is empty"))
	})

	It("Fails to parse a custom directory without a scripts subdirectory", func() {
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir(), "scripts"))).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parsing custom directory: "))
		Expect(err.Error()).To(ContainSubstring("/custom/scripts: no such file or directory"))
	})

	It("Fails to parse an empty custom scripts directory", func() {
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir(), "scripts"))).To(Succeed())
		Expect(fs.Mkdir(filepath.Join(configDir.CustomDir(), "scripts"), vfs.DirPerm)).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("parsing custom directory: directory \"/tmp/config-dir/custom/scripts\" is empty")))
	})

	It("Fails to parse an empty custom files directory", func() {
		Expect(fs.RemoveAll(filepath.Join(configDir.CustomDir(), "files"))).To(Succeed())
		Expect(fs.Mkdir(filepath.Join(configDir.CustomDir(), "files"), vfs.DirPerm)).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("parsing custom directory: directory \"/tmp/config-dir/custom/files\" is empty"))
	})

	It("Parses {server,agent}.yaml without manifests subdir", func() {
		Expect(fs.RemoveAll(configDir.KubernetesManifestsDir())).To(Succeed())

		cfg, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(cfg.Kubernetes.Config.ServerFilePath).To(Equal("/tmp/config-dir/kubernetes/config/server.yaml"))
		Expect(cfg.Kubernetes.Config.AgentFilePath).To(Equal("/tmp/config-dir/kubernetes/config/agent.yaml"))
	})

	It("Fails on invalid configuration", func() {
		installFile := filepath.Join(string(configDir), "install.yaml")
		invalidInstallYAML := `
schema: v0
bootloader: invalid
raw:
  diskSize: 35X
`
		Expect(fs.WriteFile(installFile, []byte(invalidInstallYAML), 0644)).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("validating configuration"))
		Expect(err.Error()).To(ContainSubstring("field \"Configuration.Installation.Bootloader\" must be one of [grub none], but got \"invalid\""))
		Expect(err.Error()).To(ContainSubstring("field \"Configuration.Installation.RAW.DiskSize\" must be a valid disk size (e.g., 10G, 500M), but got \"35X\""))
	})

	It("Fails on missing required release configuration", func() {
		releaseFile := filepath.Join(string(configDir), "release.yaml")
		Expect(fs.Remove(releaseFile)).To(Succeed())

		_, err := Parse(fs, configDir)
		Expect(err).To(HaveOccurred())
		// Parse will fail first on reading the file
		Expect(err.Error()).To(ContainSubstring("reading config file"))
	})
})

var _ = Describe("Write", Label("configuration", "write"), func() {
	var configDir Dir = "/tmp/write-config-dir"
	var fs vfs.FS
	var cleanup func()
	var err error

	BeforeEach(func() {
		fs, cleanup, err = sysmock.TestFS(map[string]any{})
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Writes install.yaml and release.yaml", func() {
		conf := &image.Configuration{
			Installation: install.Installation{
				SchemaVersion: "v0",
				Bootloader:    "grub",
				RAW: install.RAW{
					DiskSize: "20G",
				},
			},
			Release: release.Release{
				Name:        "test-product",
				ManifestURI: "oci://registry.example.com/test:latest",
			},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		data, err := fs.ReadFile(configDir.InstallFilepath())
		Expect(err).ToNot(HaveOccurred())

		var parsedInstall install.Installation
		Expect(ParseAny(data, &parsedInstall)).To(Succeed())
		Expect(parsedInstall.SchemaVersion).To(Equal("v0"))
		Expect(parsedInstall.Bootloader).To(Equal("grub"))
		Expect(parsedInstall.RAW.DiskSize).To(Equal(install.DiskSize("20G")))

		data, err = fs.ReadFile(configDir.ReleaseFilepath())
		Expect(err).ToNot(HaveOccurred())

		var parsedRelease release.Release
		Expect(ParseAny(data, &parsedRelease)).To(Succeed())
		Expect(parsedRelease.Name).To(Equal("test-product"))
		Expect(parsedRelease.ManifestURI).To(Equal("oci://registry.example.com/test:latest"))
	})

	It("Writes butane.yaml when ButaneConfig is set", func() {
		conf := &image.Configuration{
			Installation: install.Installation{SchemaVersion: "v0"},
			Release:      release.Release{ManifestURI: "oci://test"},
			ButaneConfig: map[string]any{
				"version": "1.6.0",
				"variant": "fcos",
			},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		data, err := fs.ReadFile(configDir.ButaneFilepath())
		Expect(err).ToNot(HaveOccurred())

		var parsed map[string]any
		Expect(ParseAny(data, &parsed)).To(Succeed())
		Expect(parsed["version"]).To(Equal("1.6.0"))
		Expect(parsed["variant"]).To(Equal("fcos"))
	})

	It("Skips butane.yaml when ButaneConfig is nil", func() {
		conf := &image.Configuration{
			Installation: install.Installation{SchemaVersion: "v0"},
			Release:      release.Release{ManifestURI: "oci://test"},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		exists, _ := vfs.Exists(fs, configDir.ButaneFilepath())
		Expect(exists).To(BeFalse())
	})

	It("Creates network and kubernetes directories", func() {
		conf := &image.Configuration{
			Installation: install.Installation{SchemaVersion: "v0"},
			Release:      release.Release{ManifestURI: "oci://test"},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		info, err := fs.Stat(configDir.NetworkDir())
		Expect(err).ToNot(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())

		info, err = fs.Stat(configDir.kubernetesDir())
		Expect(err).ToNot(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
	})

	It("Writes cluster.yaml when Kubernetes has content", func() {
		conf := &image.Configuration{
			Installation: install.Installation{SchemaVersion: "v0"},
			Release:      release.Release{ManifestURI: "oci://test"},
			Kubernetes: kubernetes.Kubernetes{
				RemoteManifests: []string{"https://example.com/manifest.yaml"},
				Nodes: kubernetes.Nodes{
					{Hostname: "node1.example", Type: "server"},
				},
			},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		data, err := fs.ReadFile(configDir.ClusterFilepath())
		Expect(err).ToNot(HaveOccurred())

		var parsed kubernetes.Kubernetes
		Expect(ParseAny(data, &parsed)).To(Succeed())
		Expect(parsed.RemoteManifests).To(ConsistOf("https://example.com/manifest.yaml"))
		Expect(parsed.Nodes).To(HaveLen(1))
		Expect(parsed.Nodes[0].Hostname).To(Equal("node1.example"))
	})

	It("Skips cluster.yaml when Kubernetes is empty", func() {
		conf := &image.Configuration{
			Installation: install.Installation{SchemaVersion: "v0"},
			Release:      release.Release{ManifestURI: "oci://test"},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		exists, _ := vfs.Exists(fs, configDir.ClusterFilepath())
		Expect(exists).To(BeFalse())
	})

	It("Produces files that can be round-tripped through Parse", func() {
		conf := &image.Configuration{
			Installation: install.Installation{
				SchemaVersion: "v0",
				Bootloader:    "grub",
				CryptoPolicy:  crypto.FIPSPolicy,
				RAW: install.RAW{
					DiskSize: "35G",
				},
			},
			Release: release.Release{
				Name:        "roundtrip-product",
				ManifestURI: "oci://registry.example.com/roundtrip:1.0",
				Components: release.Components{
					SystemdExtensions: []release.SystemdExtension{
						{Name: "rke2"},
					},
					HelmCharts: []release.HelmChart{
						{Name: "test-chart"},
					},
				},
			},
			ButaneConfig: map[string]any{
				"version": "1.6.0",
				"variant": "fcos",
			},
		}

		Expect(Write(fs, configDir, conf)).To(Succeed())

		// Remove empty network dir so Parse doesn't reject it
		Expect(fs.Remove(configDir.NetworkDir())).To(Succeed())

		parsed, err := Parse(fs, configDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(parsed.Installation.SchemaVersion).To(Equal("v0"))
		Expect(parsed.Installation.Bootloader).To(Equal("grub"))
		Expect(parsed.Installation.CryptoPolicy).To(Equal(crypto.FIPSPolicy))
		Expect(parsed.Installation.RAW.DiskSize).To(Equal(install.DiskSize("35G")))
		Expect(parsed.Release.Name).To(Equal("roundtrip-product"))
		Expect(parsed.Release.ManifestURI).To(Equal("oci://registry.example.com/roundtrip:1.0"))
		Expect(parsed.Release.Components.SystemdExtensions).To(HaveLen(1))
		Expect(parsed.Release.Components.SystemdExtensions[0].Name).To(Equal("rke2"))
		Expect(parsed.Release.Components.HelmCharts).To(HaveLen(1))
		Expect(parsed.Release.Components.HelmCharts[0].Name).To(Equal("test-chart"))
		Expect(parsed.ButaneConfig["version"]).To(Equal("1.6.0"))
		Expect(parsed.ButaneConfig["variant"]).To(Equal("fcos"))
	})
})

func containsChart(name string, charts []release.HelmChart) bool {
	return slices.ContainsFunc(charts, func(c release.HelmChart) bool {
		return c.Name == name
	})
}
