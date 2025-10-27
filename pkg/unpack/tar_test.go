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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

var _ = Describe("TarUnpacker", Label("tar"), func() {
	var tfs vfs.FS
	var unpacker *unpack.Tar
	var s *sys.System

	BeforeEach(func() {
		var err error
		var gzData []byte

		gzData, err = os.ReadFile("../../tests/assets/test.tar.gz")
		Expect(err).NotTo(HaveOccurred())

		// Include tarballs in test environment
		tfs, _, err = sysmock.TestFS(map[string]any{
			"/data/test.tar.gz":    gzData,
			"/root/etc/os-release": "test",
			"/root/etc/somefile":   "pre-existing file",
		})

		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		unpacker = unpack.NewTarUnpacker(s, "/data/test.tar.gz")
	})
	AfterEach(func() {
		// Test tarball includes directories with 0500 permissions
		// testFS cleanup function does not delete them
		Expect(vfs.ForceRemoveAll(tfs, "/")).To(Succeed())
	})

	It("unpacks data and overwrites content", func() {
		data, err := tfs.ReadFile("/root/etc/os-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("test"))

		_, err = unpacker.Unpack(context.Background(), "/root")
		Expect(err).NotTo(HaveOccurred())

		data, err = tfs.ReadFile("/root/etc/os-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(Equal("test"))

		ok, _ := vfs.Exists(tfs, "/root/etc/elemental/hardlink")
		Expect(ok).To(BeTrue())

		ok, _ = vfs.Exists(tfs, "/root/etc/somefile")
		Expect(ok).To(BeTrue())
	})

	It("unpacks data excluding given paths", func() {
		_, err := unpacker.Unpack(context.Background(), "/root", "var", "etc/os")
		Expect(err).NotTo(HaveOccurred())

		ok, _ := vfs.Exists(tfs, "/root/var")
		Expect(ok).To(BeFalse())

		// exclude 'etc/os' does not exlude os-release file
		// exludes require a full directory or file path to be effective
		ok, _ = vfs.Exists(tfs, "/root/etc/os-release")
		Expect(ok).To(BeTrue())
	})

	It("mirrors data deleting pre-existing content", func() {
		data, err := tfs.ReadFile("/root/etc/os-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("test"))

		_, err = unpacker.SynchedUnpack(context.Background(), "/root", []string{"/etc/elemental"}, nil)
		Expect(err).NotTo(HaveOccurred())

		data, err = tfs.ReadFile("/root/etc/os-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(Equal("test"))

		ok, _ := vfs.Exists(tfs, "/root/etc/somefile")
		Expect(ok).To(BeFalse())

		ok, _ = vfs.Exists(tfs, "/root/etc/elemental")
		Expect(ok).To(BeFalse())
	})
})
