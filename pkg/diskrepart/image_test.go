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

package diskrepart_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Image", Label("image"), func() {
	var fs vfs.FS
	var s *sys.System
	var runner *sysmock.Runner
	var cleanup func()
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		Expect(err).ToNot(HaveOccurred())
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(vfs.MkdirAll(fs, "/test", vfs.DirPerm))
		Expect(vfs.MkdirAll(fs, "/some/root", vfs.DirPerm))
		Expect(fs.WriteFile("/some/root/file", []byte{}, vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Creates an empty file with the required size", func() {
		Expect(diskrepart.CreateEmptyFile(fs, "/test/raw.img", 10, false)).To(Succeed())
		ok, _ := vfs.Exists(fs, "/test/raw.img")
		Expect(ok).To(BeTrue())

		Expect(diskrepart.CreateEmptyFile(fs, "/test/raw_nosparse.img", 10, true)).To(Succeed())
		ok, _ = vfs.Exists(fs, "/test/raw_nosparse.img")
		Expect(ok).To(BeTrue())

		size, _ := vfs.DirSizeMB(fs, "/test")
		Expect(size).To(Equal(uint(21)))
	})
	It("Fails to create a file in a read only FS", func() {
		Expect(vfs.MkdirAll(fs, "/test", vfs.DirPerm))
		roFS, err := sysmock.ReadOnlyTestFS(fs)
		Expect(err).NotTo(HaveOccurred())
		Expect(diskrepart.CreateEmptyFile(roFS, "/test/raw.img", 10, false)).NotTo(Succeed())
	})
	It("Creates a ext4 image with preloaded content", func() {
		Expect(diskrepart.CreatePreloadedFileSystemImage(s, "/some/root", "/test/raw.img", "ROOT", 64, deployment.Ext4)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{"mkfs.ext4", "-L", "ROOT", "-F", "-d", "/some/root", "/test/raw.img"}})).To(Succeed())
		size, _ := vfs.DirSizeMB(fs, "/test")
		Expect(size).To(Equal(uint(129)))
	})
	It("Creates a ext2 image with preloaded content", func() {
		Expect(diskrepart.CreatePreloadedFileSystemImage(s, "/some/root", "/test/raw.img", "ROOT", 32, deployment.Ext2)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{"mkfs.ext2", "-L", "ROOT", "-F", "-d", "/some/root", "/test/raw.img"}})).To(Succeed())
		size, _ := vfs.DirSizeMB(fs, "/test")
		Expect(size).To(Equal(uint(65)))
	})
	It("Creates a btrfs image with preloaded content", func() {
		Expect(diskrepart.CreatePreloadedFileSystemImage(s, "/some/root", "/test/raw.img", "ROOT", 32, deployment.Btrfs)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{"mkfs.btrfs", "-L", "ROOT", "-f", "--root-dir", "/some/root", "/test/raw.img"}})).To(Succeed())
		size, _ := vfs.DirSizeMB(fs, "/test")
		Expect(size).To(Equal(uint(65)))
	})
	It("Creates a vfat image with preloaded content", func() {
		Expect(diskrepart.CreatePreloadedFileSystemImage(s, "/some/root", "/test/raw.img", "ROOT", 16, deployment.VFat)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"mkfs.vfat", "-n", "ROOT", "/test/raw.img"},
			{"mcopy", "-s", "-i", "/test/raw.img", "/some/root/file", "::"},
		})).To(Succeed())
		size, _ := vfs.DirSizeMB(fs, "/test")
		Expect(size).To(Equal(uint(33)))
	})
	It("Fails to create a preloaded image with a not supported filesystem", func() {
		Expect(diskrepart.CreatePreloadedFileSystemImage(s, "/some/root", "/test/raw.img", "ROOT", 16, deployment.XFS)).NotTo(Succeed())
	})
})
