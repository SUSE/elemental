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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/os"
	"github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Config tests", func() {
	const buildDir = "/_build"

	var fs vfs.FS
	var cleanup func()
	var err error

	BeforeEach(func() {
		fs, cleanup, err = mock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(vfs.MkdirAll(fs, buildDir, vfs.DirPerm)).To(Succeed())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Fails writing config script to the FS", func() {
		fs, err = mock.ReadOnlyTestFS(fs)
		Expect(err).NotTo(HaveOccurred())

		definition := &image.Definition{}

		script, err := writeConfigScript(fs, definition, buildDir, "", "")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("writing config script: WriteFile /_build/config.sh: operation not permitted"))
		Expect(script).To(BeEmpty())
	})

	It("Succeeds writing config script to the filesystem without Kubernetes manifests", func() {
		definition := &image.Definition{
			OperatingSystem: os.OperatingSystem{
				Users: []os.User{
					{Username: "root", Password: "linux"},
					{Username: "suse", Password: "suse"},
				},
			},
		}

		script, err := writeConfigScript(fs, definition, buildDir, "", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(script).To(Equal("/_build/config.sh"))

		info, err := fs.Stat(script)
		Expect(err).NotTo(HaveOccurred())
		Expect(int(info.Mode())).To(Equal(0o744), "Verifying permissions")

		b, err := fs.ReadFile(script)
		Expect(err).NotTo(HaveOccurred(), "Reading contents")

		contents := string(b)

		// Users
		Expect(contents).To(ContainSubstring("useradd -m root || true"))
		Expect(contents).To(ContainSubstring("echo 'root:linux' | chpasswd"))
		Expect(contents).To(ContainSubstring("useradd -m suse || true"))
		Expect(contents).To(ContainSubstring("echo 'suse:suse' | chpasswd"))

		// Kubernetes
		Expect(contents).NotTo(ContainSubstring("/etc/systemd/system/k8s-resource-installer.service"))
	})

	It("Succeeds writing config script to the filesystem with Kubernetes manifests", func() {
		definition := &image.Definition{
			OperatingSystem: os.OperatingSystem{
				Users: []os.User{
					{Username: "root", Password: "linux"},
					{Username: "suse", Password: "suse"},
				},
			},
		}

		script, err := writeConfigScript(fs, definition, buildDir, "/var/lib/elemental/configure-network.sh", "/var/lib/elemental/k8s-resources.sh")
		Expect(err).NotTo(HaveOccurred())
		Expect(script).To(Equal("/_build/config.sh"))

		info, err := fs.Stat(script)
		Expect(err).NotTo(HaveOccurred())
		Expect(int(info.Mode())).To(Equal(0o744), "Verifying permissions")

		b, err := fs.ReadFile(script)
		Expect(err).NotTo(HaveOccurred(), "Reading contents")

		contents := string(b)

		// Users
		Expect(contents).To(ContainSubstring("useradd -m root || true"))
		Expect(contents).To(ContainSubstring("echo 'root:linux' | chpasswd"))
		Expect(contents).To(ContainSubstring("useradd -m suse || true"))
		Expect(contents).To(ContainSubstring("echo 'suse:suse' | chpasswd"))

		// Kubernetes
		Expect(contents).To(ContainSubstring("/etc/systemd/system/k8s-resource-installer.service"))
		Expect(contents).To(ContainSubstring("ExecStart=/bin/bash \"/var/lib/elemental/k8s-resources.sh\""))
		Expect(contents).To(ContainSubstring("ExecStartPost=/bin/sh -c 'rm -rf \"/var/lib/elemental\""))

		// Network
		Expect(contents).To(ContainSubstring("ExecStart=/bin/bash \"/var/lib/elemental/configure-network.sh\""))
	})
})
