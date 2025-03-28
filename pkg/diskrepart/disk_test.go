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
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/block"
	blockmock "github.com/suse/elemental/v3/pkg/block/mock"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const uuid = "236dacf0-b37e-4bca-a21a-59e4aef3ea4c"

const sgdiskPrint = `Disk /dev/sda: 500118192 sectors, 238.5 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): CE4AA9A2-59DF-4DCC-B55A-A27A80676B33
Partition table holds up to 128 entries
First usable sector is 34, last usable sector is 500118158
Partitions will be aligned on 2048-sector boundaries
Total free space is 2014 sectors (1007.0 KiB)

Number  Start (sector)    End (sector)  Size       Code  Name
   1            2048          526335   256.0 MiB   EF00
   2          526336        17303551   8.0 GiB     8200  
   3        17303552       500118158   230.2 GiB   8300  `

const partedPrint = `BYT;
/dev/loop0:50593792s:loopback:512:512:msdos:Loopback device:;
1:2048s:98303s:96256s:ext4::type=83;
2:98304s:29394943s:29296640s:ext4::boot, type=83;
3:29394944s:45019135s:15624192s:ext4::type=83;
4:45019136s:50331647s:5312512s:ext4::type=83;`

func TestDiskRepartSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiskRepart test suite")
}

var _ = Describe("DiskRepart", Label("diskrepart"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var dev *diskrepart.Disk
	var cmds [][]string
	var printCmd []string
	var bd *blockmock.Device
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(sys.WithMounter(mounter), sys.WithRunner(runner), sys.WithFS(fs))
		Expect(err).ToNot(HaveOccurred())
		err = vfs.MkdirAll(fs, "/dev", vfs.DirPerm)
		Expect(err).To(BeNil())
		_, err = fs.Create("/dev/device")
		Expect(err).To(BeNil())

		bd = blockmock.NewBlockDevice()
		dev = diskrepart.NewDisk(
			s, "/dev/device", diskrepart.WithBlockDevice(bd), diskrepart.WithParted(),
		)
		printCmd = []string{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "print",
		}
		cmds = [][]string{printCmd}
	})
	AfterEach(func() {
		cleanup()
	})
	It("Creates a disk object", func() {
		dev = diskrepart.NewDisk(s, "/dev/device", diskrepart.WithParted())
	})
	Describe("Load data without changes", func() {
		BeforeEach(func() {
			runner.ReturnValue = []byte(partedPrint)
		})
		It("Loads disk layout data", func() {
			Expect(dev.Reload()).To(BeNil())
			Expect(dev.String()).To(Equal("/dev/device"))
			Expect(dev.GetSectorSize()).To(Equal(uint(512)))
			Expect(dev.GetLastSector()).To(Equal(uint(50593792)))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Computes available free space", func() {
			Expect(dev.GetFreeSpace()).To(Equal(uint(262145)))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Checks it has at least 128MB of free space", func() {
			Expect(dev.CheckDiskFreeSpaceMiB(128)).To(Equal(true))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Checks it has less than 130MB of free space", func() {
			Expect(dev.CheckDiskFreeSpaceMiB(130)).To(Equal(false))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Get partition label", func() {
			dev.Reload()
			Expect(dev.GetLabel()).To(Equal("msdos"))
		})
		It("It fixes GPT headers if the disk was expanded", func() {
			// for parted regex
			runner.ReturnValue = []byte("Warning: Not all of the space available to /dev/loop0...\n" + partedPrint)
			Expect(dev.Reload()).To(BeNil())
			Expect(runner.MatchMilestones([][]string{
				{"parted", "--script", "--machine", "--", "/dev/device", "unit", "s", "print"},
				{"sgdisk", "-e", "/dev/device"},
				{"parted", "--script", "--machine", "--", "/dev/device", "unit", "s", "print"},
			})).To(BeNil())
			// for sgdisk regex
			dev = diskrepart.NewDisk(s, "/dev/device", diskrepart.WithGdisk())
			runner.ReturnValue = []byte(sgdiskPrint + "\nProblem: The secondary header's self-pointer indicates that...\n")
			runner.ClearCmds()
			Expect(dev.Reload()).To(BeNil())
			Expect(runner.MatchMilestones([][]string{
				{"sgdisk", "-p", "-v", "/dev/device"},
				{"sgdisk", "-e", "/dev/device"},
				{"sgdisk", "-p", "-v", "/dev/device"},
			})).To(BeNil())
		})
	})
	Describe("Modify disk", func() {
		It("Format an already existing partition", func() {
			Expect(diskrepart.FormatDevice(s, "/dev/device1", "ext4", "MY_LABEL", uuid)).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"mkfs.ext4", "-L", "MY_LABEL", "-U", uuid, "/dev/device1"},
			})).To(BeNil())
		})
		It("Fails to create an unsupported partition table label", func() {
			runner.ReturnValue = []byte(partedPrint)
			_, err := dev.NewPartitionTable("invalidLabel")
			Expect(err).NotTo(BeNil())
		})
		It("Creates new partition table label", func() {
			cmds = [][]string{{
				"parted", "--script", "--machine", "--", "/dev/device",
				"unit", "s", "mklabel", "gpt",
			}, {
				"partx", "-u", "/dev/device",
			}, printCmd}
			runner.ReturnValue = []byte(partedPrint)
			_, err := dev.NewPartitionTable("gpt")
			Expect(err).To(BeNil())
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Adds a new partition", func() {
			cmds = [][]string{printCmd, {
				"parted", "--script", "--machine", "--", "/dev/device",
				"unit", "s", "mkpart", "primary", "ext4", "50331648", "100%",
				"set", "5", "boot", "on",
			}, {
				"partx", "-u", "/dev/device",
			}, printCmd}
			runner.ReturnValue = []byte(partedPrint)
			num, err := dev.AddPartition(0, "ext4", "ignored", "boot")
			Expect(err).To(BeNil())
			Expect(num).To(Equal(5))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Fails to a new partition if there is not enough space available", func() {
			cmds = [][]string{printCmd}
			runner.ReturnValue = []byte(partedPrint)
			_, err := dev.AddPartition(130, "ext4", "ignored")
			Expect(err).NotTo(BeNil())
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Finds device for a given partition number", func() {
			_, err := fs.Create("/dev/device4")
			Expect(err).To(BeNil())
			cmds = [][]string{{"udevadm", "settle"}}
			Expect(dev.FindPartitionDevice(4)).To(Equal("/dev/device4"))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Does not find device for a given partition number", func() {
			dev := diskrepart.NewDisk(s, "/dev/lp0")
			_, err := dev.FindPartitionDevice(4)
			Expect(err).NotTo(BeNil())
		})
		It("Formats a partition", func() {
			_, err := fs.Create("/dev/device4")
			Expect(err).To(BeNil())
			cmds = [][]string{
				{"udevadm", "settle"},
				{"mkfs.xfs", "-L", "OEM", "-m", fmt.Sprintf("uuid=%s", uuid), "/dev/device4"},
			}
			Expect(dev.FormatPartition(4, "xfs", "OEM", uuid)).To(Succeed())
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Clears filesystem header from a partition", func() {
			cmds = [][]string{
				{"wipefs", "--all", "/dev/device1"},
			}
			Expect(dev.WipeFsOnPartition("/dev/device1")).To(BeNil())
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("Fails while removing file system header", func() {
			runner.ReturnError = errors.New("some error")
			Expect(dev.WipeFsOnPartition("/dev/device1")).NotTo(BeNil())
		})
		Describe("Expanding partitions", func() {
			BeforeEach(func() {
				cmds = [][]string{
					printCmd, {
						"parted", "--script", "--machine", "--", "/dev/device",
						"unit", "s", "rm", "4", "mkpart", "primary", "", "45019136", "100%",
					}, {
						"partx", "-u", "/dev/device",
					}, printCmd, {"udevadm", "settle"},
				}
				runFunc := func(cmd string, args ...string) ([]byte, error) {
					switch cmd {
					case "parted":
						return []byte(partedPrint), nil
					default:
						return []byte{}, nil
					}
				}
				runner.SideEffect = runFunc
			})
			It("Expands ext4 partition", func() {
				_, err := fs.Create("/dev/device4")
				Expect(err).To(BeNil())
				extCmds := [][]string{
					{"e2fsck", "-fy", "/dev/device4"}, {"resize2fs", "/dev/device4"},
				}
				bd.SetPartitions([]*block.Partition{
					{
						Disk:       "device",
						Path:       "/dev/device4",
						FileSystem: "ext4",
					},
				})
				err = dev.ExpandLastPartition(0)
				Expect(err).To(BeNil())
				Expect(runner.CmdsMatch(append(cmds, extCmds...))).To(BeNil())
			})
			It("Expands xfs partition", func() {
				_, err := fs.Create("/dev/device4")
				Expect(err).To(BeNil())
				xfsCmds := [][]string{{"xfs_growfs"}}
				bd.SetPartitions([]*block.Partition{
					{
						Disk:       "device",
						Path:       "/dev/device4",
						FileSystem: "xfs",
					},
				})
				err = dev.ExpandLastPartition(0)
				Expect(err).To(BeNil())
				Expect(runner.CmdsMatch(append(cmds, xfsCmds...))).To(BeNil())
			})
			It("Expands btrfs partition", func() {
				_, err := fs.Create("/dev/device4")
				Expect(err).To(BeNil())
				xfsCmds := [][]string{{"btrfs", "filesystem", "resize"}}
				bd.SetPartitions([]*block.Partition{
					{
						Disk:       "device",
						Path:       "/dev/device4",
						FileSystem: "btrfs",
					},
				})
				err = dev.ExpandLastPartition(0)
				Expect(err).To(BeNil())
				Expect(runner.CmdsMatch(append(cmds, xfsCmds...))).To(BeNil())
			})
		})
	})
})
