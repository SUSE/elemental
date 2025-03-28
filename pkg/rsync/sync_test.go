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

package rsync_test

import (
	"bytes"
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/rsync"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestRsyncSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rsync test suite")
}

func getNamesFromListFiles(list []fs.DirEntry) []string {
	var names []string
	for _, f := range list {
		names = append(names, f.Name())
	}
	return names
}

var _ = Describe("Sync tests", Label("rsync"), func() {
	var sourceDir, destDir string
	var err error
	var tfs vfs.FS
	var s *sys.System
	var logger log.Logger
	var cleanup func()
	var memLog *bytes.Buffer
	BeforeEach(func() {
		memLog = &bytes.Buffer{}
		logger = log.New(log.WithBuffer(memLog))
		logger.SetLevel(log.DebugLevel())

		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(sys.WithFS(tfs), sys.WithLogger(logger))
		Expect(err).NotTo(HaveOccurred())
		s.Logger().SetLevel(log.DebugLevel())
		sourceDir, err = vfs.TempDir(tfs, "", "elementalsource")
		Expect(err).ShouldNot(HaveOccurred())
		destDir, err = vfs.TempDir(tfs, "", "elementaltarget")
		Expect(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Copies all files from source to target", func() {
		for range 5 {
			_, _ = vfs.TempFile(tfs, sourceDir, "file*")
		}

		r := rsync.NewRsync(s)
		Expect(r.SyncData(sourceDir, destDir)).To(BeNil())

		filesDest, err := tfs.ReadDir(destDir)
		Expect(err).To(BeNil())

		destNames := getNamesFromListFiles(filesDest)
		filesSource, err := tfs.ReadDir(sourceDir)
		Expect(err).To(BeNil())

		SourceNames := getNamesFromListFiles(filesSource)

		// Should be the same files in both dirs now
		Expect(destNames).To(Equal(SourceNames))
	})

	It("Copies all files from source to target respecting excludes", func() {
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "host"), vfs.DirPerm)
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "run"), vfs.DirPerm)

		// /tmp/run would be excluded as well, as we define an exclude without the "/" prefix
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "tmp", "run"), vfs.DirPerm)

		for range 5 {
			_, _ = vfs.TempFile(tfs, sourceDir, "file*")
		}

		r := rsync.NewRsync(s)
		Expect(r.SyncData(sourceDir, destDir, "host", "run")).To(BeNil())

		filesDest, err := tfs.ReadDir(destDir)
		Expect(err).To(BeNil())

		destNames := getNamesFromListFiles(filesDest)

		filesSource, err := tfs.ReadDir(sourceDir)
		Expect(err).To(BeNil())

		sourceNames := getNamesFromListFiles(filesSource)

		// Shouldn't be the same
		Expect(destNames).ToNot(Equal(sourceNames))

		Expect(slices.Contains(destNames, "host")).To(BeFalse())
		Expect(slices.Contains(destNames, "run")).To(BeFalse())

		Expect(slices.Contains(sourceNames, "host")).To(BeTrue())
		Expect(slices.Contains(sourceNames, "run")).To(BeTrue())

		// /tmp/run is not copied over
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "tmp", "run"))).To(BeFalse())
	})

	It("Copies all files from source to target respecting excludes with '/' prefix", func() {
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "host"), vfs.DirPerm)
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "run"), vfs.DirPerm)
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "var", "run"), vfs.DirPerm)
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "tmp", "host"), vfs.DirPerm)

		r := rsync.NewRsync(s)
		Expect(r.SyncData(sourceDir, destDir, "/host", "/run")).To(BeNil())

		filesDest, err := tfs.ReadDir(destDir)
		Expect(err).To(BeNil())
		destNames := getNamesFromListFiles(filesDest)

		filesSource, err := tfs.ReadDir(sourceDir)
		Expect(err).To(BeNil())
		sourceNames := getNamesFromListFiles(filesSource)

		// Shouldn't be the same
		Expect(destNames).ToNot(Equal(sourceNames))

		Expect(vfs.Exists(tfs, filepath.Join(destDir, "var", "run"))).To(BeTrue())
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "tmp", "host"))).To(BeTrue())
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "host"))).To(BeFalse())
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "run"))).To(BeFalse())
	})

	It("Copies all files from source to target respecting excludes with wildcards", func() {
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "run"), vfs.DirPerm)
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "var", "run"), vfs.DirPerm)
		Expect(tfs.WriteFile(filepath.Join(sourceDir, "run", "testfile"), []byte{}, vfs.DirPerm)).To(Succeed())

		r := rsync.NewRsync(s)
		Expect(r.SyncData(sourceDir, destDir, "/run/*")).To(BeNil())

		Expect(vfs.Exists(tfs, filepath.Join(destDir, "var", "run"))).To(BeTrue())
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "run"))).To(BeTrue())
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "run", "testfile"))).To(BeFalse())
	})

	It("Mirrors all files from source to destination deleting pre-existing files in destination if needed", func() {
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "run"), vfs.DirPerm)
		vfs.MkdirAll(tfs, filepath.Join(sourceDir, "var", "run"), vfs.DirPerm)
		Expect(tfs.WriteFile(filepath.Join(sourceDir, "run", "testfile"), []byte{}, vfs.DirPerm)).To(Succeed())
		Expect(tfs.WriteFile(filepath.Join(destDir, "testfile"), []byte{}, vfs.DirPerm)).To(Succeed())

		r := rsync.NewRsync(s)
		Expect(r.MirrorData(sourceDir, destDir)).To(BeNil())

		filesDest, err := tfs.ReadDir(destDir)
		Expect(err).To(BeNil())
		destNames := getNamesFromListFiles(filesDest)

		filesSource, err := tfs.ReadDir(sourceDir)
		Expect(err).To(BeNil())
		sourceNames := getNamesFromListFiles(filesSource)

		// Should be the same
		Expect(destNames).To(Equal(sourceNames))

		// pre-exising file in destination deleted if this is not part of source
		Expect(vfs.Exists(tfs, filepath.Join(destDir, "testfile"))).To(BeFalse())
	})

	It("should not fail if dirs are empty", func() {
		Expect(rsync.NewRsync(s).SyncData(sourceDir, destDir)).To(BeNil())
	})
	It("should not fail if destination does not exist", func() {
		Expect(rsync.NewRsync(s).SyncData(sourceDir, "/welp")).To(Succeed())
		exists, err := vfs.Exists(tfs, "/welp")
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
	})
	It("should fail if source does not exist", func() {
		Expect(rsync.NewRsync(s).SyncData("/welp", destDir)).NotTo(BeNil())
	})
	It("uses context to cancel an inprogress sync and logs progress", func() {
		Expect(vfs.MkdirAll(tfs, filepath.Join(sourceDir, "subfolder"), vfs.DirPerm)).To(Succeed())
		f, err := tfs.Create(filepath.Join(sourceDir, "subfolder/file2"))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(f.Truncate(10 * 1024 * 1024)).To(Succeed()) // 10MB
		ctx, cancel := context.WithCancel(context.Background())

		var wg sync.WaitGroup
		r := rsync.NewRsync(
			s, rsync.WithFlags(append(rsync.DefaultFlags(), "--bwlimit=1M")...),
			rsync.WithContext(ctx),
		)
		wg.Add(1)
		go func() {
			err = r.SyncData(sourceDir, destDir)
			wg.Done()
		}()
		time.Sleep(1 * time.Second)
		cancel()
		wg.Wait()
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("killed"))
		// Check there are progress messages
		Expect(memLog.String()).To(ContainSubstring("synchronizing:"))
	})
})
