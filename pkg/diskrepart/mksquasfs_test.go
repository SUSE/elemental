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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Mksquashfs", Label("mksquashfs"), func() {
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
		Expect(vfs.MkdirAll(fs, "/some/root/subdir", vfs.DirPerm))
		Expect(fs.WriteFile("/some/root/file", []byte{}, vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("Creates a squashfs image form the given root without compression", func() {
		Expect(diskrepart.CreateSquashFS(
			context.Background(), s, "/some/root", "/some/rootfs.squashfs",
			diskrepart.SquashfsNoCompressionOptions(),
		)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"mksquashfs", "/some/root", "/some/rootfs.squashfs", "-no-compression"},
		})).To(Succeed())
	})
	It("Creates a squashfs image form the given root excluding a folder", func() {
		Expect(diskrepart.CreateSquashFS(
			context.Background(), s, "/some/root", "/some/rootfs.squashfs",
			diskrepart.SquashfsExcludeOptions("subdir*"),
		)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"mksquashfs", "/some/root", "/some/rootfs.squashfs", "-wildcards", "-e", "subdir*"},
		})).To(Succeed())
	})
	It("Creates a squashfs image with default parameters", func() {
		Expect(diskrepart.CreateSquashFS(
			context.Background(), s, "/some/root", "/some/rootfs.squashfs",
			diskrepart.DefaultSquashfsCompressionOptions(),
		)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"mksquashfs", "/some/root", "/some/rootfs.squashfs", "-b", "1024k"},
		})).To(Succeed())
	})
})
