/*
Copyright © 2022-2025 SUSE LLC
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

package sys_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("FS", Label("fs"), func() {
	var tfs sys.FS
	var cleanup func()
	var err error

	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(sys.MkdirAll(tfs, "/folder/subfolder", sys.DirPerm)).To(Succeed())
		Expect(err).ShouldNot(HaveOccurred())
		f, err := tfs.Create("/folder/file")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(f.Truncate(1024)).To(Succeed())

		f, err = tfs.Create("/folder/subfolder/file1")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(f.Truncate(2048)).To(Succeed())
	})

	AfterEach(func() {
		if cleanup != nil {
			cleanup()
		}
	})

	Describe("DirSize", func() {
		BeforeEach(func() {
			f, err := tfs.Create("/folder/subfolder/file2")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(f.Truncate(1 * 1024 * 1024)).To(Succeed()) // 1MB
		})
		It("Returns the expected size of a test folder", func() {
			size, err := sys.DirSize(tfs, "/folder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(size).To(Equal(int64(1*1024*1024 + 2048 + 1024)))
			usize, err := sys.DirSizeMB(tfs, "/folder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(usize).To(Equal(uint(2)))
		})
		It("Returns the size of a test folder when skipping subdirectories", func() {
			size, err := sys.DirSize(tfs, "/folder", "/folder/subfolder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(size).To(Equal(int64(1024)))
		})
		It("Fails with permission denied", func() {
			err := tfs.Chmod("/folder/subfolder", 0600)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = sys.DirSize(tfs, "/folder")
			Expect(err).Should(HaveOccurred())
			_, err = sys.DirSizeMB(tfs, "/folder")
			Expect(err).Should(HaveOccurred())
		})
	})
	Describe("IsDir", func() {
		It("discriminates directories and files", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())

			dir, err := sys.IsDir(tfs, "/folder")
			Expect(dir).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())

			dir, err = sys.IsDir(tfs, "/folder/subfolder/file1")
			Expect(dir).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())

			// does not follow symlinks
			dir, err = sys.IsDir(tfs, "/folder/linkToSubfolder")
			Expect(dir).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())

			// follows symlinks
			dir, err = sys.IsDir(tfs, "/folder/linkToSubfolder", true)
			Expect(dir).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())

			dir, err = sys.IsDir(tfs, "/nonexisting")
			Expect(dir).To(BeFalse())
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("RemoveAll", func() {
		It("Removes nested files and folders", func() {
			Expect(sys.RemoveAll(tfs, "/folder")).To(Succeed())
			Expect(sys.Exists(tfs, "/folder/subfolder")).To(BeFalse())
			Expect(sys.Exists(tfs, "/folder")).To(BeFalse())
		})
		It("Does not fail for nonexisting paths", func() {
			Expect(sys.Exists(tfs, "/non-existing")).To(BeFalse())
			Expect(sys.RemoveAll(tfs, "/non-existing")).To(Succeed())
		})
	})
	Describe("Exists", func() {
		It("Checks file existence as expected", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())
			Expect(tfs.Symlink("nonexisting", "/folder/brokenlink")).To(Succeed())

			Expect(sys.Exists(tfs, "/folder/subfolder")).To(BeTrue())
			Expect(sys.Exists(tfs, "/folder/subfolder/file1")).To(BeTrue())
			Expect(sys.Exists(tfs, "/folder/brokenlink")).To(BeTrue())
			Expect(sys.Exists(tfs, "/folder/brokenlink", true)).To(BeFalse())
			_, err := sys.Exists(tfs, "/folder/brokenlink", true)
			Expect(err).NotTo(HaveOccurred())
			Expect(sys.Exists(tfs, "/folder/linkToSubfolder")).To(BeTrue())
			Expect(sys.Exists(tfs, "/folder/linkToSubfolder", true)).To(BeTrue())
		})
	})
	Describe("ReadLink", func() {
		var osFS sys.FS
		It("Reads symlinks in TestFS", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())
			Expect(tfs.Symlink("nonexisting", "/folder/brokenlink")).To(Succeed())

			path, err := sys.ReadLink(tfs, "/folder/linkToSubfolder")
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("subfolder"))

			path, err = sys.ReadLink(tfs, "/folder/brokenlink")
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("nonexisting"))

			_, err = sys.ReadLink(tfs, "/folder/subfolder")
			Expect(err).To(HaveOccurred())
		})
		It("Reads symlinks in OSFS", func() {
			osFS = vfs.OSFS()
			tempDir, err := sys.TempDir(osFS, "", "testing")
			Expect(err).NotTo(HaveOccurred())
			defer sys.RemoveAll(osFS, tempDir)

			Expect(sys.MkdirAll(tfs, filepath.Join(tempDir, "subfolder"), sys.DirPerm)).To(Succeed())
			Expect(tfs.Symlink("subfolder", filepath.Join(tempDir, "linkToSubfolder"))).To(Succeed())
			Expect(tfs.Symlink("nonexisting", filepath.Join(tempDir, "brokenlink"))).To(Succeed())

			path, err := sys.ReadLink(tfs, filepath.Join(tempDir, "linkToSubfolder"))
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("subfolder"))

			path, err = sys.ReadLink(tfs, filepath.Join(tempDir, "brokenlink"))
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("nonexisting"))

			_, err = sys.ReadLink(tfs, tempDir)
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("TempDir", func() {
		var osFS sys.FS
		It("Creates a deterministic temporary directory on TestFS", func() {
			tempDir, err := sys.TempDir(tfs, "/customTmp", "testing")
			Expect(err).ToNot(HaveOccurred())
			Expect(tempDir).To(Equal("/customTmp/testing"))
		})
		It("Creates a randomized directory under os.TempDir with a deterministic prefix", func() {
			osFS = vfs.OSFS()
			tempDir, err := sys.TempDir(osFS, "", "testing")
			Expect(err).NotTo(HaveOccurred())
			defer sys.RemoveAll(osFS, tempDir)

			Expect(tempDir).NotTo(Equal(filepath.Join(os.TempDir(), "testing")))
			Expect(strings.HasPrefix(tempDir, filepath.Join(os.TempDir(), "testing"))).To(BeTrue())
		})
	})
	Describe("TempFile", func() {
		var osFS sys.FS
		It("Creates a randomized file with a deterministic prefix", func() {
			osFS = vfs.OSFS()
			tempFile, err := sys.TempFile(osFS, "", "testing")
			Expect(err).ToNot(HaveOccurred())
			defer sys.RemoveAll(osFS, tempFile.Name())
			Expect(tempFile.Name()).NotTo(Equal(filepath.Join(os.TempDir(), "testing")))
			Expect(strings.HasPrefix(tempFile.Name(), filepath.Join(os.TempDir(), "testing"))).To(BeTrue())
		})
	})
	Describe("WalkDirFs", func() {
		It("It walks through all the files in tree", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())
			Expect(tfs.Symlink("nonexisting", "/folder/brokenlink")).To(Succeed())

			currentPahts := []string{
				"/", "/folder", "/folder/brokenlink", "/folder/file",
				"/folder/linkToSubfolder", "/folder/subfolder", "/folder/subfolder/file1",
			}

			var foundPaths []string
			f := func(path string, _ fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				foundPaths = append(foundPaths, path)
				return err
			}
			sys.WalkDirFs(tfs, "/", f)
			Expect(len(foundPaths)).To(Equal(len(currentPahts)))
			Expect(foundPaths).To(Equal(currentPahts))
		})
	})
})
