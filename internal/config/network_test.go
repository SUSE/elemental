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
	var output = Output{
		RootPath: "/_out",
	}

	var m *Manager
	var system *sys.System
	var fs vfs.FS
	var runner *sysmock.Runner
	var cleanup func()
	var err error

	BeforeEach(func() {
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/etc/configure-network.sh": "./some-command", // custom script
			"/etc/nmstate/libvirt.yaml": "libvirt: true",  // nmstate config
			"/etc/nmstate/qemu.yaml":    "qemu: true",     // nmstate config
		})
		Expect(err).ToNot(HaveOccurred())

		runner = sysmock.NewRunner()

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithRunner(runner),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())

		m = NewManager(system, nil)
	})

	AfterEach(func() {
		cleanup()
	})

	It("Skips configuration", func() {
		err := m.configureNetworkOnFirstboot(&image.Configuration{}, Output{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("Fails to copy custom script", func() {
		conf := &image.Configuration{
			Network: image.Network{
				CustomScript: "/etc/custom.sh",
			},
		}

		err := m.configureNetworkOnFirstboot(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("copying custom network script: stat"))
		Expect(err.Error()).To(ContainSubstring("/etc/custom.sh: no such file or directory"))
	})

	It("Successfully copies custom script", func() {
		conf := &image.Configuration{
			Network: image.Network{
				CustomScript: "/etc/configure-network.sh",
			},
		}

		err := m.configureNetworkOnFirstboot(conf, output)
		Expect(err).NotTo(HaveOccurred())

		// Verify script contents
		netDir := filepath.Join(output.CatalystConfigDir(), "network")
		scriptPath := filepath.Join(netDir, "configure-network.sh")
		contents, err := fs.ReadFile(scriptPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("./some-command"))
	})

	It("Fails to copy network directory content", func() {
		nestedDir := "/etc/network/nested"
		Expect(vfs.MkdirAll(fs, nestedDir, vfs.DirPerm)).To(Succeed())

		conf := &image.Configuration{
			Network: image.Network{
				ConfigDir: "/etc/missing",
			},
		}

		err := m.configureNetworkOnFirstboot(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("copying network config: stat"))
		Expect(err.Error()).To(ContainSubstring("/etc/missing: no such file or directory"))

		conf.Network.ConfigDir = "/etc/network"
		err = m.configureNetworkOnFirstboot(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("copying network config: directories under /etc/network are not supported"))
	})

	It("Successfully copies network directory nmstate files", func() {
		conf := &image.Configuration{
			Network: image.Network{
				ConfigDir: "/etc/nmstate",
			},
		}

		err := m.configureNetworkOnFirstboot(conf, output)
		Expect(err).ToNot(HaveOccurred())

		netDir := filepath.Join(output.CatalystConfigDir(), "network")

		libvirt := filepath.Join(netDir, "libvirt.yaml")
		contents, err := fs.ReadFile(libvirt)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("libvirt: true"))

		qemu := filepath.Join(netDir, "qemu.yaml")
		contents, err = fs.ReadFile(qemu)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("qemu: true"))
	})
})
