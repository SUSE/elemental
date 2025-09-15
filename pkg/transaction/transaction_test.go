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
	"context"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
)

func TestTransactionSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Transaction test suite")
}

const lsblkJson = `{
	"blockdevices": [
	   {
		  "label": "EFI",
		  "partlabel": "efi",
		  "uuid": "34A8-ABB8",
		  "size": 272629760,
		  "fstype": "vfat",
		  "mountpoints": [
			  "/boot"
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
	   },{
		  "label": "HIDDEN",
		  "partlabel": "",
		  "uuid": "d7dd841f-aeaa-4fe3-a383-8913f4e8d4de",
		  "size": 2726297600,
		  "fstype": "btrfs",
		  "mountpoints": [
			  "/run/hidden"
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

// Global variables for transaction tests
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

// var trans *transaction.Transaction
// var upgradeH transaction.UpgradeHelper
var syscall sys.Syscall

func snapperContextMock() {
	syscall = &sysmock.Syscall{}
	mount = sysmock.NewMounter()
	sideEffects = map[string]func(...string) ([]byte, error){}
	runner = sysmock.NewRunner()
	tfs, cleanup, err = sysmock.TestFS(nil)
	Expect(err).NotTo(HaveOccurred())
	logger := log.New(log.WithDiscardAll())
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
	}, &deployment.Partition{
		Label:      "HIDDEN",
		FileSystem: deployment.Btrfs,
		Role:       deployment.Data,
		UUID:       "d7dd841f-aeaa-4fe3-a383-8913f4e8d4de",
		MountPoint: "/run/hidden",
		Hidden:     true,
	})
	runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
		if f := sideEffects[cmd]; f != nil {
			return f(args...)
		}
		return runner.ReturnValue, runner.ReturnError
	}
	imgsrc = deployment.NewDirSrc("/image/mounted")
}

func initSnapperInstall(root string) transaction.UpgradeHelper {
	By("initiate snapper transactioner")
	mount.Mount("/dev/sda2", root, "", []string{"subvol=@"})
	sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
		return []byte(lsblkJson), nil
	}
	sn = transaction.NewSnapperTransaction(ctx, s)
	upgradeH, err := sn.Init(*d)
	Expect(err).NotTo(HaveOccurred())
	Expect(runner.CmdsMatch([][]string{
		{"lsblk", "-p", "-b", "-n", "-J", "--output"},
		{"/usr/lib/snapper/installation-helper", "--root-prefix", "/some/root"},
	})).To(Succeed())
	return upgradeH
}

func initSnapperUpgrade(root string) transaction.UpgradeHelper {
	By("initiate snapper transactioner")
	sideEffects["snapper"] = func(args ...string) ([]byte, error) {
		if slices.Contains(args, "list") && slices.Contains(args, "root") {
			return []byte(upgradeSnapList), nil
		}
		return runner.ReturnValue, runner.ReturnError
	}
	mount.Mount("/dev/sda2", root, "", []string{"ro", "subvol=@/.snapshots/4/snapshot"})
	sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
		return []byte(lsblkJson), nil
	}
	sn = transaction.NewSnapperTransaction(ctx, s)
	upgradeH, err := sn.Init(*d)
	Expect(err).NotTo(HaveOccurred())
	Expect(runner.CmdsMatch([][]string{
		{"lsblk", "-p", "-b", "-n", "-J", "--output"},
		{"snapper", "--no-dbus", "-c", "root", "--jsonout", "list"},
	})).To(Succeed())
	return upgradeH
}

func startInstallTransaction() *transaction.Transaction {
	By("starting a transaction")

	sideEffects["snapper"] = func(args ...string) ([]byte, error) {
		if slices.Contains(args, "snapper") {
			if slices.Contains(args, "--print-number") {
				return []byte("1\n"), nil
			}
		}
		return runner.ReturnValue, runner.ReturnError
	}

	trans, err := sn.Start()

	Expect(err).NotTo(HaveOccurred())
	Expect(trans.ID).To(Equal(1))
	Expect(len(trans.Merges)).To(Equal(0))
	Expect(runner.MatchMilestones([][]string{
		{"btrfs", "subvolume", "create"},
		{"btrfs", "subvolume", "create"},
	})).To(Succeed())
	runner.ClearCmds()

	return trans
}

func startUpgradeTransaction() *transaction.Transaction {
	By("starting a transaction")

	sideEffects["snapper"] = func(args ...string) ([]byte, error) {
		if slices.Contains(args, "create") {
			return []byte("5\n"), nil
		}
		if slices.Contains(args, "etc") && slices.Contains(args, "list") {
			return []byte(etcSnaps), nil
		}
		if slices.Contains(args, "home") && slices.Contains(args, "list") {
			return []byte(homeSnaps), nil
		}
		return runner.ReturnValue, runner.ReturnError
	}

	trans, err := sn.Start()
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
	return trans
}
