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

package diskrepart_test

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestDiskRepartSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiskRepart test suite")
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

var _ = Describe("DiskRepart", Label("diskrepart"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).ToNot(HaveOccurred())
		err = vfs.MkdirAll(fs, "/dev", vfs.DirPerm)
		Expect(err).To(BeNil())
		_, err = fs.Create("/dev/device")
		Expect(err).To(BeNil())
		_, err = fs.Create("/dev/device1")
		Expect(err).To(BeNil())
		_, err = fs.Create("/dev/device2")
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		cleanup()
	})
	It("formats and partitions the given disk", func() {
		table := sgdiskEmpty
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
			default:
				return runner.ReturnValue, runner.ReturnError
			}
		}
		d := deployment.DefaultDeployment()
		d.Disks[0].Device = "/dev/device"
		err := diskrepart.PartitionAndFormatDevice(s, d.Disks[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(runner.MatchMilestones([][]string{
			{"sgdisk", "--zap-all", "/dev/device"},
			{"partx", "-u", "/dev/device"},
			{"sgdisk", "-p", "-v", "/dev/device"},
			{"sgdisk", "-n=1:2048:+2097152", "-c=1:efi", "-t=1:EF00", "/dev/device"},
			{"mkfs.vfat", "-n", "EFI", "-i"},
			{"sgdisk", "-n=2:2099200:+0", "-c=2:system", "-t=2:8300", "/dev/device"},
			{"mkfs.btrfs", "-L", "SYSTEM", "-U"},
		})).To(Succeed())
	})
	It("Clears filesystem header from a partition", func() {
		cmds := [][]string{
			{"wipefs", "--all", "/dev/device1"},
		}
		Expect(diskrepart.WipeFSOnPartition(s, "/dev/device1")).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
})
