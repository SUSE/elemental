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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/transaction"
)

var _ = Describe("SnapperTransaction", Label("transaction"), func() {
	var root string
	BeforeEach(func() {
		snapperContextMock()
	})
	AfterEach(func() {
		cleanup()
	})
	Describe("running install transaction", func() {
		BeforeEach(func() {
			root = "/some/root"
			_ = initSnapperInstall(root)
		})
		Describe("transaction started", func() {
			var trans *transaction.Transaction
			BeforeEach(func() {
				trans = startInstallTransaction()
			})
			It("commits the current transaction", func() {
				sideEffects["snapper"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "create") {
						return []byte("2\n"), nil
					}
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
				sideEffects["snapper"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "create") {
						return []byte("2\n"), nil
					}
					if slices.Contains(args, "list") {
						return []byte(installSnapList), nil
					}
					return runner.ReturnValue, runner.ReturnError
				}
				cancel()
				err := sn.Commit(trans)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("context canceled"))
				Expect(runner.MatchMilestones([][]string{
					{"snapper", "--no-dbus", "--root", "/some/root/@/.snapshots/1/snapshot", "modify", "--default"},
				})).To(Succeed())
			})
			It("fails to set default snapshot", func() {
				sideEffects["snapper"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "create") {
						return []byte("2\n"), nil
					}
					if slices.Contains(args, "--default") {
						return []byte("error setting default\n"), fmt.Errorf("failed setting default")
					}
					return runner.ReturnValue, runner.ReturnError
				}
				err = sn.Commit(trans)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("setting new default snapshot: failed setting default"))
			})
			It("fails to commit a non started transaction", func() {
				trans = &transaction.Transaction{ID: 4}
				err := sn.Commit(trans)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("transaction '4' is not started"))
			})
		})
		It("returns error if context is cancelled", func() {
			sideEffects["snapper"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "create") {
					return []byte("1\n"), nil
				}
				return runner.ReturnValue, runner.ReturnError
			}
			cancel()
			_, err = sn.Start()

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("context canceled"))
			Expect(runner.MatchMilestones([][]string{
				{"btrfs", "subvolume", "create"},
			})).To(Succeed())
			Expect(mount.IsMountPoint("/some/root/@/.snapshots/1/snapshot/var")).To(BeTrue())
		})
		It("fails to create etc subvolume", func() {
			sideEffects["btrfs"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "/some/root/@/.snapshots/1/snapshot/etc") {
					return []byte{}, fmt.Errorf("failed creating /etc")
				}
				return runner.ReturnValue, runner.ReturnError
			}
			_, err = sn.Start()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("preparing partitions: creating snapshotted subvolume '/etc': creating subvolume"))
			Expect(err.Error()).To(ContainSubstring("failed creating /etc"))
			Expect(runner.MatchMilestones([][]string{
				{"btrfs", "subvolume", "create"},
				{"btrfs", "property", "set", "-ts", "/some/root/@/.snapshots/1/snapshot", "ro", "false"},
				{"btrfs", "subvolume", "delete", "-c", "-R"},
			})).To(Succeed())
		})
	})
	Describe("running upgrade transaction", func() {
		var trans *transaction.Transaction
		BeforeEach(func() {
			root = "/"
			_ = initSnapperUpgrade(root)
		})
		Describe("starts an upgrade transaction", func() {
			BeforeEach(func() {
				trans = startUpgradeTransaction()
			})
			It("commits a transaction", func() {
				sideEffects["snapper"] = func(args ...string) ([]byte, error) {
					if slices.Contains(args, "create") {
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
		It("it fails to start a transaction if it does not find previous snapshotted volumes", func() {
			sideEffects["snapper"] = func(args ...string) ([]byte, error) {
				if slices.Contains(args, "create") {
					return []byte("5\n"), nil
				}
				return runner.ReturnValue, runner.ReturnError
			}
			trans, err = sn.Start()
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("preparing partitions: creating snapshotted subvolume '/etc': " +
				"creating volume with merge: listing snapshots for rw volume '/etc': " +
				"unmarshalling snapshots: unexpected end of JSON input"))
			Expect(runner.MatchMilestones([][]string{
				{"snapper", "--no-dbus", "--root", "/.snapshots/4/snapshot", "-c", "etc", "--jsonout", "list"},
			})).To(Succeed())
		})
	})
	It("fails to init snapper transactioner if it can't list snapshots", func() {
		Expect(mount.Mount("/dev/sda2", "/", "", []string{"ro", "subvol=@/.snapshots/4/snapshot"})).To(Succeed())
		sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
			return []byte(lsblkJson), nil
		}
		sn = transaction.NewSnapperTransaction(ctx, s)
		_, err = sn.Init(*d)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("determining snapshots state: listing snapshots: " +
			"unmarshalling snapshots: unexpected end of JSON input"))
		Expect(runner.CmdsMatch([][]string{
			{"lsblk", "-p", "-b", "-n", "-J", "--output"},
			{"snapper", "--no-dbus", "-c", "root", "--jsonout", "list"},
		})).To(Succeed())
	})
	It("fails to init snapper transactioner if it does not detect partitions", func() {
		sn = transaction.NewSnapperTransaction(ctx, s)
		_, err = sn.Init(*d)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("determining snapshots state: probing host partitions: unexpected end of JSON input"))
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
		_, err = sn.Init(*d)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("context canceled"))
		Expect(runner.CmdsMatch([][]string{
			{"lsblk", "-p", "-b", "-n", "-J", "--output"},
			{"/usr/lib/snapper/installation-helper", "--root-prefix", "/some/root"},
		})).To(Succeed())
	})
})
