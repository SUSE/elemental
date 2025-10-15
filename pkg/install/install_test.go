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

package install_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestInstallSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install test suite")
}

const systemdRepartJson = `[
	{"uuid" : "c60d1845-7b04-4fc4-8639-8c49eb7277d5", "partno" : 0},
	{"uuid" : "34a8abb8-ddb3-48a2-8ecc-2443e92c7510", "partno" : 1}
]`

const sectorSizeJson = `{
   "blockdevices": [
      {
         "name": "device",
         "phy-sec": 512
      }
   ]
}`

const lsblkJson = `{
	"blockdevices": [
	   {
		  "label": "EFI",
		  "partlabel": "efi",
		  "partuuid": "c60d1845-7b04-4fc4-8639-8c49eb7277d5",
		  "size": 272629760,
		  "fstype": "vfat",
		  "mountpoints": [
			  "/boot"
		  ],
		  "path": "/dev/device1",
		  "pkname": "/dev/device",
		  "type": "part"
	   },{
		  "label": "SYSTEM",
		  "partlabel": "system",
		  "partuuid": "34a8abb8-ddb3-48a2-8ecc-2443e92c7510",
		  "size": 2726297600,
		  "fstype": "btrfs",
		  "mountpoints": [
			  "/some/root"
		  ],
		  "path": "/dev/device2",
		  "pkname": "/dev/device",
		  "type": "part"
	   }
	]
 }`

type upgraderMock struct {
	Error error
}

func (u upgraderMock) Upgrade(_ *deployment.Deployment) error {
	return u.Error
}

var _ = Describe("Install", Label("install"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var d *deployment.Deployment
	var i *install.Installer
	var upgrader *upgraderMock
	var sideEffects map[string]func(...string) ([]byte, error)
	BeforeEach(func() {
		var err error
		upgrader = &upgraderMock{}
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		sideEffects = map[string]func(...string) ([]byte, error){}

		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/dev/device":  []byte{},
			"/dev/device1": []byte{},
			"/dev/device2": []byte{},
		})
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		d = deployment.DefaultDeployment()
		d.Disks[0].Device = "/dev/device"
		d.Disks[0].Partitions[0].UUID = "c60d1845-7b04-4fc4-8639-8c49eb7277d5"
		d.Disks[0].Partitions[1].UUID = "34a8abb8-ddb3-48a2-8ecc-2443e92c7510"
		d.SourceOS = deployment.NewDirSrc("/some/dir")
		Expect(d.Sanitize(s)).To(Succeed())
		i = install.New(context.Background(), s, install.WithUpgrader(upgrader))

		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if f := sideEffects[cmd]; f != nil {
				return f(args...)
			}
			return runner.ReturnValue, runner.ReturnError
		}
		sideEffects["systemd-repart"] = func(args ...string) ([]byte, error) {
			return []byte(systemdRepartJson), runner.ReturnError
		}
		sideEffects["lsblk"] = func(args ...string) ([]byte, error) {
			if slices.Contains(args, "NAME,PHY-SEC") {
				return []byte(sectorSizeJson), runner.ReturnError
			}
			return []byte(lsblkJson), runner.ReturnError
		}
	})
	AfterEach(func() {
		cleanup()
	})
	It("installs the given deployment", func() {
		Expect(i.Install(d)).To(Succeed())
		Expect(runner.MatchMilestones([][]string{
			{"systemd-repart"},
			{"btrfs", "subvolume", "create"},
		}))
	})
	It("fails if upgrader errors out", func() {
		upgrader.Error = fmt.Errorf("transaction failed")
		err := i.Install(d)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("executing transaction: transaction failed"))
		Expect(runner.MatchMilestones([][]string{
			{"systemd-repart"},
			{"btrfs", "subvolume", "create"},
		}))
	})
})
