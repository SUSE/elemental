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

package vfs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestVfsSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vfs test suite")
}

var _ = Describe("FS", Label("fs"), func() {
	var tfs vfs.FS
	var cleanup func()
	var err error

	BeforeEach(func() {
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(vfs.MkdirAll(tfs, "/folder/subfolder", vfs.DirPerm)).To(Succeed())
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
			size, err := vfs.DirSize(tfs, "/folder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(size).To(Equal(int64(1*1024*1024 + 2048 + 1024)))
			usize, err := vfs.DirSizeMB(tfs, "/folder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(usize).To(Equal(uint(2)))
		})
		It("Returns the size of a test folder when skipping subdirectories", func() {
			size, err := vfs.DirSize(tfs, "/folder", "/folder/subfolder")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(size).To(Equal(int64(1024)))
		})
		It("Fails with permission denied", func() {
			err := tfs.Chmod("/folder/subfolder", 0600)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = vfs.DirSize(tfs, "/folder")
			Expect(err).Should(HaveOccurred())
			_, err = vfs.DirSizeMB(tfs, "/folder")
			Expect(err).Should(HaveOccurred())
		})
	})
	Describe("IsDir", func() {
		It("discriminates directories and files", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())

			dir, err := vfs.IsDir(tfs, "/folder")
			Expect(dir).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())

			dir, err = vfs.IsDir(tfs, "/folder/subfolder/file1")
			Expect(dir).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())

			// does not follow symlinks
			dir, err = vfs.IsDir(tfs, "/folder/linkToSubfolder")
			Expect(dir).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())

			// follows symlinks
			dir, err = vfs.IsDir(tfs, "/folder/linkToSubfolder", true)
			Expect(dir).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())

			dir, err = vfs.IsDir(tfs, "/nonexisting")
			Expect(dir).To(BeFalse())
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("RemoveAll", func() {
		It("Removes nested files and folders", func() {
			Expect(vfs.RemoveAll(tfs, "/folder")).To(Succeed())
			Expect(vfs.Exists(tfs, "/folder/subfolder")).To(BeFalse())
			Expect(vfs.Exists(tfs, "/folder")).To(BeFalse())
		})
		It("Does not fail for nonexisting paths", func() {
			Expect(vfs.Exists(tfs, "/non-existing")).To(BeFalse())
			Expect(vfs.RemoveAll(tfs, "/non-existing")).To(Succeed())
		})
	})
	Describe("Exists", func() {
		It("Checks file existence as expected", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())
			Expect(tfs.Symlink("nonexisting", "/folder/brokenlink")).To(Succeed())

			Expect(vfs.Exists(tfs, "/folder/subfolder")).To(BeTrue())
			Expect(vfs.Exists(tfs, "/folder/subfolder/file1")).To(BeTrue())
			Expect(vfs.Exists(tfs, "/folder/brokenlink")).To(BeTrue())
			Expect(vfs.Exists(tfs, "/folder/brokenlink", true)).To(BeFalse())
			_, err := vfs.Exists(tfs, "/folder/brokenlink", true)
			Expect(err).NotTo(HaveOccurred())
			Expect(vfs.Exists(tfs, "/folder/linkToSubfolder")).To(BeTrue())
			Expect(vfs.Exists(tfs, "/folder/linkToSubfolder", true)).To(BeTrue())
		})
	})
	Describe("ReadLink", func() {
		var osFS vfs.FS
		It("Reads symlinks in TestFS", func() {
			Expect(tfs.Symlink("subfolder", "/folder/linkToSubfolder")).To(Succeed())
			Expect(tfs.Symlink("nonexisting", "/folder/brokenlink")).To(Succeed())

			path, err := vfs.ReadLink(tfs, "/folder/linkToSubfolder")
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("subfolder"))

			path, err = vfs.ReadLink(tfs, "/folder/brokenlink")
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("nonexisting"))

			_, err = vfs.ReadLink(tfs, "/folder/subfolder")
			Expect(err).To(HaveOccurred())
		})
		It("Reads symlinks in OSFS", func() {
			osFS = vfs.New()
			tempDir, err := vfs.TempDir(osFS, "", "testing")
			Expect(err).NotTo(HaveOccurred())
			defer vfs.RemoveAll(osFS, tempDir)

			Expect(vfs.MkdirAll(tfs, filepath.Join(tempDir, "subfolder"), vfs.DirPerm)).To(Succeed())
			Expect(tfs.Symlink("subfolder", filepath.Join(tempDir, "linkToSubfolder"))).To(Succeed())
			Expect(tfs.Symlink("nonexisting", filepath.Join(tempDir, "brokenlink"))).To(Succeed())

			path, err := vfs.ReadLink(tfs, filepath.Join(tempDir, "linkToSubfolder"))
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("subfolder"))

			path, err = vfs.ReadLink(tfs, filepath.Join(tempDir, "brokenlink"))
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("nonexisting"))

			_, err = vfs.ReadLink(tfs, tempDir)
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("TempDir", func() {
		var osFS vfs.FS
		It("Creates a deterministic temporary directory on TestFS", func() {
			tempDir, err := vfs.TempDir(tfs, "/customTmp", "testing")
			Expect(err).ToNot(HaveOccurred())
			Expect(tempDir).To(Equal("/customTmp/testing"))
		})
		It("Creates a randomized directory under os.TempDir with a deterministic prefix", func() {
			osFS = vfs.New()
			tempDir, err := vfs.TempDir(osFS, "", "testing")
			Expect(err).NotTo(HaveOccurred())
			defer vfs.RemoveAll(osFS, tempDir)

			Expect(tempDir).NotTo(Equal(filepath.Join(os.TempDir(), "testing")))
			Expect(strings.HasPrefix(tempDir, filepath.Join(os.TempDir(), "testing"))).To(BeTrue())
		})
	})
	Describe("TempFile", func() {
		var osFS vfs.FS
		It("Creates a randomized file with a deterministic prefix", func() {
			osFS = vfs.New()
			tempFile, err := vfs.TempFile(osFS, "", "testing")
			Expect(err).ToNot(HaveOccurred())
			defer vfs.RemoveAll(osFS, tempFile.Name())
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
			vfs.WalkDirFs(tfs, "/", f)
			Expect(len(foundPaths)).To(Equal(len(currentPahts)))
			Expect(foundPaths).To(Equal(currentPahts))
		})
	})
	Describe("CopyFile", func() {
		It("Copies source file to target file", func() {
			err := vfs.MkdirAll(tfs, "/some", vfs.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = tfs.Create("/some/file")
			Expect(err).ShouldNot(HaveOccurred())
			_, err = tfs.Stat("/some/otherfile")
			Expect(err).Should(HaveOccurred())
			Expect(vfs.CopyFile(tfs, "/some/file", "/some/otherfile")).ShouldNot(HaveOccurred())
			e, err := vfs.Exists(tfs, "/some/otherfile")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(e).To(BeTrue())
		})
		It("Copies source file to target folder", func() {
			err := vfs.MkdirAll(tfs, "/some", vfs.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			err = vfs.MkdirAll(tfs, "/someotherfolder", vfs.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = tfs.Create("/some/file")
			Expect(err).ShouldNot(HaveOccurred())
			_, err = tfs.Stat("/someotherfolder/file")
			Expect(err).Should(HaveOccurred())
			Expect(vfs.CopyFile(tfs, "/some/file", "/someotherfolder")).ShouldNot(HaveOccurred())
			e, err := vfs.Exists(tfs, "/someotherfolder/file")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(e).To(BeTrue())
		})
		It("Fails to open non existing file", func() {
			err := vfs.MkdirAll(tfs, "/some", vfs.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(vfs.CopyFile(tfs, "/some/file", "/some/otherfile")).NotTo(BeNil())
			_, err = tfs.Stat("/some/otherfile")
			Expect(err).NotTo(BeNil())
		})
		It("Fails to copy on non writable target", func() {
			err := vfs.MkdirAll(tfs, "/some", vfs.DirPerm)
			Expect(err).ShouldNot(HaveOccurred())
			tfs.Create("/some/file")
			_, err = tfs.Stat("/some/otherfile")
			Expect(err).NotTo(BeNil())
			tfs, err = sysmock.ReadOnlyTestFS(tfs)
			Expect(err).NotTo(HaveOccurred())
			Expect(vfs.CopyFile(tfs, "/some/file", "/some/otherfile")).NotTo(BeNil())
			_, err = tfs.Stat("/some/otherfile")
			Expect(err).NotTo(BeNil())
		})
	})
	Describe("ResolveLink", func() {
		var rootDir, file, relSymlink, absSymlink, nestSymlink, brokenSymlink string

		BeforeEach(func() {
			// The root directory
			rootDir = "/some/root"
			Expect(vfs.MkdirAll(tfs, rootDir, vfs.DirPerm)).To(Succeed())

			// The target file of all symlinks
			file = "/path/with/needle/findme.extension"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(file)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(filepath.Join(rootDir, file), []byte("some data"), vfs.FilePerm)).To(Succeed())

			// A symlink pointing to a relative path
			relSymlink = "/path/to/symlink/pointing-to-file"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(relSymlink)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.Symlink("../../with/needle/findme.extension", filepath.Join(rootDir, relSymlink))).To(Succeed())

			// A symlink pointing to an absolute path
			absSymlink = "/path/to/symlink/absolute-pointer"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(absSymlink)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.Symlink(file, filepath.Join(rootDir, absSymlink))).To(Succeed())

			// A bunch of nested symlinks
			nestSymlink = "/path/to/symlink/nested-pointer"
			nestFst := "/path/to/symlink/nestFst"
			nest2nd := "/path/to/nest2nd"
			nest3rd := "/path/with/nest3rd"
			Expect(tfs.Symlink("nestFst", filepath.Join(rootDir, nestSymlink))).To(Succeed())
			Expect(tfs.Symlink(nest2nd, filepath.Join(rootDir, nestFst))).To(Succeed())
			Expect(tfs.Symlink("../with/nest3rd", filepath.Join(rootDir, nest2nd))).To(Succeed())
			Expect(tfs.Symlink("./needle/findme.extension", filepath.Join(rootDir, nest3rd))).To(Succeed())

			// A broken symlink
			brokenSymlink = "/path/to/symlink/broken"
			Expect(tfs.Symlink("/path/to/nowhere", filepath.Join(rootDir, brokenSymlink))).To(Succeed())
		})

		It("resolves a simple relative symlink", func() {
			systemPath := filepath.Join(rootDir, relSymlink)
			Expect(vfs.ResolveLink(tfs, systemPath, rootDir, vfs.MaxLinkDepth)).To(Equal(filepath.Join(rootDir, file)))
		})

		It("resolves a simple absolute symlink", func() {
			systemPath := filepath.Join(rootDir, absSymlink)
			Expect(vfs.ResolveLink(tfs, systemPath, rootDir, vfs.MaxLinkDepth)).To(Equal(filepath.Join(rootDir, file)))
		})

		It("resolves some nested symlinks", func() {
			systemPath := filepath.Join(rootDir, nestSymlink)
			Expect(vfs.ResolveLink(tfs, systemPath, rootDir, vfs.MaxLinkDepth)).To(Equal(filepath.Join(rootDir, file)))
		})

		It("does not resolve broken links", func() {
			systemPath := filepath.Join(rootDir, brokenSymlink)
			// Return the symlink path without resolving it
			resolved, err := vfs.ResolveLink(tfs, systemPath, rootDir, vfs.MaxLinkDepth)
			Expect(resolved).To(Equal(systemPath))
			Expect(err).To(HaveOccurred())
		})

		It("does not resolve too many levels of netsed links", func() {
			systemPath := filepath.Join(rootDir, nestSymlink)
			// Returns the symlink resolution up to the second level
			resolved, err := vfs.ResolveLink(tfs, systemPath, rootDir, 2)
			Expect(resolved).To(Equal(filepath.Join(rootDir, "/path/to/nest2nd")))
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("FindFile", func() {
		var rootDir, file1, file2, relSymlink string

		BeforeEach(func() {
			// The root directory
			rootDir = "/some/root"
			Expect(vfs.MkdirAll(tfs, rootDir, vfs.DirPerm)).To(Succeed())

			// Files to find
			file1 = "/path/with/needle/findme.extension"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(file1)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(filepath.Join(rootDir, file1), []byte("some data"), vfs.FilePerm)).To(Succeed())
			file2 = "/path/with/needle.aarch64/findme.ext"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(file2)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(filepath.Join(rootDir, file2), []byte("some data"), vfs.FilePerm)).To(Succeed())

			// A symlink pointing to a relative path
			relSymlink = "/path/to/symlink/pointing-to-file"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(relSymlink)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.Symlink("../../with/needle/findme.extension", filepath.Join(rootDir, relSymlink))).To(Succeed())
		})
		It("finds a matching file, first match wins file1", func() {
			f, err := vfs.FindFile(tfs, rootDir, "/path/with/*dle*/*me.*", "/path/with/*aarch64/find*")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(f).To(Equal(filepath.Join(rootDir, file1)))
		})
		It("finds a matching file, first match wins file2", func() {
			f, err := vfs.FindFile(tfs, rootDir, "/path/with/*aarch64/find*", "/path/with/*dle*/*me.*")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(f).To(Equal(filepath.Join(rootDir, file2)))
		})
		It("finds a matching file and resolves the link", func() {
			f, err := vfs.FindFile(tfs, rootDir, "/path/*/symlink/pointing-to-*", "/path/with/*aarch64/find*")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(f).To(Equal(filepath.Join(rootDir, file1)))
		})
		It("fails if there is no match", func() {
			_, err := vfs.FindFile(tfs, rootDir, "/path/*/symlink/*no-match-*")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("failed to find"))
		})
		It("fails on invalid parttern", func() {
			_, err := vfs.FindFile(tfs, rootDir, "/path/*/symlink/badformat[]")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("syntax error"))
		})
	})
	Describe("FindFiles", func() {
		var rootDir, file1, file2, file3, relSymlink string
		BeforeEach(func() {
			// The root directory
			rootDir = "/some/root"
			Expect(vfs.MkdirAll(tfs, rootDir, vfs.DirPerm)).To(Succeed())

			// Files to find
			file1 = "/path/with/needle/findme.extension1"
			file2 = "/path/with/needle/findme.extension2"
			file3 = "/path/with/needle/hardtofindme"
			Expect(vfs.MkdirAll(tfs, filepath.Join(rootDir, filepath.Dir(file1)), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(filepath.Join(rootDir, file1), []byte("file1"), vfs.FilePerm)).To(Succeed())
			Expect(tfs.WriteFile(filepath.Join(rootDir, file2), []byte("file2"), vfs.FilePerm)).To(Succeed())
			Expect(tfs.WriteFile(filepath.Join(rootDir, file3), []byte("file3"), vfs.FilePerm)).To(Succeed())

			// A symlink pointing to a relative path
			relSymlink = "/path/with/needle/findme.symlink"
			Expect(tfs.Symlink("hardtofindme", filepath.Join(rootDir, relSymlink))).To(Succeed())
		})
		It("finds all matching files", func() {
			f, err := vfs.FindFiles(tfs, rootDir, "/path/with/*dle*/find*")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(f)).To(Equal(3))
			Expect(f).Should(ContainElement(filepath.Join(rootDir, file1)))
			Expect(f).Should(ContainElement(filepath.Join(rootDir, file2)))
			Expect(f).Should(ContainElement(filepath.Join(rootDir, file3)))
		})
		It("finds all matching files when pattern starts with magic character", func() {
			f, err := vfs.FindFiles(tfs, rootDir, "*/with/*dle*/find*")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(f)).To(Equal(3))
			Expect(f).Should(ContainElement(filepath.Join(rootDir, file1)))
			Expect(f).Should(ContainElement(filepath.Join(rootDir, file2)))
			Expect(f).Should(ContainElement(filepath.Join(rootDir, file3)))
		})
		It("Returns empty list if there is no match", func() {
			f, err := vfs.FindFiles(tfs, rootDir, "/path/with/needle/notthere*")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(len(f)).Should(Equal(0))
		})
	})
})
