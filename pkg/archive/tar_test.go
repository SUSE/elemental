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

package archive_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/archive"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestArchiveSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Archive test suite")
}

var _ = Describe("Archive", Label("archive"), func() {
	var s *sys.System
	var tfs vfs.FS
	var err error
	var buffer *bytes.Buffer

	BeforeEach(func() {
		var gzData []byte

		buffer = &bytes.Buffer{}
		gzData, err = os.ReadFile("../../tests/assets/test.tar.gz")
		Expect(err).NotTo(HaveOccurred())

		// Include tarballs in test environment
		tfs, _, err = sysmock.TestFS(map[string]any{
			"/data/test.tar.gz":  gzData,
			"/data/test.tar.bz2": "invalid",
			"/data/test.tar":     "invalid",
		})
		s, err = sys.NewSystem(
			sys.WithFS(tfs),
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/root", vfs.DirPerm)).To(Succeed())
	})

	AfterEach(func() {
		// Test tarball includes directories with 0500 permissions
		// testFS cleanup function does not delete them
		Expect(vfs.ForceRemoveAll(tfs, "/")).To(Succeed())
	})

	It("Extracts a tar.gz file content to the given target", func() {
		Expect(archive.ExtractTarball(context.Background(), s, "/data/test.tar.gz", "/root")).To(Succeed())
		ok, _ := vfs.Exists(tfs, "/root/etc/os-release")
		Expect(ok).To(BeTrue())
		info, err := tfs.Lstat("/root/var/readonly-dir")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode() & 0222).To(Equal(os.FileMode(0)))
		info, err = tfs.Lstat("/root/var/readonly-dir/readonly-file")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode() & 0222).To(Equal(os.FileMode(0)))
		info, err = tfs.Lstat("/root/etc/elemental/symlink")
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode() & os.ModeSymlink).To(Equal(os.ModeSymlink))
		infoHl, err := tfs.Lstat("/root/etc/elemental/hardlink")
		Expect(err).ToNot(HaveOccurred())
		infoRel, err := tfs.Lstat("/root/etc/os-release")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.SameFile(infoRel, infoHl)).To(BeTrue())

	})

	It("Extracts with a filter", func() {
		filter := func(h *tar.Header) (bool, error) {
			if filepath.Join("/root", h.Name) == "/root/etc/os-release" {
				return false, fmt.Errorf("needle found")
			}
			return true, nil
		}
		Expect(archive.ExtractTarball(context.Background(), s, "/data/test.tar.gz", "/root", filter)).To(MatchError(ContainSubstring("needle found")))

		// Exclude certain path from extraction
		filter = func(h *tar.Header) (bool, error) {
			if filepath.Join("/root", h.Name) == "/root/etc/os-release" {
				return false, nil
			}
			return true, nil
		}
		Expect(archive.ExtractTarball(context.Background(), s, "/data/test.tar.gz", "/root", filter)).To(Succeed())
		ok, _ := vfs.Exists(tfs, "/root/etc/os-release")
		Expect(ok).To(BeFalse())
	})

	It("fails to extract files of wrong type", func() {
		Expect(archive.ExtractTarball(context.Background(), s, "/data/test.tar", "/root")).NotTo(Succeed())
		Expect(archive.ExtractTarball(context.Background(), s, "/data/test.tar.bz2", "/root")).NotTo(Succeed())
	})
})
