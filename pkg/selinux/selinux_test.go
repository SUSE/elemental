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

const (
	semanageConf = "/etc/selinux/semanage.conf"
	defaultStore = "/var/lib/selinux/targeted/active"
	etcStore     = "/etc/selinux/targeted/active"
	marker       = "/etc/selinux/elemental-selinux_modules_migrated-targeted"
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
	It("relabels the given path for the targeted context, no shared paths no .autorelabel file", func() {
		Expect(fs.WriteFile(contextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(selinux.SystemRelabel(context.Background(), s, root, []string{root + "/etc"}, nil)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"setfiles", "-i", "-r", "/some/root", "-F", "-e", "/some/root/etc",
			"/some/root/etc/selinux/targeted/contexts/files/file_contexts", "/some/root",
		}, {
			"setfiles", "-i", "-r", "/some/root",
			"/some/root/etc/selinux/targeted/contexts/files/file_contexts", "/some/root/etc",
		}})).To(Succeed())
		Expect(vfs.Exists(s.FS(), filepath.Join(root, "/etc/selinux/.autorelabel"))).To(BeFalse())
	})
	It("relabels the given path for the targeted context, including shared paths sets the .autorelabel file", func() {
		Expect(fs.WriteFile(contextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(selinux.SystemRelabel(context.Background(), s, root, nil, []string{root + "/var"})).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"setfiles", "-i", "-r", "/some/root", "-F", "-e", "/some/root/var",
			"/some/root/etc/selinux/targeted/contexts/files/file_contexts", "/some/root",
		}})).To(Succeed())
		Expect(vfs.Exists(s.FS(), filepath.Join(root, "/etc/selinux/.autorelabel"))).To(BeTrue())
	})
	It("does nothing if the context is not found", func() {
		Expect(selinux.SystemRelabel(context.Background(), s, root, nil, nil)).To(Succeed())
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
		Expect(selinux.SystemRelabel(context.Background(), s, root, []string{}, []string{})).
			To(MatchError(ContainSubstring("setfiles failed")))
	})
	It("relabels the given paths for the targeted context in a chroot env", func() {
		Expect(fs.WriteFile(selinux.SelinuxTargetedContextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(fs.WriteFile(contextFile, []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, "/partition/var", vfs.DirPerm)).To(Succeed())
		Expect(selinux.ChrootedSystemRelabel(
			context.Background(), s, root, []string{"/etc"}, []string{"/var"}),
		).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"setfiles", "-i", "-F", "-e", "/etc", "-e", "/var", "/etc/selinux/targeted/contexts/files/file_contexts", "/"},
			{"setfiles", "-i", "/etc/selinux/targeted/contexts/files/file_contexts", "/etc"},
			{"sync"},
		})).To(Succeed())
	})
	It("does nothing if the context is not found in chroot", func() {
		Expect(selinux.ChrootedSystemRelabel(
			context.Background(), s, root, nil, nil),
		).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{}}))
		Expect(buffer.String()).To(ContainSubstring("no context found"))
	})
})

var _ = Describe("Policy refresh", Label("selinux"), func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var buffer *bytes.Buffer
	root := "/some/root"

	BeforeEach(func() {
		var err error
		buffer = &bytes.Buffer{}
		runner = sysmock.NewRunner()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		logger := log.New(log.WithBuffer(buffer))
		logger.SetLevel(log.DebugLevel())
		s, err = sys.NewSystem(
			sys.WithMounter(sysmock.NewMounter()), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(logger),
			sys.WithSyscall(&sysmock.Syscall{}),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(fs, root, vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, etcStore+"/modules", 0o700)).To(Succeed())
		Expect(fs.WriteFile(semanageConf, []byte("module-store = direct\nstore-root=/etc/selinux\n"), vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})

	It("skips refresh when no store-root has been provided", func() {
		Expect(fs.WriteFile(semanageConf, []byte("module-store = direct\n"), vfs.FilePerm)).To(Succeed())
		Expect(selinux.ChrootedRefreshPolicy(context.Background(), s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(runner.IncludesCmds([][]string{{"semodule"}})).NotTo(Succeed())
		Expect(buffer.String()).To(ContainSubstring("skipping policy refresh"))
	})

	It("skips refresh when semanage.conf is missing", func() {
		Expect(fs.Remove(semanageConf)).To(Succeed())
		Expect(selinux.ChrootedRefreshPolicy(context.Background(), s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())
		Expect(runner.IncludesCmds([][]string{{"semodule"}})).NotTo(Succeed())
	})

	It("skips refresh when there is no active policy store", func() {
		Expect(fs.RemoveAll(etcStore)).To(Succeed())
		Expect(selinux.ChrootedRefreshPolicy(context.Background(), s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())
		Expect(runner.IncludesCmds([][]string{{"semodule"}})).NotTo(Succeed())
		Expect(buffer.String()).To(ContainSubstring("No SE Linux policy store found"))
	})

	It("fails on semodule command error", func() {
		runner.SideEffect = func(cmd string, _ ...string) ([]byte, error) {
			if cmd == "semodule" {
				return []byte{}, fmt.Errorf("semodule failed")
			}
			return []byte{}, nil
		}
		Expect(selinux.ChrootedRefreshPolicy(context.Background(), s, root, selinux.SelinuxTargetedPolicyType)).To(MatchError(ContainSubstring("semodule failed")))
	})
})

var _ = Describe("Policy migration", Label("selinux"), func() {
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var buffer *bytes.Buffer
	root := "/some/root"

	writeModule := func(dir, content string) {
		Expect(vfs.MkdirAll(fs, dir, 0o700)).To(Succeed())
		Expect(fs.WriteFile(filepath.Join(dir, "cil"), []byte(content), 0o600)).To(Succeed())
	}

	BeforeEach(func() {
		var err error
		buffer = &bytes.Buffer{}
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		logger := log.New(log.WithBuffer(buffer))
		logger.SetLevel(log.DebugLevel())
		s, err = sys.NewSystem(
			sys.WithMounter(sysmock.NewMounter()), sys.WithRunner(sysmock.NewRunner()),
			sys.WithFS(fs), sys.WithLogger(logger),
			sys.WithSyscall(&sysmock.Syscall{}),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(fs, root, vfs.DirPerm)).To(Succeed())
		// Prepare the env to have both an /etc/selinux and /var/lib/selinux active policy stores
		Expect(vfs.MkdirAll(fs, etcStore+"/modules", 0o700)).To(Succeed())
		Expect(vfs.MkdirAll(fs, defaultStore+"/modules", 0o700)).To(Succeed())
		Expect(fs.WriteFile(semanageConf, []byte("module-store = direct\nstore-root=/etc/selinux\n"), vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})

	It("does not copy base or package modules", func() {
		writeModule(defaultStore+"/modules/100/foo", "foo")
		writeModule(defaultStore+"/modules/200/bar", "bar")
		writeModule(defaultStore+"/modules/tmp/baz", "baz")

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, etcStore+"/modules/100")).To(BeFalse())
		Expect(vfs.Exists(fs, etcStore+"/modules/200")).To(BeFalse())
		Expect(vfs.Exists(fs, etcStore+"/modules/tmp")).To(BeFalse())
	})

	It("does not overwrite modules already in the new store", func() {
		writeModule(defaultStore+"/modules/400/rke2", "old")
		writeModule(etcStore+"/modules/400/rke2", "new")

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(fs.ReadFile(etcStore + "/modules/400/rke2/cil")).To(Equal([]byte("new")))
	})

	It("does not migrate once the migration marker is in place", func() {
		Expect(fs.WriteFile(marker, []byte{}, 0o644)).To(Succeed())
		writeModule(defaultStore+"/modules/400/rke2", "rke2")
		Expect(fs.WriteFile(defaultStore+"/booleans.local", []byte("httpd_can_network_connect=1\n"), 0o600)).To(Succeed())

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, etcStore+"/modules/400/rke2")).To(BeFalse())
		Expect(vfs.Exists(fs, etcStore+"/booleans.local")).To(BeFalse())
		Expect(vfs.Exists(fs, marker)).To(BeTrue())
	})

	It("does not migrate when semanage.conf exists, but store-root is missing", func() {
		Expect(fs.WriteFile(semanageConf, []byte("module-store = direct\n"), vfs.FilePerm)).To(Succeed())
		writeModule(defaultStore+"/modules/400/rke2", "rke2")

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, etcStore+"/modules/400/rke2")).To(BeFalse())
		Expect(vfs.Exists(fs, marker)).To(BeFalse())
	})

	It("does not migrate when semanage.conf is missing", func() {
		Expect(fs.Remove(semanageConf)).To(Succeed())
		writeModule(defaultStore+"/modules/400/rke2", "rke2")

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, etcStore+"/modules/400/rke2")).To(BeFalse())
		Expect(vfs.Exists(fs, marker)).To(BeFalse())
	})

	It("does not migrate when defined new store is actually missing", func() {
		Expect(fs.RemoveAll(etcStore)).To(Succeed())
		writeModule(defaultStore+"/modules/400/rke2", "rke2")

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, marker)).To(BeFalse())
		Expect(buffer.String()).To(ContainSubstring("Local policy migration will not be performed"))
	})

	It("does not migrate when the default store and migration marker are missing", func() {
		Expect(fs.RemoveAll("/var/lib/selinux")).To(Succeed())

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, marker)).To(BeFalse())
	})

	It("replaces the local file configurations and writes migration marker", func() {
		Expect(fs.WriteFile(defaultStore+"/booleans.local", []byte("virt_use_nfs=1\nuser_conf=1\n"), 0o600)).To(Succeed())
		Expect(fs.WriteFile(defaultStore+"/foo.bar", []byte("baz"), 0o600)).To(Succeed())
		Expect(fs.WriteFile(etcStore+"/booleans.local", []byte("virt_use_nfs=1\n"), 0o600)).To(Succeed())

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(fs.ReadFile(etcStore + "/booleans.local")).To(Equal([]byte("virt_use_nfs=1\nuser_conf=1\n")))
		Expect(vfs.Exists(fs, etcStore+"/foo.bar")).To(BeFalse())
		Expect(buffer.String()).To(ContainSubstring("Replacing"))
		Expect(vfs.Exists(fs, marker)).To(BeTrue())
	})

	It("copies local and disabled modules and writes migration marker", func() {
		writeModule(defaultStore+"/modules/400/rke2", "rke2")
		Expect(vfs.MkdirAll(fs, defaultStore+"/modules/disabled", 0o700)).To(Succeed())
		Expect(fs.WriteFile(defaultStore+"/modules/disabled/rsync", []byte{}, 0o644)).To(Succeed())

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(fs.ReadFile(etcStore + "/modules/400/rke2/cil")).To(Equal([]byte("rke2")))
		Expect(vfs.Exists(fs, etcStore+"/modules/disabled/rsync")).To(BeTrue())
		info, err := fs.Stat(etcStore + "/modules/400/rke2")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(BeEquivalentTo(0o700))
		Expect(vfs.Exists(fs, marker)).To(BeTrue())
	})

	It("removes the marker once the default store is gone", func() {
		Expect(fs.RemoveAll("/var/lib/selinux")).To(Succeed())
		Expect(fs.WriteFile(marker, []byte{}, 0o644)).To(Succeed())

		Expect(selinux.ChrootedDefaultPolicyMigration(s, root, selinux.SelinuxTargetedPolicyType)).To(Succeed())

		Expect(vfs.Exists(fs, marker)).To(BeFalse())
	})

})
