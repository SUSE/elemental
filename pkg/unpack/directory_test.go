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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

var _ = Describe("DirectoryUnpacker", Label("directory"), func() {
	var tfs vfs.FS
	var unpacker *unpack.Directory
	var s *sys.System
	var cleanup func()
	BeforeEach(func() {
		var err error
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).NotTo(HaveOccurred())
		unpacker = unpack.NewDirectoryUnpacker(s, "/some/root")
		Expect(vfs.MkdirAll(tfs, "/some/root", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/some/root/datafile", []byte("data"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/some/root/foo", []byte("bar"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/some/root/pipo", []byte("bar"), vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("syncs data to target directory excluding paths", func() {
		digest, err := unpacker.Unpack(context.Background(), "/target/dir", "/foo", "/pip")
		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal(""))
		ok, _ := vfs.Exists(tfs, "/target/dir/datafile")
		Expect(ok).To(BeTrue())
		ok, _ = vfs.Exists(tfs, "/target/dir/foo")
		Expect(ok).To(BeFalse())

		// excluded files must match directory of filename
		ok, _ = vfs.Exists(tfs, "/target/dir/pipo")
		Expect(ok).To(BeTrue())
	})
	It("mirrors data to target directory", func() {
		Expect(tfs.WriteFile("/target/dir/pre-existing-file", []byte("data"), vfs.FilePerm)).To(Succeed())

		digest, err := unpacker.SynchedUnpack(context.Background(), "/target/dir", nil, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal(""))
		ok, _ := vfs.Exists(tfs, "/target/dir/datafile")
		Expect(ok).To(BeTrue())
		ok, _ = vfs.Exists(tfs, "/target/dir/pre-existing-file")
		Expect(ok).To(BeFalse())
	})
	It("reads the deployment data from the source tree", func() {
		d := deployment.DefaultDeployment()
		d.SourceOS = deployment.NewOCISrc("domain.org/image:tag")
		d.SourceOS.SetDigest("digest")
		Expect(d.WriteDeploymentFile(s, "/some/root")).To(Succeed())

		digest, err := unpacker.Unpack(context.Background(), "/target/dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal("digest"))
	})
})
