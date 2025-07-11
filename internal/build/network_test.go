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
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Network", func() {
	const buildDir image.BuildDir = "/_build"

	var system *sys.System
	var fs vfs.FS
	var runner *sysmock.Runner
	var cleanup func()
	var err error

	BeforeEach(func() {
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/etc/configure-network.sh": "./some-command", // custom script
		})
		Expect(err).ToNot(HaveOccurred())

		runner = sysmock.NewRunner()

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithRunner(runner),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Skips configuration", func() {
		b := &Builder{
			System: system,
		}

		script, err := b.configureNetwork(&image.Definition{}, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(script).To(BeEmpty())
	})

	It("Fails to copy custom script", func() {
		b := &Builder{
			System: system,
		}

		def := &image.Definition{
			Network: image.Network{
				CustomScript: "/etc/custom.sh",
			},
		}

		script, err := b.configureNetwork(def, buildDir)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("copying custom network script: stat"))
		Expect(err.Error()).To(ContainSubstring("/etc/custom.sh: no such file or directory"))

		Expect(script).To(BeEmpty())
	})

	It("Successfully copies custom script", func() {
		b := &Builder{
			System: system,
		}

		def := &image.Definition{
			Network: image.Network{
				CustomScript: "/etc/configure-network.sh",
			},
		}

		script, err := b.configureNetwork(def, buildDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(script).To(Equal("/var/lib/elemental/configure-network.sh"))

		// Verify script contents
		contents, err := fs.ReadFile(filepath.Join(buildDir.OverlaysDir(), script))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("./some-command"))
	})

	It("Fails to generate configuration from static definitions", func() {
		b := &Builder{
			System: system,
		}

		def := &image.Definition{
			Network: image.Network{
				ConfigDir: "/etc/network",
			},
		}

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			return []byte(""), fmt.Errorf("generate error")
		}

		script, err := b.configureNetwork(def, buildDir)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("generating network config: generate error"))
		Expect(script).To(BeEmpty())
	})

	It("Successfully generates configuration from static definitions", func() {
		b := &Builder{
			System: system,
		}

		def := &image.Definition{
			Network: image.Network{
				ConfigDir: "/etc/network",
			},
		}

		script, err := b.configureNetwork(def, buildDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(script).To(Equal("/var/lib/elemental/configure-network.sh"))

		// Verify script contents
		contents, err := fs.ReadFile(filepath.Join(buildDir.OverlaysDir(), script))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(ContainSubstring("/usr/bin/nmc apply --config-dir /var/lib/elemental/network"))
		Expect(string(contents)).To(ContainSubstring("nmcli connection reload"))
	})
})
