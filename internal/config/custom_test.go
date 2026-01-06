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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Custom", func() {
	var output = Output{
		RootPath: "/_out",
	}

	var m *Manager
	var system *sys.System
	var fs vfs.FS
	var cleanup func()
	var err error

	var catalystScriptPath = filepath.Join(output.CatalystConfigDir(), "script")

	BeforeEach(func() {
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/etc/custom/scripts/01-test.sh":  "./some-command",
			"/etc/custom/scripts/02-print.sh": "echo xyz",
			"/etc/custom/files/foo":           "123",
		})
		Expect(err).ToNot(HaveOccurred())

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())

		m = NewManager(system, nil)
	})

	AfterEach(func() {
		cleanup()
	})

	It("Skips configuration", func() {
		err := m.configureCustomScripts(&image.Configuration{}, Output{})
		Expect(err).NotTo(HaveOccurred())

		Expect(vfs.Exists(fs, catalystScriptPath)).To(BeFalse())
	})

	It("Fails to create catalyst directory", func() {
		tfs, err := sysmock.ReadOnlyTestFS(fs)
		Expect(err).NotTo(HaveOccurred())

		m.system, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())

		conf := &image.Configuration{
			Custom: image.Custom{
				ScriptsDir: "/etc/custom/scripts",
			},
		}

		err = m.configureCustomScripts(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("creating catalyst directory in overlays:")))

		Expect(vfs.Exists(fs, catalystScriptPath)).To(BeFalse())
	})

	It("Fails to copy non-existing scripts path", func() {
		conf := &image.Configuration{
			Custom: image.Custom{
				ScriptsDir: "/etc/non-existing",
			},
		}

		err := m.configureCustomScripts(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("/etc/non-existing: no such file or directory")))

		Expect(vfs.Exists(fs, catalystScriptPath)).To(BeFalse())
	})

	It("Fails to copy invalid custom directory content", func() {
		nestedDir := "/etc/custom/scripts/nested"
		Expect(vfs.MkdirAll(fs, nestedDir, vfs.DirPerm)).To(Succeed())

		conf := &image.Configuration{
			Custom: image.Custom{
				ScriptsDir: "/etc/custom/scripts",
			},
		}

		err = m.configureCustomScripts(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("directories under /etc/custom/scripts are not supported"))

		Expect(vfs.Exists(fs, catalystScriptPath)).To(BeFalse())
	})

	It("Fails to copy non-existing files path", func() {
		conf := &image.Configuration{
			Custom: image.Custom{
				ScriptsDir: "/etc/custom/scripts",
				FilesDir:   "/etc/non-existing",
			},
		}

		err := m.configureCustomScripts(conf, output)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("/etc/non-existing: no such file or directory")))

		Expect(vfs.Exists(fs, catalystScriptPath)).To(BeFalse())
	})

	It("Successfully copies custom scripts and files", func() {
		conf := &image.Configuration{
			Custom: image.Custom{
				ScriptsDir: "/etc/custom/scripts",
				FilesDir:   "/etc/custom/files",
			},
		}

		Expect(m.configureCustomScripts(conf, output)).To(Succeed())

		contents, err := fs.ReadFile(catalystScriptPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(ContainSubstring(`
echo "Running 01-test.sh"
./01-test.sh

echo "Running 02-print.sh"
./02-print.sh`))

		info, err := fs.Stat(catalystScriptPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode()).To(Equal(os.FileMode(0o744)))

		file := filepath.Join(output.CatalystConfigDir(), "01-test.sh")
		contents, err = fs.ReadFile(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("./some-command"))

		info, err = fs.Stat(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode()).To(Equal(os.FileMode(0o744)))

		file = filepath.Join(output.CatalystConfigDir(), "02-print.sh")
		contents, err = fs.ReadFile(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("echo xyz"))

		info, err = fs.Stat(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode()).To(Equal(os.FileMode(0o744)))

		file = filepath.Join(output.CatalystConfigDir(), "foo")
		contents, err = fs.ReadFile(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("123"))

		info, err = fs.Stat(file)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode()).To(Equal(os.FileMode(0o644)))
	})
})
