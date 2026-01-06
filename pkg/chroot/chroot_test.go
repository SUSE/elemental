/*
Copyright © 2022-2026 SUSE LLC
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
package chroot_test

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestChrootSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chroot test suite")
}

var _ = Describe("Chroot", Label("chroot"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var syscall *sysmock.Syscall
	var fs vfs.FS
	var s *sys.System
	var cleanup func()
	var chr *chroot.Chroot
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		syscall = &sysmock.Syscall{}
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithSyscall(syscall),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		chr = chroot.NewChroot(s, "/whatever")

		for _, path := range []string{"/dev", "/dev/pts", "/proc", "/sys"} {
			Expect(vfs.MkdirAll(fs, path, vfs.DirPerm)).To(Succeed())
		}
	})
	AfterEach(func() {
		cleanup()
	})

	Describe("ChrootedCallback method", func() {
		It("runs a callback in a chroot", func() {
			err := chroot.ChrootedCallback(s, "/somepath", nil, func() error {
				return nil
			})
			Expect(err).ShouldNot(HaveOccurred())
			err = chroot.ChrootedCallback(s, "/somepath", nil, func() error {
				return fmt.Errorf("callback error")
			})
			Expect(err).Should(HaveOccurred())
			Expect(err).To(MatchError("callback error"))
		})
	})
	Describe("on success", func() {
		It("calls a command in a chroot", func() {
			_, err := chr.Run("chroot-command")
			Expect(err).To(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
		})
		It("prepares chroot with and without default binds", func() {
			Expect(chr.Prepare()).To(Succeed())
			lst, err := mounter.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(lst)).To(Equal(4))
			Expect(chr.Close()).To(Succeed())

			chr = chroot.NewChroot(s, "/whatever", chroot.WithoutDefaultBinds())
			Expect(chr.Prepare()).To(Succeed())
			lst, err = mounter.List()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(lst)).To(Equal(0))
			Expect(chr.Close()).To(Succeed())
		})
		It("calls a command in a chroot with a customized binds", func() {
			binds := map[string]string{
				"/host/dir":  "/in/chroot/path",
				"/host/file": "/in/chroot/file",
			}
			Expect(vfs.MkdirAll(fs, "/host/dir", vfs.DirPerm)).To(Succeed())
			Expect(fs.WriteFile("/host/file", []byte("data"), vfs.FilePerm)).To(Succeed())
			chr.SetExtraMounts(binds)
			Expect(chr.Prepare()).To(BeNil())
			defer chr.Close()
			_, err := chr.Run("chroot-command")
			Expect(err).To(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
			_, err = chr.Run("chroot-another-command")
			Expect(err).To(BeNil())
		})
		It("runs a callback in a custom chroot", func() {
			called := false
			callback := func() error {
				called = true
				return nil
			}
			err := chr.RunCallback(callback)
			Expect(err).To(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
			Expect(called).To(BeTrue())
		})
	})
	Describe("on failure", func() {
		It("should return error if chroot-command fails", func() {
			runner.ReturnError = errors.New("run error")
			_, err := chr.Run("chroot-command")
			Expect(err).NotTo(BeNil())
			Expect(err).To(MatchError("run error"))
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
		})
		It("should return error if callback fails", func() {
			called := false
			callback := func() error {
				called = true
				return errors.New("callback error")
			}
			err := chr.RunCallback(callback)
			Expect(err).NotTo(BeNil())
			Expect(err).To(MatchError("callback error"))
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
			Expect(called).To(BeTrue())
		})
		It("should return error if preparing twice before closing", func() {
			Expect(chr.Prepare()).To(BeNil())
			DeferCleanup(func() {
				Expect(chr.Close()).To(Succeed())
			})
			err := chr.Prepare()
			Expect(err).NotTo(BeNil())
			Expect(err).To(MatchError("there are already active mountpoints for this instance"))
			Expect(chr.Close()).To(BeNil())
			Expect(chr.Prepare()).To(BeNil())
		})
		It("should return error if failed to chroot", func() {
			syscall.ErrorOnChroot = true
			_, err := chr.Run("chroot-command")
			Expect(err).ToNot(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
			Expect(err).To(MatchError("chroot /whatever: chroot error"))
		})
		It("should return error if failed to mount on prepare", Label("mount"), func() {
			mounter.ErrorOnMount = true
			_, err := chr.Run("chroot-command")
			Expect(err).ToNot(BeNil())
			Expect(err).To(MatchError("preparing default mounts: mount error"))
		})
		It("should return error if failed to unmount on close", Label("unmount"), func() {
			mounter.ErrorOnUnmount = true
			_, err := chr.Run("chroot-command")
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed closing chroot"))
		})
	})
})
