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
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/crypto"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestConfigurationSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configuration test suite")
}

var installYAML = `
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

var kubernetesYAML = `
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
			fmt.Sprintf("%s/kubernetes.yaml", configDir):                   kubernetesYAML,
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

		Expect(conf.Kubernetes.Config.AgentFilePath).To(Equal(filepath.Join(configDir.KubernetesConfigDir(), "agent.yaml")))
		Expect(conf.Kubernetes.Config.ServerFilePath).To(Equal(filepath.Join(configDir.KubernetesConfigDir(), "server.yaml")))
		Expect(conf.Kubernetes.Helm).ToNot(BeNil())
		Expect(conf.Kubernetes.Helm.Charts).ToNot(BeNil())
		Expect(conf.Kubernetes.Helm.Charts[0].Name).To(Equal("foo"))
		Expect(conf.Kubernetes.Helm.Charts[0].RepositoryName).To(Equal("foo-charts"))
		Expect(conf.Kubernetes.Helm.Charts[0].TargetNamespace).To(Equal("foo-system"))
		Expect(conf.Kubernetes.Helm.Charts[0].ValuesFile).To(BeEmpty())
		Expect(conf.Kubernetes.Helm.Charts[0].Version).To(Equal("0.0.0"))
		Expect(conf.Kubernetes.Helm.Repositories[0].Name).To(Equal("foo-charts"))
		Expect(conf.Kubernetes.Helm.Repositories[0].URL).To(Equal("https://charts.foo.bar"))
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
})

func containsChart(name string, charts []release.HelmChart) bool {
	return slices.ContainsFunc(charts, func(c release.HelmChart) bool {
		return c.Name == name
	})
}
