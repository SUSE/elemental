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

package fstab_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"github.com/suse/elemental/v3/pkg/fstab"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestFstabSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fstab test suite")
}

const fstabFile = `/dev/device   /     ext2  ro,defaults             0 1
LABEL=mylabel /data btrfs defaults,subvol=/@/data 0 0
UUID=afadf    /etc  btrfs defaults,subvol=/@/etc  0 0
`

const updatedFstab = `/dev/device /     ext2  ro,defaults                     0 1
UUID=uuid   /data btrfs defaults,subvol=/@/new/data     0 0
UUID=afadf  /etc  btrfs defaults,subvol=/@/new/path/etc 0 0
`

var _ = Describe("Fstab", Label("fstab"), func() {
	var tfs vfs.FS
	var s *sys.System
	var cleanup func()
	var err error
	var lines []fstab.Line
	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/etc", vfs.DirPerm)).To(Succeed())

		lines = []fstab.Line{{
			Device:     "/dev/device",
			MountPoint: "/",
			FileSystem: "ext2",
			Options:    []string{"ro", "defaults"},
			FsckOrder:  1,
		}, {
			Device:     "LABEL=mylabel",
			MountPoint: "/data",
			FileSystem: "btrfs",
			Options:    []string{"defaults", "subvol=/@/data"},
		}, {
			Device:     "UUID=afadf",
			MountPoint: "/etc",
			FileSystem: "btrfs",
			Options:    []string{"defaults", "subvol=/@/etc"},
		}}
		format.TruncatedDiff = false
	})
	AfterEach(func() {
		cleanup()
	})
	It("creates an fstab file with the given lines", func() {
		Expect(fstab.Write(s, fstab.File, lines)).To(Succeed())
		data, err := tfs.ReadFile(fstab.File)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(fstabFile))
	})
	It("fails to write fstab file on a read-only filesystem", func() {
		tfs, err := sysmock.ReadOnlyTestFS(tfs)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())

		err = fstab.Write(s, fstab.File, lines)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("creating file: Create /etc/fstab: operation not permitted"))
	})
	It("updates the fstab file with a new line", func() {
		Expect(fstab.Write(s, fstab.File, lines)).To(Succeed())
		Expect(fstab.Update(
			s, fstab.File, []fstab.Line{
				{MountPoint: "/etc"}, {MountPoint: "/data"},
			}, []fstab.Line{
				{
					Device:     "UUID=afadf",
					MountPoint: "/etc",
					FileSystem: "btrfs",
					Options:    []string{"defaults", "subvol=/@/new/path/etc"},
				},
				{
					Device:     "UUID=uuid",
					MountPoint: "/data",
					FileSystem: "btrfs",
					Options:    []string{"defaults", "subvol=/@/new/data"},
				},
			},
		)).To(Succeed())
		data, err := tfs.ReadFile(fstab.File)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(updatedFstab))
	})
	It("fails to update fstab file on a read-only filesystem", func() {
		Expect(fstab.Write(s, fstab.File, lines)).To(Succeed())
		tfs, err := sysmock.ReadOnlyTestFS(tfs)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())

		err = fstab.Update(s, fstab.File, lines, lines)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("opening file: OpenFile /etc/fstab: operation not permitted"))
	})
})
