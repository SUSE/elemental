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
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
	transmock "github.com/suse/elemental/v3/pkg/transaction/mock"
)

func TestInstallSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install test suite")
}

const sgdiskEmpty = `Disk /dev/sda: 500118192 sectors, 238.5 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): CE4AA9A2-59DF-4DCC-B55A-A27A80676B33
Partition table holds up to 128 entries
First usable sector is 34, last usable sector is 500118158
Partitions will be aligned on 2048-sector boundaries
`

const firstPart = `
Number  Start (sector)    End (sector)  Size       Code  Name
   1            2048          2099199  1024 MiB    EF00  
`

const secondPart = `2099200        500118158  237.5 GiB   8300  `

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
		  "path": "/dev/device1",
		  "pkname": "/dev/device",
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
		  "path": "/dev/device2",
		  "pkname": "/dev/device",
		  "type": "part"
	   }
	]
 }`

var _ = Describe("Install", Label("install"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var d *deployment.Deployment
	var i *install.Installer
	var t *transmock.Transactioner
	var trans *transaction.Transaction
	var table string
	BeforeEach(func() {
		var err error
		t = &transmock.Transactioner{}
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(fs, "/dev", vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(fs, "/etc", vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/dev/device", []byte{}, vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/dev/device1", []byte{}, vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/dev/device2", []byte{}, vfs.DirPerm)).To(Succeed())
		d = deployment.DefaultDeployment()
		d.Disks[0].Device = "/dev/device"
		d.Disks[0].Partitions[0].UUID = "34A8-ABB8"
		d.Disks[0].Partitions[1].UUID = "34a8abb8-ddb3-48a2-8ecc-2443e92c7510"
		d.SourceOS = deployment.NewDirSrc("/some/dir")
		Expect(d.Sanitize(s)).To(Succeed())
		i = install.New(context.Background(), s, install.WithTransaction(t))
		table = sgdiskEmpty

		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			switch cmd {
			case "sgdisk":
				if args[0] == "-p" {
					return []byte(table), nil
				}
				if strings.HasPrefix(args[0], "-n=1") {
					table += firstPart
				}
				if strings.HasPrefix(args[0], "-n=2") {
					table += secondPart
				}
				return runner.ReturnValue, runner.ReturnError
			case "lsblk":
				return []byte(lsblkJson), runner.ReturnError
			default:
				return runner.ReturnValue, runner.ReturnError
			}
		}
		trans = &transaction.Transaction{
			ID:   1,
			Path: "/snapshot/path",
		}
		t.Trans = trans
		t.SrcDigest = "imagedigest"
		dir, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		Expect(vfs.MkdirAll(fs, dir, vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile(filepath.Join(dir, "config.sh"), []byte{}, vfs.FilePerm)).To(Succeed())
		Expect(fs.WriteFile("/etc/config.sh", []byte{}, vfs.FilePerm)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
	})
	It("installs the given deployment", func() {
		Expect(i.Install(d)).To(Succeed())
		Expect(d.SourceOS.GetDigest()).To(Equal("imagedigest"))
		Expect(runner.MatchMilestones([][]string{{
			"/etc/config.sh",
		}}))
	})
	It("fails on transaction initalization", func() {
		t.InitErr = fmt.Errorf("init failed")
		err := i.Install(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("init failed"))
		Expect(t.RollbackCalled()).To(BeFalse())
	})
	It("fails on transaction start", func() {
		t.StartErr = fmt.Errorf("start failed")
		err := i.Install(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("start failed"))
		Expect(t.RollbackCalled()).To(BeFalse())
	})
	It("fails on transaction commit", func() {
		t.CommitErr = fmt.Errorf("commit failed")
		err := i.Install(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("commit failed"))
		Expect(t.RollbackCalled()).To(BeTrue())
	})
})
