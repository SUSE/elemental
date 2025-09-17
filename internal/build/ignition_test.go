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
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/os"
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

	It("Does no ignition configuration if no butaneConfig is provided", func() {
		def := &image.Definition{
			OperatingSystem: os.OperatingSystem{},
		}

		ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translates given butaneConfig to an ignition file", func() {
		var node yaml.Node

		butaneConfigString := `
version: 0.1.0
passwd:
  users:
  - name: user1
    ssh_authorized_keys:
    - key1
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(yaml.Unmarshal([]byte(butaneConfigString), &node)).To(Succeed())

		def := &image.Definition{
			OperatingSystem: os.OperatingSystem{
				ButaneConfig: node,
			},
		}

		ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("$y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A"))
	})

	It("Fails to translate a butaneConfig with a given variant", func() {
		var node yaml.Node

		butaneConfigString := `
variant: fcos
version: 0.1.0
passwd:
  users:
  - name: user1
    ssh_authorized_keys:
    - key1
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(yaml.Unmarshal([]byte(butaneConfigString), &node)).To(Succeed())

		def := &image.Definition{
			OperatingSystem: os.OperatingSystem{
				ButaneConfig: node,
			},
		}

		ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir)).To(MatchError("butaneConfig does nos support 'variant' key"))
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Fails to translate a butaneConfig with a wrong version", func() {
		var node yaml.Node

		butaneConfigString := `
version: 0.0.1
passwd:
  users:
  - name: user1
    ssh_authorized_keys:
    - key1
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(yaml.Unmarshal([]byte(butaneConfigString), &node)).To(Succeed())

		def := &image.Definition{
			OperatingSystem: os.OperatingSystem{
				ButaneConfig: node,
			},
		}

		ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir)).To(MatchError(
			ContainSubstring("No translator exists for variant unifiedcore with version"),
		))
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("Translates a butaneConfig with unknown keys by ignoring them and reporting a warning", func() {
		var node yaml.Node

		butaneConfigString := `
version: 0.1.0
passwd:
  usrs:
  - name: user1
    ssh_authorized_keys:
    - key1
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
		Expect(yaml.Unmarshal([]byte(butaneConfigString), &node)).To(Succeed())

		def := &image.Definition{
			OperatingSystem: os.OperatingSystem{
				ButaneConfig: node,
			},
		}

		ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).NotTo(ContainSubstring("user1"))
		Expect(buffer.String()).To(ContainSubstring("translating Butane to Ignition reported non fatal entries"))
	})

	It("Translates the butaneConfig from an os.yaml to an ignition file", func() {
		osYamlString := `
diskSize: 32G
butaneConfig:
  version: 0.1.0
  passwd:
    users:
    - name: user1
      ssh_authorized_keys:
      - key1
      password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`

		def := &image.Definition{OperatingSystem: os.OperatingSystem{}}
		Expect(image.ParseConfig([]byte(osYamlString), &def.OperatingSystem)).To(Succeed())

		ignitionFile := filepath.Join(buildDir.OverlaysDir(), image.IgnitionFilePath())

		Expect(builder.configureIgnition(def, buildDir)).To(Succeed())
		ok, err := vfs.Exists(system.FS(), ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		ignition, err := system.FS().ReadFile(ignitionFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(ignition).To(ContainSubstring("$y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A"))
	})

	It("Fails if butaneConfig includes non supported fields in unifiedcore variant, luks not supported", func() {
		osYamlString := `
diskSize: 32G
butaneConfig:
  version: 0.1.0
  passwd:
    users:
    - name: user1
      ssh_authorized_keys:
      - key1
      password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
  storage:
    luks:
    - name: static-key-example
      device: /dev/sdb
      key_file:
        inline: REPLACE-THIS-WITH-YOUR-KEY-MATERIAL
`

		def := &image.Definition{OperatingSystem: os.OperatingSystem{}}
		Expect(image.ParseConfig([]byte(osYamlString), &def.OperatingSystem)).To(Succeed())

		Expect(builder.configureIgnition(def, buildDir)).To(MatchError(ContainSubstring("source config is invalid")))
	})
})
