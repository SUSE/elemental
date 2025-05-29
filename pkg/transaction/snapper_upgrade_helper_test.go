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

package transaction_test

import (
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
)

var _ = Describe("SnapperUpgradeHelper", Label("transaction"), func() {
	var root string
	var trans *transaction.Transaction
	var upgradeH transaction.UpgradeHelper
	BeforeEach(func() {
		snapperContextMock()
	})
	AfterEach(func() {
		cleanup()
	})
	Describe("upgrade helper for an install transaction", func() {
		BeforeEach(func() {
			root = "/some/root"
			upgradeH = initSnapperInstall(root)
			trans = startInstallTransaction()
		})
		It("Syncs the source image", func() {
			Expect(upgradeH.SyncImageContent(imgsrc, trans)).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"rsync", "--info=progress2", "--human-readable"},
			})).To(Succeed())
		})
		It("fails to sync the source image", func() {
			sideEffects["rsync"] = func(args ...string) ([]byte, error) {
				return []byte{}, fmt.Errorf("rsync error")
			}
			Expect(upgradeH.SyncImageContent(imgsrc, trans)).NotTo(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"rsync", "--info=progress2", "--human-readable"},
			})).To(Succeed())
		})
		It("configures snapper and merges RW volumes", func() {
			snapshotP := ".snapshots/1/snapshot"
			snTemplate := "/usr/share/snapper/config-templates/default"
			snSysConf := filepath.Join(root, btrfs.TopSubVol, snapshotP, "/etc/sysconfig/snapper")
			template := filepath.Join(root, btrfs.TopSubVol, snapshotP, snTemplate)
			configsDir := filepath.Join(root, btrfs.TopSubVol, snapshotP, "/etc/snapper/configs")

			Expect(vfs.MkdirAll(tfs, configsDir, vfs.DirPerm)).To(Succeed())
			Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
			Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(template, []byte{}, vfs.FilePerm)).To(Succeed())
			Expect(vfs.MkdirAll(tfs, filepath.Dir(snSysConf), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(snSysConf, []byte{}, vfs.FilePerm)).To(Succeed())

			Expect(upgradeH.Merge(trans)).To(Succeed())
			// Snapper configuration is done before merging
			Expect(runner.MatchMilestones([][]string{
				{"snapper", "--no-dbus", "-c", "etc", "create-config", "--fstype", "btrfs", "/etc"},
				{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "etc", "create", "--print-number"},
				{"snapper", "--no-dbus", "-c", "home", "create-config", "--fstype", "btrfs", "/home"},
				{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "home", "create", "--print-number"},
			})).To(Succeed())
			// No merge is executed on first (install) transaction
			Expect(runner.MatchMilestones([][]string{
				{"rsync"},
			})).NotTo(Succeed())
		})
		It("fails to create snapper configuration if templates are not found", func() {
			err = upgradeH.Merge(trans)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find file matching"))
		})
		It("creates fstab", func() {
			path := filepath.Join(root, btrfs.TopSubVol, ".snapshots/1/snapshot/etc")
			Expect(vfs.MkdirAll(tfs, path, vfs.DirPerm)).To(Succeed())

			fstab := filepath.Join(trans.Path, transaction.FstabFile)
			ok, _ := vfs.Exists(tfs, fstab)
			Expect(ok).To(BeFalse())
			Expect(upgradeH.UpdateFstab(trans)).To(Succeed())
			ok, _ = vfs.Exists(tfs, fstab)
			Expect(ok).To(BeTrue())
		})
		It("it fails to create fstab file if the path does not exist", func() {
			Expect(upgradeH.UpdateFstab(trans)).NotTo(Succeed())
		})
		It("locks the current transaction", func() {
			Expect(upgradeH.Lock(trans)).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--read-only"},
			})).To(Succeed())
		})
		It("fails to lock the current transaction", func() {
			sideEffects["snapper"] = func(args ...string) ([]byte, error) {
				return []byte{}, fmt.Errorf("snapper error")
			}
			Expect(upgradeH.Lock(trans)).NotTo(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--read-only"},
			})).To(Succeed())
		})
	})
	Describe("upgrade helper for an ugrade] transaction", func() {
		BeforeEach(func() {
			root = "/"
			upgradeH = initSnapperUpgrade(root)
			trans = startUpgradeTransaction()
		})
		It("configures snapper and merges RW volumes", func() {
			snapshotP := ".snapshots/5/snapshot"
			snTemplate := "/usr/share/snapper/config-templates/default"
			snSysConf := filepath.Join(root, snapshotP, "/etc/sysconfig/snapper")
			template := filepath.Join(root, snapshotP, snTemplate)
			configsDir := filepath.Join(root, snapshotP, "/etc/snapper/configs")

			Expect(vfs.MkdirAll(tfs, configsDir, vfs.DirPerm)).To(Succeed())
			Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
			Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(template, []byte{}, vfs.FilePerm)).To(Succeed())
			Expect(vfs.MkdirAll(tfs, filepath.Dir(snSysConf), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(snSysConf, []byte{}, vfs.FilePerm)).To(Succeed())

			Expect(upgradeH.Merge(trans)).To(Succeed())
			Expect(runner.MatchMilestones([][]string{
				{"snapper", "--no-dbus", "-c", "etc", "create-config", "--fstype", "btrfs", "/etc"},
				{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "etc", "create", "--print-number"},
				{"snapper", "--no-dbus", "-c", "home", "create-config", "--fstype", "btrfs", "/home"},
				{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "home", "create", "--print-number"},
				{"rsync"},
			})).To(Succeed())
		})
		It("updates fstab", func() {
			fstab := filepath.Join(root, ".snapshots/5/snapshot/etc/fstab")
			Expect(vfs.MkdirAll(tfs, filepath.Dir(fstab), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(fstab, []byte("UUID=dafsd  /etc  btrfs defaults... 0 0"), vfs.FilePerm)).To(Succeed())
			Expect(upgradeH.UpdateFstab(trans)).To(Succeed())
			data, err := tfs.ReadFile(fstab)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("subvol=@/.snapshots/5/snapshot/etc"))
		})
	})
})
