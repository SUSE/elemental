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
	"bytes"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Ignition configuration", func() {
	const buildDir image.BuildDir = "/_build"

	var system *sys.System
	var fs vfs.FS
	var cleanup func()
	var err error
	var builder *Builder
	var buffer *bytes.Buffer

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())

		Expect(vfs.MkdirAll(fs, string(buildDir), vfs.DirPerm)).To(Succeed())

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())
		builder = &Builder{
			System: system,
		}
	})

	AfterEach(func() {
		cleanup()
	})

	It("Does no ignition configuration if no ButaneConfig or Kubernetes setup is provided", func() {
		def := &image.Definition{}

		ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir, "")).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translates given ButaneConfig to an ignition file as an embedded merge", func() {
		var butaneConf map[string]any

		butaneConfigString := `
version: 1.6.0
variant: fcos
passwd:
  users:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`

		Expect(image.ParseConfig([]byte(butaneConfigString), &butaneConf)).To(Succeed())

		def := &image.Definition{
			ButaneConfig: butaneConf,
		}

		Expect(err).NotTo(HaveOccurred())

		ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir, "")).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("merge"))
	})

	It("Configures kubernetes via ignition with the given k8s script", func() {
		def := &image.Definition{}
		ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())

		k8sScript := filepath.Join(buildDir.OverlaysDir(), "path/to/k8s/script.sh")

		Expect(builder.configureIgnition(def, buildDir, k8sScript)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).NotTo(ContainSubstring("merge"))
		Expect(ignition).To(ContainSubstring("Kubernetes Resources Installer"))
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
		k8sScript := filepath.Join(buildDir.OverlaysDir(), "path/to/k8s/script.sh")

		Expect(image.ParseConfig([]byte(butaneConfigString), &butane)).To(Succeed())
		def := &image.Definition{
			ButaneConfig: butane,
		}

		ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir, k8sScript)).To(MatchError(
			ContainSubstring("No translator exists for variant unknown with version"),
		))
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translate a ButaneConfig with unknown keys by ignoring them and throws warning messages", func() {
		var butane map[string]any

		butaneConfigString := `
version: 1.1.0
variant: flatcar
passwd:
  usrs:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(image.ParseConfig([]byte(butaneConfigString), &butane)).To(Succeed())
		def := &image.Definition{
			ButaneConfig: butane,
		}

		ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())
		Expect(builder.configureIgnition(def, buildDir, "")).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("merge"))
		Expect(buffer.String()).To(ContainSubstring("translating Butane to Ignition reported non fatal entries"))
	})
})
