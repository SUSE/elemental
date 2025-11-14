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
	"bytes"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Ignition configuration", func() {
	const outputDir OutputDir = "/_out"

	var system *sys.System
	var fs vfs.FS
	var cleanup func()
	var err error
	var m *Manager
	var buffer *bytes.Buffer

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/etc/kubernetes/config/server.yaml": "",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(vfs.MkdirAll(fs, string(outputDir), vfs.DirPerm)).To(Succeed())

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())

		m = NewManager(system, nil)
	})

	AfterEach(func() {
		cleanup()
	})

	It("Does no Ignition configuration if data is not provided", func() {
		conf := &image.Configuration{}

		ignitionFile := filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, outputDir, "", "", nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translates given ButaneConfig to an Ignition file as an embedded merge", func() {
		var butaneConf map[string]any

		butaneConfigString := `
version: 1.6.0
variant: fcos
passwd:
  users:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`

		Expect(parseAny([]byte(butaneConfigString), &butaneConf)).To(Succeed())

		conf := &image.Configuration{
			ButaneConfig: butaneConf,
		}

		Expect(err).NotTo(HaveOccurred())

		ignitionFile := filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, outputDir, "", "", nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("merge"))
	})

	It("Configures kubernetes via Ignition with the given k8s script", func() {
		conf := &image.Configuration{}
		ignitionFile := filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath())

		k8sScript := filepath.Join(outputDir.OverlaysDir(), "path/to/k8s/script.sh")
		k8sConfScript := filepath.Join(outputDir.OverlaysDir(), "path/to/k8s/conf_script.sh")

		Expect(m.configureIgnition(conf, outputDir, k8sScript, k8sConfScript, nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).NotTo(ContainSubstring("merge"))
		Expect(ignition).NotTo(ContainSubstring("/etc/elemental/extensions.yaml"))
		Expect(ignition).To(ContainSubstring("Kubernetes Resources Installer"))
		Expect(ignition).To(ContainSubstring("Kubernetes Config Installer"))
	})

	It("Writes systemd extension via Ignition", func() {
		conf := &image.Configuration{}
		ext := []api.SystemdExtension{{Name: "ext1", Image: "ext1-image"}}
		ignitionFile := filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, outputDir, "", "", ext)).To(Succeed())

		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())

		Expect(ignition).To(ContainSubstring("/etc/elemental/extensions.yaml"))
		Expect(ignition).To(ContainSubstring("Reload systemd units"))
		Expect(ignition).To(ContainSubstring("Reload kernel modules"))
		Expect(ignition).NotTo(ContainSubstring("merge"))
		Expect(ignition).NotTo(ContainSubstring("Kubernetes Resources Installer"))
		Expect(ignition).NotTo(ContainSubstring("Kubernetes Config Installer"))
	})

	It("Fails to translate a butaneConfig with a wrong version or variant", func() {
		var butane map[string]any

		butaneConfigString := `
version: 0.0.1
variant: unknown
passwd:
  users:
  - name: pipo
    ssh_authorized_keys:
    - key1
`
		k8sScript := filepath.Join(outputDir.OverlaysDir(), "path/to/k8s/script.sh")
		k8sConfScript := filepath.Join(outputDir.OverlaysDir(), "path/to/k8s/conf_script.sh")

		Expect(parseAny([]byte(butaneConfigString), &butane)).To(Succeed())
		conf := &image.Configuration{
			ButaneConfig: butane,
		}

		ignitionFile := filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(m.configureIgnition(conf, outputDir, k8sScript, k8sConfScript, nil)).To(MatchError(
			ContainSubstring("No translator exists for variant unknown with version"),
		))
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translate a ButaneConfig with unknown keys by ignoring them and throws warning messages", func() {
		var butane map[string]any

		butaneConfigString := `
version: 1.6.0
variant: fcos
passwd:
  usrs:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(parseAny([]byte(butaneConfigString), &butane)).To(Succeed())
		conf := &image.Configuration{
			ButaneConfig: butane,
		}

		ignitionFile := filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath())
		Expect(m.configureIgnition(conf, outputDir, "", "", nil)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("merge"))
		Expect(buffer.String()).To(ContainSubstring("translating Butane to Ignition reported non fatal entries"))
	})
})
