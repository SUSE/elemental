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

package merge_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/merge"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("FileChange", Label("merge"), func() {
	var (
		tfs     vfs.FS
		cleanup func()
	)

	BeforeEach(func() {
		var err error
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/old", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/new", vfs.DirPerm)).To(Succeed())
	})

	AfterEach(func() {
		cleanup()
	})

	It("returns empty change when both paths are absent", func() {
		change, err := merge.FileChange(tfs, "/old/nope", "/new/nope")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(BeEquivalentTo(""))
	})

	It("returns empty change when both files are identical", func() {
		Expect(tfs.WriteFile("/old/a", []byte("hello"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/a", []byte("hello"), vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/a", "/new/a")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(BeEquivalentTo(""))
	})

	It("reports Added when only the new path exists", func() {
		Expect(tfs.WriteFile("/new/added", []byte("v"), vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/added", "/new/added")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeAdded))
	})

	It("reports Deleted when only the old path exists", func() {
		Expect(tfs.WriteFile("/old/gone", []byte("v"), vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/gone", "/new/gone")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeDeleted))
	})

	It("ignores directory-only additions", func() {
		Expect(vfs.MkdirAll(tfs, "/new/newdir", vfs.DirPerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/newdir", "/new/newdir")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(BeEquivalentTo(""))
	})

	It("ignores directory-only deletions", func() {
		Expect(vfs.MkdirAll(tfs, "/old/gonedir", vfs.DirPerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/gonedir", "/new/gonedir")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(BeEquivalentTo(""))
	})

	It("reports Modified for different-sized regular files", func() {
		Expect(tfs.WriteFile("/old/grew", []byte("aa"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/grew", []byte("aaaa"), vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/grew", "/new/grew")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeModified))
	})

	It("detects edits that don't change file size", func() {
		// Same size (3 bytes each) means the size short-circuit says "equal";
		// only the byte-for-byte compare catches this.
		Expect(tfs.WriteFile("/old/cfg", []byte("abc"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/cfg", []byte("xyz"), vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/cfg", "/new/cfg")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeModified))
	})

	It("detects symlink target changes", func() {
		Expect(tfs.Symlink("/old/target-a", "/old/link")).To(Succeed())
		Expect(tfs.Symlink("/new/target-b", "/new/link")).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/link", "/new/link")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeModified))
	})

	It("returns empty change when symlink targets match", func() {
		Expect(tfs.Symlink("/somewhere", "/old/link")).To(Succeed())
		Expect(tfs.Symlink("/somewhere", "/new/link")).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/link", "/new/link")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(BeEquivalentTo(""))
	})

	It("reports type changes (dir -> file) as Modified", func() {
		Expect(vfs.MkdirAll(tfs, "/old/x", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/x", []byte("now a file"), vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/x", "/new/x")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeModified))
	})

	It("flags same-size files above the cap as Modified without reading them", func() {
		// Identical bytes on both sides. Without the cap the content compare
		// would report "equal" (no change); reporting Modified confirms the
		// cap branch fired instead.
		big := make([]byte, merge.MaxContentCompareSize+1)
		Expect(tfs.WriteFile("/old/huge", big, vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/huge", big, vfs.FilePerm)).To(Succeed())

		change, err := merge.FileChange(tfs, "/old/huge", "/new/huge")
		Expect(err).NotTo(HaveOccurred())
		Expect(change).To(Equal(merge.ChangeTypeModified))
	})
})

var _ = Describe("FormatConflictSummary", Label("merge"), func() {
	It("returns empty string when no conflicts", func() {
		Expect(merge.FormatConflictSummary("/etc", nil)).To(Equal(""))
		Expect(merge.FormatConflictSummary("/etc", []merge.Conflict{})).To(Equal(""))
	})

	It("formats a summary with conflicts", func() {
		conflicts := []merge.Conflict{
			{Path: "/etc/foo", UserChange: merge.ChangeTypeModified, OSChange: merge.ChangeTypeModified},
			{Path: "/etc/bar", UserChange: merge.ChangeTypeDeleted, OSChange: merge.ChangeTypeModified},
		}
		summary := merge.FormatConflictSummary("/etc", conflicts)
		Expect(summary).To(ContainSubstring("Merge conflicts detected for /etc (2 file(s))"))
		Expect(summary).To(ContainSubstring("/etc/foo — user: modified, OS: modified"))
		Expect(summary).To(ContainSubstring("/etc/bar — user: deleted, OS: modified"))
	})
})
