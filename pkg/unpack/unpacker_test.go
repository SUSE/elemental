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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

var _ = Describe("DirectoryUnpacker", Label("directory"), func() {
	var tfs vfs.FS
	var unpacker unpack.Interface
	var s *sys.System
	var cleanup func()
	var err error
	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs))
		Expect(err).NotTo(HaveOccurred())
		unpacker = unpack.NewDirectoryUnpacker(s, "/some/root")
		Expect(vfs.MkdirAll(tfs, "/some/root", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/some/root/datafile", []byte("data"), vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("creates a directory unpacker", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewDirSrc("/some/dir"))
		Expect(err).NotTo(HaveOccurred())
		_, ok := unpacker.(*unpack.Directory)
		Expect(ok).To(BeTrue())
	})
	It("creates a raw unpacker", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewRawSrc("/some/image.raw"))
		Expect(err).NotTo(HaveOccurred())
		_, ok := unpacker.(*unpack.Raw)
		Expect(ok).To(BeTrue())
	})
	It("creates an oci unpacker", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewOCISrc("domain.org/some/image:tag"))
		Expect(err).NotTo(HaveOccurred())
		_, ok := unpacker.(*unpack.OCI)
		Expect(ok).To(BeTrue())
	})
	It("fails with an empty source", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewEmptySrc())
		Expect(err).To(HaveOccurred())
	})
})
