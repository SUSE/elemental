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

var _ = Describe("DetectConflicts", Label("merge"), func() {
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

	It("returns no conflicts when user changes don't overlap the OS delta", func() {
		// OS modified /os-only; user touched a different path.
		Expect(tfs.WriteFile("/old/os-only", []byte("v1"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/os-only", []byte("v2"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/user-only": merge.ChangeTypeAdded}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(BeEmpty())
	})

	It("returns no conflicts when the OS tree is unchanged", func() {
		Expect(tfs.WriteFile("/old/shared", []byte("hello"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/shared", []byte("hello"), vfs.FilePerm)).To(Succeed())
		// User modified it, but the OS did not; not a conflict.
		userChanges := map[string]merge.ChangeType{"/shared": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(BeEmpty())
	})

	It("flags a both-modified file", func() {
		Expect(tfs.WriteFile("/old/shared", []byte("baseline"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/shared", []byte("os-updated"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/shared": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/shared",
			UserChange: merge.ChangeTypeModified,
			OSChange:   merge.ChangeTypeModified,
		}))
	})

	It("flags OS-deleted vs user-modified", func() {
		Expect(tfs.WriteFile("/old/gone", []byte("v1"), vfs.FilePerm)).To(Succeed())
		// /new/gone is absent — OS removed it
		userChanges := map[string]merge.ChangeType{"/gone": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/gone",
			UserChange: merge.ChangeTypeModified,
			OSChange:   merge.ChangeTypeDeleted,
		}))
	})

	It("flags both-added paths", func() {
		Expect(tfs.WriteFile("/new/added", []byte("v"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/added": merge.ChangeTypeAdded}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/added",
			UserChange: merge.ChangeTypeAdded,
			OSChange:   merge.ChangeTypeAdded,
		}))
	})

	It("returns conflicts sorted by path", func() {
		Expect(tfs.WriteFile("/old/a.conf", []byte("v1"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/a.conf", []byte("v2"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/m.conf", []byte("m"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/old/z.conf", []byte("v1"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/z.conf", []byte("v2"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{
			"/z.conf": merge.ChangeTypeModified,
			"/a.conf": merge.ChangeTypeDeleted,
			"/m.conf": merge.ChangeTypeAdded,
		}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(Equal([]merge.Conflict{
			{Path: "/a.conf", UserChange: merge.ChangeTypeDeleted, OSChange: merge.ChangeTypeModified},
			{Path: "/m.conf", UserChange: merge.ChangeTypeAdded, OSChange: merge.ChangeTypeAdded},
			{Path: "/z.conf", UserChange: merge.ChangeTypeModified, OSChange: merge.ChangeTypeModified},
		}))
	})

	It("detects edits that don't change file size", func() {
		// Both files are the same length -- the size check short-circuits to "equal"
		// here, so reaching the conflict requires the content compare to
		// notice the bytes actually differ.
		Expect(tfs.WriteFile("/old/cfg", []byte("abc"), vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/cfg", []byte("xyz"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/cfg": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/cfg",
			UserChange: merge.ChangeTypeModified,
			OSChange:   merge.ChangeTypeModified,
		}))
	})

	It("detects symlink target changes", func() {
		Expect(tfs.Symlink("/old/target-a", "/old/link")).To(Succeed())
		Expect(tfs.Symlink("/new/target-b", "/new/link")).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/link": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/link",
			UserChange: merge.ChangeTypeModified,
			OSChange:   merge.ChangeTypeModified,
		}))
	})

	It("reports type changes (dir→file) as modified", func() {
		// /x existed in old as a directory; the new image replaced it with
		// a regular file. The user independently edited the directory's
		// stand-in (e.g. dropped a file into it that maps to the same rel
		// path), so we expect a both-modified conflict.
		Expect(vfs.MkdirAll(tfs, "/old/x", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/x", []byte("now a file"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/x": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/x",
			UserChange: merge.ChangeTypeModified,
			OSChange:   merge.ChangeTypeModified,
		}))
	})

	It("does not descend into nested .snapshots directories", func() {
		// snapper-style nested snapshot tree: the walk must skip it even when
		// its contents differ wildly between old and new.
		Expect(vfs.MkdirAll(tfs, "/old/.snapshots/1/snapshot", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/old/.snapshots/1/snapshot/inner", []byte("old"), vfs.FilePerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, "/new/.snapshots/9/snapshot", vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/.snapshots/9/snapshot/inner", []byte("different"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/.snapshots/1/snapshot/inner": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(BeEmpty())
	})

	It("flags same-size files above the cap as modified", func() {
		// Both files hold identical bytes. Without the size cap the content
		// compare would return "equal" and there would be no conflict; a
		// conflict appearing confirms the cap branch fired instead.
		big := make([]byte, merge.MaxContentCompareSize+1)
		Expect(tfs.WriteFile("/old/huge", big, vfs.FilePerm)).To(Succeed())
		Expect(tfs.WriteFile("/new/huge", big, vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/huge": merge.ChangeTypeModified}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/old", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/huge",
			UserChange: merge.ChangeTypeModified,
			OSChange:   merge.ChangeTypeModified,
		}))
	})

	It("treats a missing old root as an empty tree", func() {
		// With no old tree, every file in /new is an addition. A user file
		// at the same path collides → both-added conflict, exactly as if the
		// old tree existed and was empty.
		Expect(tfs.WriteFile("/new/a", []byte("a"), vfs.FilePerm)).To(Succeed())
		userChanges := map[string]merge.ChangeType{"/a": merge.ChangeTypeAdded}

		conflicts, err := merge.DetectConflicts(tfs, userChanges, "/does-not-exist", "/new")
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicts).To(ConsistOf(merge.Conflict{
			Path:       "/a",
			UserChange: merge.ChangeTypeAdded,
			OSChange:   merge.ChangeTypeAdded,
		}))
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
