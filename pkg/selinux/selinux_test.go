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

package selinux_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/selinux"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestSELinuxSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SELinux test suite")
}

var _ = Describe("Selinux", Label("selinux"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var syscall *sysmock.Syscall
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var root string
	var contextFile string
	var buffer *bytes.Buffer

	BeforeEach(func() {
		var err error
		syscall = &sysmock.Syscall{}
		buffer = &bytes.Buffer{}
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		logger := log.New(log.WithBuffer(buffer))
		logger.SetLevel(log.DebugLevel())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(logger),
			sys.WithSyscall(syscall),
		)
		Expect(err).NotTo(HaveOccurred())
		root = "/some/root"
		Expect(vfs.MkdirAll(fs, root, vfs.DirPerm)).To(Succeed())
		contextFile = filepath.Join(root, selinux.SelinuxTargetedContextFile)
		Expect(vfs.MkdirAll(fs, filepath.Dir(contextFile), vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, filepath.Dir(selinux.SelinuxTargetedContextFile), vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("relabels the given paths for the targeted context", func() {
		Expect(fs.WriteFile(contextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(selinux.Relabel(context.Background(), s, root)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"setfiles", "-i", "-F", "-r", "/some/root",
			"/some/root/etc/selinux/targeted/contexts/files/file_contexts", "/some/root",
		}})).To(Succeed())
	})
	It("does nothing if the context is not found", func() {
		Expect(selinux.Relabel(context.Background(), s, root)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{}}))
		Expect(buffer.String()).To(ContainSubstring("no context found"))
	})
	It("errors out if setfiles call fails", func() {
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd == "setfiles" {
				return []byte{}, fmt.Errorf("setfiles failed")
			}
			return []byte{}, nil
		}
		Expect(fs.WriteFile(contextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(selinux.Relabel(context.Background(), s, root)).NotTo(Succeed())
	})
	It("relabels the given paths for the targeted context in a chroot env", func() {
		Expect(fs.WriteFile(selinux.SelinuxTargetedContextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(fs.WriteFile(contextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, "/partition/var", vfs.DirPerm)).To(Succeed())
		Expect(selinux.ChrootedRelabel(
			context.Background(), s, root, map[string]string{"/partition/var": "/var"}),
		).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"setfiles", "-i", "-F", "/etc/selinux/targeted/contexts/files/file_contexts", "/", "/var"},
			{"sync"},
			{"setfiles", "-i", "-F", "-r", "/some/root", "/some/root/etc/selinux/targeted/contexts/files/file_contexts", "/some/root/var"},
		})).To(Succeed())
	})
	It("fails to relabel in a chroot env if chroot mounts fail", func() {
		Expect(fs.WriteFile(selinux.SelinuxTargetedContextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		// /partion/var does not exist
		Expect(selinux.ChrootedRelabel(
			context.Background(), s, root, map[string]string{"/partition/var": "/var"}),
		).NotTo(Succeed())
	})
	It("does nothing if the context is not found in chroot", func() {
		Expect(vfs.MkdirAll(fs, "/partition/var", vfs.DirPerm)).To(Succeed())
		Expect(selinux.ChrootedRelabel(
			context.Background(), s, root, map[string]string{"/partition/var": "/var"}),
		).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{}}))
		Expect(buffer.String()).To(ContainSubstring("no context found"))
	})
})
