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
	"slices"

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
	var unpacker unpack.Interface
	var s *sys.System
	var cleanup func()
	var err error
	var runner *sysmock.Runner
	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		runner = sysmock.NewRunner()
		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithRunner(runner),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/some/root", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/some/root/datafile", []byte("data"), vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/target/dir", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("creates a directory unpacker with default rsync flags", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewDirSrc("/some/root"), unpack.WithRsyncFlags())
		Expect(err).NotTo(HaveOccurred())
		_, ok := unpacker.(*unpack.Directory)
		Expect(ok).To(BeTrue())
		_, err = unpacker.Unpack(context.Background(), "/target/dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Contains(runner.GetCmds()[0], "--human-readable")).To(BeTrue())
	})
	It("creates a directory unpacker with custom rsync flags", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewDirSrc("/some/root"), unpack.WithRsyncFlags("--custom"))
		Expect(err).NotTo(HaveOccurred())
		_, err = unpacker.Unpack(context.Background(), "/target/dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Contains(runner.GetCmds()[0], "--custom")).To(BeTrue())
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
	It("creates a tar unpacker", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewTarSrc("/some/tarball.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		_, ok := unpacker.(*unpack.Tar)
		Expect(ok).To(BeTrue())
	})
	It("fails with an empty source", func() {
		unpacker, err = unpack.NewUnpacker(s, deployment.NewEmptySrc())
		Expect(err).To(HaveOccurred())
	})
})
