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
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
)

const lsblkJson = `{
	"blockdevices": [
	   {
		  "label": "EFI",
		  "partlabel": "efi",
		  "uuid": "34A8-ABB8",
		  "size": 272629760,
		  "fstype": "vfat",
		  "mountpoints": [
			  "/boot/efi"
		  ],
		  "path": "/dev/sda1",
		  "pkname": "/dev/sda",
		  "type": "part"
	   },{
		  "label": "SYSTEM",
		  "partlabel": "system",
		  "uuid": "34a8abb8-ddb3-48a2-8ecc-2443e92c7510",
		  "size": 2726297600,
		  "fstype": "btrfs",
		  "mountpoints": [
			  "/some/root"
		  ],
		  "path": "/dev/sda2",
		  "pkname": "/dev/sda",
		  "type": "part"
	   },{
		  "label": "DATA",
		  "partlabel": "",
		  "uuid": "2443e92c-ddb3-48a2-8ecc-34a8abb87510",
		  "size": 2726297600,
		  "fstype": "btrfs",
		  "mountpoints": [
			  "/some/root"
		  ],
		  "path": "/dev/sda2",
		  "pkname": "/dev/sda",
		  "type": "part"
	   }
	]
 }`

const etcSnaps = `{
	"etc": [
	  {
		"number": 1,
		"default": false,
		"active": false,
		"userdata": {
		    "stock": "true"
		}
	  },{
		"number": 2,
		"default": false,
		"active": false,
		"userdata": {
		    "post-transaction": "true"
		}
	  }
	]
  }
`

const homeSnaps = `{
	"home": [
	  {
		"number": 1,
		"default": false,
		"active": false,
		"userdata": {
		    "stock": "true"
		}
	  },{
		"number": 2,
		"default": false,
		"active": false,
		"userdata": {
		    "post-transaction": "true"
		}
	  }
	]
  }
`

const upgradeSnapList = `{
	"root": [
	  {
		"number": 1,
		"default": false,
		"active": false,
		"userdata": null
	  },{
		"number": 2,
		"default": false,
		"active": false,
		"userdata": null
	  },{
		"number": 3,
		"default": false,
		"active": false,
		"userdata": null
	  },{
		"number": 4,
		"default": true,
		"active": true,
		"userdata": null
	  }
	]
  }
`

const installSnapList = `{
	"root": [
	  {
		"number": 0,
		"default": false,
		"active": false,
		"userdata": null
	  },
	  {
		"number": 1,
		"default": true,
		"active": false,
		"userdata": null
	  }
	]
  }
`

var _ = Describe("SnapperTransaction", Label("transaction"), func() {
	var tfs vfs.FS
	var s *sys.System
	var cleanup func()
	var err error
	var runner *sysmock.Runner
	var mount *sysmock.Mounter
	var ctx context.Context
	var cancel func()
	var sn transaction.Interface
	var d *deployment.Deployment
	var sideEffects map[string]func(...string) ([]byte, error)
	var imgsrc *deployment.ImageSource
	var trans *transaction.Transaction
	var root string
	var buffer *bytes.Buffer
	var syscall sys.Syscall
	BeforeEach(func() {
		syscall = &sysmock.Syscall{}
		mount = sysmock.NewMounter()
		buffer = &bytes.Buffer{}
		sideEffects = map[string]func(...string) ([]byte, error){}
		runner = sysmock.NewRunner()
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		logger := log.New(log.WithBuffer(buffer))
		logger.SetLevel(log.DebugLevel())
		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithLogger(logger), sys.WithSyscall(syscall),
			sys.WithRunner(runner), sys.WithMounter(mount),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, "/etc", vfs.DirPerm)).To(Succeed())
		ctx, cancel = context.WithCancel(context.Background())
		d = deployment.DefaultDeployment()
		d.Disks[0].Partitions[0].UUID = "34A8-ABB8"
		d.Disks[0].Partitions[1].UUID = "34a8abb8-ddb3-48a2-8ecc-2443e92c7510"
		d.Disks[0].Partitions[1].Size = 4096
		d.Disks[0].Partitions = append(d.Disks[0].Partitions, &deployment.Partition{
			Label:      "DATA",
			FileSystem: deployment.Btrfs,
			Role:       deployment.Data,
			UUID:       "2443e92c-ddb3-48a2-8ecc-34a8abb87510",
			RWVolumes: []deployment.RWVolume{{
				Path:        "/home",
				Snapshotted: true,
			}},
		})
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if f := sideEffects[cmd]; f != nil {
				return f(args...)
			}
			return runner.ReturnValue, runner.ReturnError
		}
		imgsrc = deployment.NewDirSrc("/image/mounted")

		root = "/some/root"
	})
	AfterEach(func() {
		cleanup()
	})
	Describe("running install transaction", func() {
		var snTemplate, snapshotP string
		BeforeEach(func() {
			By("initiate snapper transactioner")
			mount.Mount("/dev/sda2", "/some/root", "", []string{"subvol=@"})
			sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
				return []byte(lsblkJson), nil
			}
			sn = transaction.NewSnapperTransaction(ctx, s)
			err = sn.Init(*d)
			Expect(err).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"lsblk", "-p", "-b", "-n", "-J", "--output"},
				{"/usr/lib/snapper/installation-helper", "--root-prefix", "/some/root"},
			})).To(Succeed())
		})
		Describe("transaction started", func() {
			var rsyncSideEffect func(args ...string) ([]byte, error)
			BeforeEach(func() {
				By("starting a transaction")

				sideEffects["env"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "snapper") {
						if slices.Contains(args, "--print-number") {
							return []byte("1\n"), nil
						}
					}
					return runner.ReturnValue, runner.ReturnError
				}

				trans, err = sn.Start()

				Expect(err).NotTo(HaveOccurred())
				Expect(trans.ID).To(Equal(1))
				Expect(len(trans.Merges)).To(Equal(0))
				Expect(runner.MatchMilestones([][]string{
					{"btrfs", "subvolume", "create"},
					{"btrfs", "subvolume", "create"},
				})).To(Succeed())

				rsyncSideEffect = func(args ...string) ([]byte, error) {
					// create expected files in subvolume
					snapshotP = ".snapshots/1/snapshot"
					snTemplate = "/usr/share/snapper/config-templates/default"
					snSysConf := filepath.Join(root, btrfs.TopSubVol, snapshotP, "/etc/sysconfig/snapper")
					template := filepath.Join(root, btrfs.TopSubVol, snapshotP, snTemplate)
					configsDir := filepath.Join(root, btrfs.TopSubVol, snapshotP, "/etc/snapper/configs")

					Expect(vfs.MkdirAll(tfs, configsDir, vfs.DirPerm)).To(Succeed())
					Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
					Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
					Expect(tfs.WriteFile(template, []byte{}, vfs.FilePerm)).To(Succeed())
					Expect(vfs.MkdirAll(tfs, filepath.Dir(snSysConf), vfs.DirPerm)).To(Succeed())
					Expect(tfs.WriteFile(snSysConf, []byte{}, vfs.FilePerm)).To(Succeed())
					return runner.ReturnValue, runner.ReturnError
				}
				runner.ClearCmds()
			})
			Describe("transaction updated", func() {
				BeforeEach(func() {
					By("updating transaction with the image source")
					sideEffects["rsync"] = rsyncSideEffect
					hookCalled := false
					Expect(sn.Update(trans, imgsrc, func() error {
						hookCalled = true
						return nil
					})).To(Succeed())
					Expect(hookCalled).To(BeTrue())
					Expect(runner.MatchMilestones([][]string{
						{"rsync", "--info=progress2", "--human-readable"},
						{"snapper", "--no-dbus", "-c", "etc", "create-config", "--fstype", "btrfs", "/etc"},
						{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "etc", "create", "--print-number"},
						{"snapper", "--no-dbus", "-c", "home", "create-config", "--fstype", "btrfs", "/home"},
						{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "home", "create", "--print-number"},
						{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--read-only", "1"},
					})).To(Succeed())
					runner.ClearCmds()
				})
				It("commits the current transaction", func() {
					sideEffects["env"] = func(args ...string) ([]byte, error) {
						if slices.Contains(args, "snapper") {
							return []byte("2\n"), nil
						}
						return runner.ReturnValue, runner.ReturnError
					}
					sideEffects["snapper"] = func(args ...string) ([]byte, error) {
						if slices.Contains(args, "list") {
							return []byte(installSnapList), nil
						}
						return runner.ReturnValue, runner.ReturnError
					}
					Expect(sn.Commit(trans)).To(Succeed())
					Expect(runner.MatchMilestones([][]string{
						{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--default"},
					})).To(Succeed())
				})
				It("commits a transaction with error if context is cancelled", func() {
					sideEffects["env"] = func(args ...string) ([]byte, error) {
						if slices.Contains(args, "snapper") {
							return []byte("2\n"), nil
						}
						return runner.ReturnValue, runner.ReturnError
					}
					sideEffects["snapper"] = func(args ...string) ([]byte, error) {
						if slices.Contains(args, "list") {
							return []byte(installSnapList), nil
						}
						return runner.ReturnValue, runner.ReturnError
					}
					cancel()
					err := sn.Commit(trans)
					Expect(err).To(HaveOccurred())
					Expect(runner.MatchMilestones([][]string{
						{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--default"},
					})).To(Succeed())
					Expect(err.Error()).To(ContainSubstring("context canceled"))
				})
				It("fails to set default snapshot", func() {
					sideEffects["snapper"] = func(args ...string) ([]byte, error) {
						if slices.Contains(args, "--default") {
							return []byte("error setting default\n"), fmt.Errorf("failed setting default")
						}
						return runner.ReturnValue, runner.ReturnError
					}
					err = sn.Commit(trans)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed setting default"))
				})
			})
			It("returns error if source sync fails", func() {
				sideEffects["rsync"] = func(args ...string) ([]byte, error) {
					return []byte{}, fmt.Errorf("failed to sync source")
				}
				err := sn.Update(trans, imgsrc, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to sync source"))
				Expect(runner.MatchMilestones([][]string{
					{"rsync", "--info=progress2", "--human-readable"},
				})).To(Succeed())

			})
			It("returns error if snapper configuration fails", func() {
				err := sn.Update(trans, imgsrc, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to find file"))
				Expect(runner.MatchMilestones([][]string{
					{"rsync", "--info=progress2", "--human-readable"},
				})).To(Succeed())

			})
			It("returns error if hook fails", func() {
				sideEffects["rsync"] = rsyncSideEffect
				err := sn.Update(trans, imgsrc, func() error { return fmt.Errorf("hook error") })
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("hook error"))
			})
			It("returns error if context is cancelled fails", func() {
				sideEffects["rsync"] = rsyncSideEffect
				cancel()
				err := sn.Update(trans, imgsrc, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(""))
			})
			It("fails to commit a non updated transaction", func() {
				err := sn.Commit(trans)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not updated"))
			})
		})
		It("returns error if context is cancelled", func() {
			sideEffects["env"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "snapper") {
					return []byte("1\n"), nil
				}
				return runner.ReturnValue, runner.ReturnError
			}
			cancel()
			_, err = sn.Start()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
			Expect(runner.MatchMilestones([][]string{
				{"btrfs", "subvolume", "create"},
			})).To(Succeed())
			Expect(mount.IsMountPoint("/some/root/@/.snapshots/1/snapshot/var")).To(BeTrue())
		})
		It("fails to create etc subvolume", func() {
			sideEffects["btrfs"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "/some/root/@/.snapshots/1/snapshot/etc") {
					return []byte{}, fmt.Errorf("failed creating etc")
				}
				return runner.ReturnValue, runner.ReturnError
			}
			_, err = sn.Start()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed creating etc"))
			Expect(runner.MatchMilestones([][]string{
				{"btrfs", "subvolume", "create"},
				{"btrfs", "property", "set", "-ts", "/some/root/@/.snapshots/1/snapshot", "ro", "false"},
				{"btrfs", "subvolume", "delete", "-c", "-R"},
			})).To(Succeed())
		})
	})
	Describe("running upgrade transaction", func() {
		var snTemplate, snapshotP string
		BeforeEach(func() {
			By("initiate snapper transactioner")
			sideEffects["snapper"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "list") && slices.Contains(args, "root") {
					return []byte(upgradeSnapList), nil
				}
				return runner.ReturnValue, runner.ReturnError
			}
			mount.Mount("/dev/sda2", "/", "", []string{"ro", "subvol=@/.snapshots/4/snapshot"})
			sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
				return []byte(lsblkJson), nil
			}
			sn = transaction.NewSnapperTransaction(ctx, s)
			Expect(sn.Init(*d)).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"lsblk", "-p", "-b", "-n", "-J", "--output"},
				{"snapper", "--no-dbus", "-c", "root", "--jsonout", "list"},
			})).To(Succeed())
			root = "/"
		})
		Describe("starts an upgrade transaction", func() {
			BeforeEach(func() {
				By("starting a transaction")

				sideEffects["env"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "snapper") {
						return []byte("5\n"), nil
					}
					return runner.ReturnValue, runner.ReturnError
				}
				sideEffects["snapper"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "etc") && slices.Contains(args, "list") {
						return []byte(etcSnaps), nil
					}
					if slices.Contains(args, "home") && slices.Contains(args, "list") {
						return []byte(homeSnaps), nil
					}
					return runner.ReturnValue, runner.ReturnError
				}

				trans, err = sn.Start()
				Expect(err).NotTo(HaveOccurred())
				Expect(trans.ID).To(Equal(5))
				Expect(len(trans.Merges)).To(Equal(2))
				Expect(runner.MatchMilestones([][]string{
					{"snapper", "--no-dbus", "--root", "/.snapshots/4/snapshot", "-c", "etc", "--jsonout", "list"},
					{"btrfs", "subvolume", "snapshot"},
					{"snapper", "--no-dbus", "--root", "/tmp/elemental_data/.snapshots/4/snapshot", "-c", "home", "--jsonout", "list"},
					{"btrfs", "subvolume", "snapshot"},
				})).To(Succeed())
				runner.ClearCmds()
			})
			Describe("transaction updated", func() {
				BeforeEach(func() {
					By("updates a transaction with the image contents")
					sideEffects["rsync"] = func(args ...string) ([]byte, error) {
						// create expected files in subvolume
						snapshotP = ".snapshots/5/snapshot"
						snTemplate = "/usr/share/snapper/config-templates/default"
						snSysConf := filepath.Join(root, snapshotP, "/etc/sysconfig/snapper")
						template := filepath.Join(root, snapshotP, snTemplate)
						configsDir := filepath.Join(root, snapshotP, "/etc/snapper/configs")

						Expect(vfs.MkdirAll(tfs, configsDir, vfs.DirPerm)).To(Succeed())
						Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
						Expect(vfs.MkdirAll(tfs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
						Expect(tfs.WriteFile(template, []byte{}, vfs.FilePerm)).To(Succeed())
						Expect(vfs.MkdirAll(tfs, filepath.Dir(snSysConf), vfs.DirPerm)).To(Succeed())
						Expect(tfs.WriteFile(snSysConf, []byte{}, vfs.FilePerm)).To(Succeed())
						Expect(tfs.WriteFile(filepath.Join(root, snapshotP, "/etc/fstab"), []byte{}, vfs.FilePerm)).To(Succeed())
						return runner.ReturnValue, runner.ReturnError
					}
					hookCalled := false
					Expect(sn.Update(trans, imgsrc, func() error {
						hookCalled = true
						return nil
					})).To(Succeed())
					Expect(hookCalled).To(BeTrue())
					Expect(runner.MatchMilestones([][]string{
						{"rsync", "--info=progress2", "--human-readable"},
						{"snapper", "--no-dbus", "-c", "etc", "create-config", "--fstype", "btrfs", "/etc"},
						{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "etc", "create", "--print-number"},
						{"snapper", "--no-dbus", "-c", "home", "create-config", "--fstype", "btrfs", "/home"},
						{"env", "LC_ALL=C", "snapper", "--no-dbus", "-c", "home", "create", "--print-number"},
						{"snapper", "--no-dbus", "--root", "/.snapshots/5/snapshot", "modify", "--read-only", "5"},
					})).To(Succeed())
					runner.ClearCmds()
				})
				It("commit a transaction", func() {
					sideEffects["env"] = func(args ...string) ([]byte, error) {
						if slices.Contains(args, "snapper") {
							if slices.Contains(args, "etc") || slices.Contains(args, "home") {
								return []byte("2\n"), nil
							}
						}
						return runner.ReturnValue, runner.ReturnError
					}
					Expect(sn.Commit(trans)).To(Succeed())
					Expect(runner.MatchMilestones([][]string{
						{"snapper", "--no-dbus", "--root", "/.snapshots/5/snapshot", "modify", "--default"},
					})).To(Succeed())
				})
			})
		})
		It("it fails to start a transaction if it does not find previous snapshotted volumes", func() {
			sideEffects["env"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "snapper") {
					return []byte("5\n"), nil
				}
				return runner.ReturnValue, runner.ReturnError
			}
			trans, err = sn.Start()
			Expect(err).To(HaveOccurred())
			Expect(runner.MatchMilestones([][]string{
				{"snapper", "--no-dbus", "--root", "/.snapshots/4/snapshot", "-c", "etc", "--jsonout", "list"},
			})).To(Succeed())
		})
	})
	It("fails to init snapper transactioner if it can't list snapshots", func() {
		mount.Mount("/dev/sda2", "/", "", []string{"ro", "subvol=@/.snapshots/4/snapshot"})
		sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
			return []byte(lsblkJson), nil
		}
		sn = transaction.NewSnapperTransaction(ctx, s)
		Expect(sn.Init(*d)).NotTo(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"lsblk", "-p", "-b", "-n", "-J", "--output"},
			{"snapper", "--no-dbus", "-c", "root", "--jsonout", "list"},
		})).To(Succeed())
	})
	It("fails to init snapper transactioner if it does not detect partitions", func() {
		sn = transaction.NewSnapperTransaction(ctx, s)
		Expect(sn.Init(*d)).NotTo(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"lsblk", "-p", "-b", "-n", "-J", "--output",
		}})).To(Succeed())
	})
	It("returns error if context is cancelled", func() {
		mount.Mount("/dev/sda2", "/some/root", "", []string{"subvol=@"})
		sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
			return []byte(lsblkJson), nil
		}
		sn = transaction.NewSnapperTransaction(ctx, s)
		cancel()
		Expect(sn.Init(*d)).NotTo(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{"lsblk", "-p", "-b", "-n", "-J", "--output"},
			{"/usr/lib/snapper/installation-helper", "--root-prefix", "/some/root"},
		})).To(Succeed())
	})
})
