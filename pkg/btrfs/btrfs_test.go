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

package btrfs_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestBtrfsSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Btrfs test suite")
}

var _ = Describe("DirectoryUnpacker", Label("directory"), func() {
	var tfs vfs.FS
	var s *sys.System
	var cleanup func()
	var err error
	var runner *sysmock.Runner
	BeforeEach(func() {
		runner = sysmock.NewRunner()
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithRunner(runner),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/etc", vfs.DirPerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("enables quota", func() {
		Expect(btrfs.EnableQuota(s, "/path/to/mountpoint")).To(Succeed())
		Expect(runner.IncludesCmds([][]string{{
			"btrfs", "quota", "enable", "/path/to/mountpoint",
		}})).To(Succeed())
	})
	It("creates a subvolume without copy on write", func() {
		Expect(btrfs.CreateSubvolume(s, "/path/to/subvolume", false)).To(Succeed())
		Expect(runner.IncludesCmds([][]string{
			{"btrfs", "subvolume", "create", "/path/to/subvolume"},
			{"chattr", "+C", "/path/to/subvolume"},
		})).To(Succeed())
	})
	It("creates a snapshot without copy on write", func() {
		Expect(btrfs.CreateSnapshot(s, "/path/to/new/subvolume", "/path/to/old/subvolume", false)).To(Succeed())
		Expect(runner.IncludesCmds([][]string{
			{"btrfs", "subvolume", "snapshot", "/path/to/old/subvolume", "/path/to/new/subvolume"},
			{"chattr", "+C", "/path/to/new/subvolume"},
		})).To(Succeed())
	})
	It("creates a quota group", func() {
		Expect(btrfs.CreateQuotaGroup(s, "/path/to/subvolume", "1/0")).To(Succeed())
		Expect(runner.IncludesCmds([][]string{
			{"btrfs", "qgroup", "create", "1/0", "/path/to/subvolume"},
		})).To(Succeed())
	})
	It("sets default subvolume", func() {
		Expect(btrfs.SetDefaultSubvolume(s, "/path/to/subvolume")).To(Succeed())
		Expect(runner.IncludesCmds([][]string{
			{"btrfs", "subvolume", "set-default", "/path/to/subvolume"},
		})).To(Succeed())
	})
	It("deletes subvolume", func() {
		Expect(btrfs.DeleteSubvolume(s, "/path/to/subvolume")).To(Succeed())
		Expect(runner.IncludesCmds([][]string{
			{"btrfs", "property", "set", "-ts", "/path/to/subvolume", "ro", "false"},
			{"btrfs", "subvolume", "delete", "-c", "-R", "/path/to/subvolume"},
		})).To(Succeed())
	})
	It("sets a btrfs partition", func() {
		Expect(btrfs.SetBtrfsPartition(s, "/path/to/mountpoint")).To(Succeed())
		Expect(runner.IncludesCmds([][]string{
			{"btrfs", "quota", "enable", "/path/to/mountpoint"},
			{"btrfs", "subvolume", "create", "/path/to/mountpoint/@"},
			{"btrfs", "qgroup", "create", "1/0", "/path/to/mountpoint"},
			{"btrfs", "subvolume", "set-default", "/path/to/mountpoint/@"},
		})).To(Succeed())
	})
})
