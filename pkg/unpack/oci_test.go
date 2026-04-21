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

package unpack_test

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrdmock "github.com/suse/elemental/v3/pkg/containerd/mock"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/runner"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

const (
	alpineImageRef = "docker.io/library/alpine:3.21.3"
	bogusImageRef  = "registry.invalid./alpine:3.21.3"
)

var _ = Describe("OCIUnpacker", Label("oci", "rootlesskit"), func() {
	var tfs vfs.FS
	var s *sys.System
	var cleanup func()
	BeforeEach(func() {
		var err error
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithRunner(runner.NewRunner()), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Unpacks a remote alpine image", func() {
		unpacker := unpack.NewOCIUnpacker(s, alpineImageRef, unpack.WithPlatformRefOCI("linux/amd64"), unpack.WithLocalOCI(false))
		Expect(vfs.MkdirAll(tfs, "/target/root", vfs.DirPerm)).To(Succeed())
		digest, err := unpacker.Unpack(context.Background(), "/target/root")
		Expect(err).NotTo(HaveOccurred())
		exists, _ := vfs.Exists(tfs, "/target/root/etc/os-release")
		Expect(exists).To(BeTrue())
		data, err := tfs.ReadFile("/target/root/etc/os-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("VERSION_ID=3.21.3"))
		Expect(digest).To(ContainSubstring("sha256:"))
	})
	It("Fails to unpacks a remote bogus image", func() {
		unpacker := unpack.NewOCIUnpacker(s, bogusImageRef, unpack.WithPlatformRefOCI("linux/amd64"), unpack.WithLocalOCI(false))
		Expect(vfs.MkdirAll(tfs, "/target/root", vfs.DirPerm)).To(Succeed())
		digest, err := unpacker.Unpack(context.Background(), "/target/root")
		Expect(err).To(HaveOccurred())
		exists, _ := vfs.Exists(tfs, "/target/root/etc/os-release")
		Expect(exists).To(BeFalse())
		Expect(digest).To(BeEmpty())
	})
	It("Unpacks a local alpine image", Serial, func() {
		_, err := s.Runner().Run("docker", "pull", alpineImageRef)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			_, err := s.Runner().Run("docker", "image", "rm", alpineImageRef)
			Expect(err).NotTo(HaveOccurred())
		})

		unpacker := unpack.NewOCIUnpacker(s, alpineImageRef, unpack.WithPlatformRefOCI("linux/amd64"), unpack.WithLocalOCI(true))
		Expect(vfs.MkdirAll(tfs, "/target/root", vfs.DirPerm)).To(Succeed())

		// Unpack excluding /usr/sbin
		digest, err := unpacker.Unpack(context.Background(), "/target/root", "/usr/sbin")
		Expect(err).NotTo(HaveOccurred())
		exists, _ := vfs.Exists(tfs, "/target/root/etc/os-release")
		Expect(exists).To(BeTrue())
		exists, _ = vfs.Exists(tfs, "/target/root/usr/sbin")
		Expect(exists).To(BeFalse())
		data, err := tfs.ReadFile("/target/root/etc/os-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("VERSION_ID=3.21.3"))
		Expect(digest).To(ContainSubstring("sha256:"))
	})
	It("Fails to unpacks a local bogus image", func() {
		unpacker := unpack.NewOCIUnpacker(s, bogusImageRef, unpack.WithPlatformRefOCI("linux/amd64"), unpack.WithLocalOCI(true))
		Expect(vfs.MkdirAll(tfs, "/target/root", vfs.DirPerm)).To(Succeed())
		digest, err := unpacker.Unpack(context.Background(), "/target/root")
		Expect(err).To(HaveOccurred())
		exists, _ := vfs.Exists(tfs, "/target/root/etc/os-release")
		Expect(exists).To(BeFalse())
		Expect(digest).To(BeEmpty())
	})
	It("Syncs a remote alpine image to destination, excludes paths and keeps protected ones", func() {
		unpacker := unpack.NewOCIUnpacker(s, alpineImageRef, unpack.WithPlatformRefOCI("linux/amd64"), unpack.WithLocalOCI(false))
		Expect(vfs.MkdirAll(tfs, "/target/root/protected", vfs.DirPerm)).To(Succeed())
		digest, err := unpacker.SynchedUnpack(context.Background(), "/target/root", []string{"/etc/os-release"}, []string{"/protected"})
		Expect(err).NotTo(HaveOccurred())
		exists, _ := vfs.Exists(tfs, "/target/root/etc/os-release")
		Expect(exists).To(BeFalse())
		exists, _ = vfs.Exists(tfs, "/target/root/protected")
		Expect(exists).To(BeTrue())
		Expect(digest).To(ContainSubstring("sha256:"))
	})
})

var _ = Describe("OCIUnpacker", Label("oci", "containerd"), func() {
	var tfs vfs.FS
	var s *sys.System
	var cleanup func()
	var ctrd *ctrdmock.ContainerdMock
	BeforeEach(func() {
		var err error
		ctrd = &ctrdmock.ContainerdMock{
			MntRootFS: "/mnt/root",
		}
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())
		err = os.Setenv(unpack.CtrdSockEnv, "/run/containerd/containerd.sock")
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/target/root", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/mnt/root", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/run/containerd", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/run/containerd/containerd.sock", []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/mnt/root/content", []byte("image content"), vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Unpacks a local image from containerd", func() {
		ctrd.Img.Digest = "imageID"
		ctrd.MntRootFS = "/mnt/root"
		unpacker := unpack.NewOCIUnpacker(s, alpineImageRef, unpack.WithContainerd(ctrd), unpack.WithLocalOCI(true))

		digest, err := unpacker.Unpack(context.Background(), "/target/root")
		Expect(err).NotTo(HaveOccurred())
		exists, _ := vfs.Exists(tfs, "/target/root/content")
		Expect(exists).To(BeTrue())
		Expect(digest).To(Equal("imageID"))
	})
	It("Fails to find an image in containerd", func() {
		ctrd.EFind = fmt.Errorf("image not found")
		unpacker := unpack.NewOCIUnpacker(s, alpineImageRef, unpack.WithContainerd(ctrd), unpack.WithLocalOCI(true))
		_, err := unpacker.Unpack(context.Background(), "/target/root")
		Expect(err).To(MatchError("image not found"))
	})
	It("Fails to mount a containerd image", func() {
		ctrd.ERunOnMounted = fmt.Errorf("failed to mount image")
		unpacker := unpack.NewOCIUnpacker(s, alpineImageRef, unpack.WithContainerd(ctrd), unpack.WithLocalOCI(true))
		_, err := unpacker.Unpack(context.Background(), "/target/root")
		Expect(err).To(MatchError("failed to mount image"))
	})
})
