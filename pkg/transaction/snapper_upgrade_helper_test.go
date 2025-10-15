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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
)

const snapperStatus = `{
-..... /etc/deletedFile
+..... /etc/createdFile
c..... /etc/modifiedFile
....x. /etc/relabelledFile
`

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
			err := upgradeH.SyncImageContent(imgsrc, trans)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unpacking image to "))
			Expect(err.Error()).To(ContainSubstring("rsync error"))
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

			sideEffects["snapper"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "create") {
					return []byte("2\n"), nil
				}
				return []byte{}, nil
			}

			Expect(upgradeH.Merge(trans)).To(Succeed())
			// Snapper configuration is done before merging
			Expect(runner.MatchMilestones([][]string{
				{"snapper", "--no-dbus", "-c", "etc", "create-config", "--fstype", "btrfs", "/etc"},
				{"snapper", "--no-dbus", "-c", "etc", "create", "--print-number"},
				{"snapper", "--no-dbus", "-c", "home", "create-config", "--fstype", "btrfs", "/home"},
				{"snapper", "--no-dbus", "-c", "home", "create", "--print-number"},
			})).To(Succeed())
			// No merge is executed on first (install) transaction
			Expect(runner.MatchMilestones([][]string{
				{"rsync"},
			})).NotTo(Succeed())
		})
		It("fails to create snapper configuration if templates are not found", func() {
			err = upgradeH.Merge(trans)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configuring snapper: setting root configuration: " +
				"finding default snapper configuration template: failed to find file matching"))
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
			data, err := tfs.ReadFile(fstab)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Not(ContainSubstring("PARTUUID=d7dd841f-aeaa-4fe3-a383-8913f4e8d4de")))
		})
		It("it fails to create fstab file if the path does not exist", func() {
			err := upgradeH.UpdateFstab(trans)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("creating fstab: creating file: open"))
			Expect(err.Error()).To(ContainSubstring("no such file or directory"))
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
			err := upgradeH.Lock(trans)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("configuring new snapshot as read-only: snapper error"))
			Expect(runner.CmdsMatch([][]string{
				{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--read-only"},
			})).To(Succeed())
		})
	})
	Describe("upgrade helper for an upgrade transaction", func() {
		BeforeEach(func() {
			root = "/"
			upgradeH = initSnapperUpgrade(root)
			trans = startUpgradeTransaction()
		})
		It("configures snapper and merges RW volumes", func() {
			etcStatus := "/tmp/snapStatus/snap_status_etc"
			homeStatus := "/tmp/snapStatus/snap_status_home"
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
			Expect(vfs.MkdirAll(tfs, filepath.Dir(etcStatus), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(etcStatus, []byte(snapperStatus), vfs.FilePerm)).To(Succeed())
			Expect(tfs.WriteFile(homeStatus, []byte{}, vfs.FilePerm)).To(Succeed())

			Expect(upgradeH.Merge(trans)).To(Succeed())
			Expect(runner.MatchMilestones([][]string{
				{"snapper", "--no-dbus", "-c", "etc", "create-config", "--fstype", "btrfs", "/etc"},
				{"snapper", "--no-dbus", "-c", "etc", "create", "--print-number"},
				{"snapper", "--no-dbus", "-c", "home", "create-config", "--fstype", "btrfs", "/home"},
				{"snapper", "--no-dbus", "-c", "home", "create", "--print-number"},
				{
					"snapper", "--no-dbus", "--root", "/.snapshots/4/snapshot", "-c", "etc",
					"status", "--output", "/tmp/snapStatus/snap_status_etc", "1..5",
				},
				{"rsync"},
				{
					"snapper", "--no-dbus", "--root", "/tmp/elemental_data/.snapshots/4/snapshot",
					"-c", "home", "status", "--output", "/tmp/snapStatus/snap_status_home", "1..5",
				},
				{"rsync"},
			})).To(Succeed())
		})
		It("updates fstab", func() {
			fstab := filepath.Join(root, ".snapshots/5/snapshot/etc/fstab")
			Expect(vfs.MkdirAll(tfs, filepath.Dir(fstab), vfs.DirPerm)).To(Succeed())
			Expect(tfs.WriteFile(fstab, []byte("PARTUUID=dafsd  /etc  btrfs defaults... 0 0"), vfs.FilePerm)).To(Succeed())
			Expect(upgradeH.UpdateFstab(trans)).To(Succeed())
			data, err := tfs.ReadFile(fstab)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("subvol=@/.snapshots/5/snapshot/etc"))
			Expect(string(data)).To(Not(ContainSubstring("PARTUUID=d7dd841f-aeaa-4fe3-a383-8913f4e8d4de")))
		})
	})
})
