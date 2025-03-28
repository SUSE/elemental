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

package unpack_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

var _ = Describe("DirectoryUnpacker", Label("raw"), func() {
	var tfs vfs.FS
	var unpacker *unpack.Raw
	var s *sys.System
	var mount *sysmock.Mounter
	var cleanup func()
	BeforeEach(func() {
		var err error
		mount = sysmock.NewMounter()
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithMounter(mount))
		Expect(err).NotTo(HaveOccurred())
		unpacker = unpack.NewRawUnpacker(s, "/some/image.raw")
		Expect(vfs.MkdirAll(tfs, "/tmp/elemental_unpack", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/tmp/elemental_unpack/datafile", []byte("data"), vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("syncs data to target directory", func() {
		digest, err := unpacker.Unpack(context.Background(), "/target/dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal(""))
		ok, _ := vfs.Exists(tfs, "/target/dir/datafile")
		Expect(ok).To(BeTrue())
	})
	It("mirrors data to target directory", func() {
		Expect(tfs.WriteFile("/target/dir/pre-existing-file", []byte("data"), vfs.FilePerm)).To(Succeed())

		digest, err := unpacker.SynchedUnpack(context.Background(), "/target/dir")

		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal(""))
		ok, _ := vfs.Exists(tfs, "/target/dir/datafile")
		Expect(ok).To(BeTrue())
		ok, _ = vfs.Exists(tfs, "/target/dir/pre-existing-file")
		Expect(ok).To(BeFalse())
	})
	It("fails on mount error", func() {
		mount.ErrorOnMount = true
		_, err := unpacker.Unpack(context.Background(), "/target/dir")
		Expect(err).To(HaveOccurred())

		_, err = unpacker.SynchedUnpack(context.Background(), "/target/dir")
		Expect(err).To(HaveOccurred())
	})
	It("fails on umount error", func() {
		mount.ErrorOnUnmount = true
		_, err := unpacker.Unpack(context.Background(), "/target/dir")
		Expect(err).To(HaveOccurred())

		_, err = unpacker.SynchedUnpack(context.Background(), "/target/dir")
		Expect(err).To(HaveOccurred())
	})
})
