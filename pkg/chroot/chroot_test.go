/*
Copyright © 2022-2025 SUSE LLC
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
			Expect(err.Error()).To(ContainSubstring("callback error"))
		})
	})
	Describe("on success", func() {
		It("command should be called in the chroot", func() {
			_, err := chr.Run("chroot-command")
			Expect(err).To(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
		})
		It("commands should be called with a customized chroot", func() {
			chr.SetExtraMounts(map[string]string{"/real/path": "/in/chroot/path"})
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
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
		})
		It("should return error if callback fails", func() {
			called := false
			callback := func() error {
				called = true
				return errors.New("Callback error")
			}
			err := chr.RunCallback(callback)
			Expect(err).NotTo(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
			Expect(called).To(BeTrue())
		})
		It("should return error if preparing twice before closing", func() {
			Expect(chr.Prepare()).To(BeNil())
			defer chr.Close()
			Expect(chr.Prepare()).NotTo(BeNil())
			Expect(chr.Close()).To(BeNil())
			Expect(chr.Prepare()).To(BeNil())
		})
		It("should return error if failed to chroot", func() {
			syscall.ErrorOnChroot = true
			_, err := chr.Run("chroot-command")
			Expect(err).ToNot(BeNil())
			Expect(syscall.WasChrootCalledWith("/whatever")).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("chroot error"))
		})
		It("should return error if failed to mount on prepare", Label("mount"), func() {
			mounter.ErrorOnMount = true
			_, err := chr.Run("chroot-command")
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("mount error"))
		})
		It("should return error if failed to unmount on close", Label("unmount"), func() {
			mounter.ErrorOnUnmount = true
			_, err := chr.Run("chroot-command")
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed closing chroot"))
		})
	})
})
