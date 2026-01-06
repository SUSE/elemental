/*
Copyright © 2022-2026 SUSE LLC
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

package lsblk_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

const sectorSizeLsblk = `{
   "blockdevices": [
      {
         "name": "nvme0n1",
         "phy-sec": 512
      }
   ]
}
`

const noSectorSizeLsblk = `{
   "blockdevices": [
      {
         "name": "nvme0n1"
      }
   ]
}
`

const fullLsblkTmpl = `{
   "blockdevices": [
      {
         "label": "EFI",
         "partlabel": "efi",
         "uuid": "236dacf0",
         "size": 272629760,
         "fstype": "vfat",
         "mountpoints": [
             "/boot"
         ],
         "path": "/dev/sda1",
         "pkname": "/dev/sda",
         "type": "part"
      }%s%s
   ]
}
`
const diskPortionLsblkOut = `,{
         "label": "DATA",
         "partlabel": "data",
         "size": 2147819008,
         "fstype": "ext4",
         "mountpoints": [
             "/data"
         ],
         "path": "/dev/sdb1",
         "pkname": "/dev/sdb",
         "type": "part"
      }`

const partsPortionLslbkOut = `,{
         "label": "STATE",
         "partlabel": "state",
         "uuid": "34a8abb8-ddb3-48a2-8ecc-2443e92c7510",
         "size": 351333777408,
         "fstype": "btrfs",
         "mountpoints": [
             "/.snapshots", "/", "/etc"
         ],
         "path": "/dev/sda2",
         "pkname": "/dev/sda",
         "type": "part"
      },{
         "label": "PERSISTENT",
         "partlabel": "persistent",
         "uuid": "236dacf0-b37e-4bca-a21a-59e4aef3ea4c",
         "size": 670454251520,
         "fstype": "xfs",
         "mountpoints": [
             "/home"
         ],
         "path": "/dev/sda3",
         "pkname": "/dev/sda",
         "type": "part"
      }`

func TestLsBlockSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LsBlock test suite")
}

var _ = Describe("BlockDevice", Label("lsblk"), func() {
	var runner *sysmock.Runner
	var b block.Device
	var json string
	var lsblkErr error
	var s *sys.System
	var err error
	BeforeEach(func() {
		runner = sysmock.NewRunner()

		s, err = sys.NewSystem(sys.WithRunner(runner))
		Expect(err).ToNot(HaveOccurred())
		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command == "lsblk" {
				if lsblkErr != nil {
					return []byte{}, lsblkErr
				}
				return []byte(json), nil
			}
			return []byte{}, nil
		}
		lsblkErr = nil
		b = lsblk.NewLsDevice(s)
	})
	Describe("GetPartitionFS", func() {
		BeforeEach(func() {
			json = fmt.Sprintf(fullLsblkTmpl, "", "")
		})
		It("returns the filesystem for the given partition", func() {
			fst, err := b.GetPartitionFS("/dev/sda1")
			Expect(err).NotTo(HaveOccurred())
			Expect(fst).To(Equal("vfat"))
		})
		It("lsblk call fails", func() {
			lsblkErr = fmt.Errorf("new lsblk error")
			_, err := b.GetPartitionFS("/dev/sda1")
			Expect(err).To(HaveOccurred())
		})
		It("fails when multiple partitions are listed", func() {
			json = fmt.Sprintf(fullLsblkTmpl, partsPortionLslbkOut, "")
			_, err := b.GetPartitionFS("/dev/sda")
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("GetDevicePartitions", func() {
		BeforeEach(func() {
			json = fmt.Sprintf(fullLsblkTmpl, partsPortionLslbkOut, "")
		})
		It("lists all partitions found by lsblk", func() {
			pl, err := b.GetDevicePartitions("/dev/sda")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(pl)).To(Equal(3))
			part := pl.GetByLabel("STATE")
			Expect(part).NotTo(BeNil())
			Expect(part.Path).To(Equal("/dev/sda2"))
			part = pl.GetByName("persistent")
			Expect(part).NotTo(BeNil())
			Expect(part.FileSystem).To(Equal("xfs"))
			part = pl.GetByUUIDNameOrLabel("invalidUUID", "wrongname", "EFI")
			Expect(part).NotTo(BeNil())
			Expect(part.Name).To(Equal("efi"))
		})
		It("lsblk call fails", func() {
			lsblkErr = fmt.Errorf("new lsblk error")
			_, err := b.GetDevicePartitions("/dev/sda")
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("GetAllPartitions", func() {
		BeforeEach(func() {
			json = fmt.Sprintf(fullLsblkTmpl, partsPortionLslbkOut, diskPortionLsblkOut)
		})
		It("lists all partitions found by lsblk", func() {
			pl, err := b.GetAllPartitions()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(pl)).To(Equal(4))
			part := pl.GetByLabel("STATE")
			Expect(part).NotTo(BeNil())
			Expect(part.Path).To(Equal("/dev/sda2"))
			part = pl.GetByName("persistent")
			Expect(part).NotTo(BeNil())
			Expect(part.FileSystem).To(Equal("xfs"))
			part = pl.GetByUUIDNameOrLabel("invalidUUID", "wrongname", "EFI")
			Expect(part).NotTo(BeNil())
			Expect(part.Name).To(Equal("efi"))
			part = pl.GetByUUIDNameOrLabel("invalidUUID", "data", "wronglable")
			Expect(part).NotTo(BeNil())
			Expect(part.Label).To(Equal("DATA"))
		})
		It("lsblk call fails", func() {
			lsblkErr = fmt.Errorf("new lsblk error")
			_, err := b.GetAllPartitions()
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("GetPartitionByLabel", func() {
		var cmds [][]string
		BeforeEach(func() {
			json = fmt.Sprintf(fullLsblkTmpl, partsPortionLslbkOut, diskPortionLsblkOut)
			cmds = [][]string{{"udevadm", "settle"}, {"lsblk"}}
		})
		It("returns found device", func() {
			out, err := block.GetPartitionDeviceByLabel(s, b, "STATE", 1)
			Expect(err).To(BeNil())
			Expect(out).To(Equal("/dev/sda2"))
			Expect(runner.CmdsMatch(cmds)).To(BeNil())
		})
		It("fails if no device is found in two attempts", func() {
			_, err := block.GetPartitionDeviceByLabel(s, b, "FAKE", 2)
			Expect(err).NotTo(BeNil())
			Expect(runner.CmdsMatch(append(cmds, cmds...))).To(BeNil())
		})
	})
	Describe("GetDeviceSectorSize", func() {
		It("parses the sector size of for the given device", func() {
			json = sectorSizeLsblk
			size, err := b.GetDeviceSectorSize("/dev/device")
			Expect(err).ToNot(HaveOccurred())
			Expect(size).To(Equal(uint(512)))
		})
		It("fails if no sector size is reported", func() {
			json = noSectorSizeLsblk
			_, err := b.GetDeviceSectorSize("/dev/device")
			Expect(err).To(MatchError(ContainSubstring("no sector size reported")))
		})
		It("fails if lsblk reports error", func() {
			lsblkErr = fmt.Errorf("lsblk call failed")
			_, err := b.GetDeviceSectorSize("/dev/device")
			Expect(err).To(MatchError(ContainSubstring("lsblk call failed")))
		})
	})
	Describe("GetByMountPoint", func() {
		BeforeEach(func() {
			json = fmt.Sprintf(fullLsblkTmpl, partsPortionLslbkOut, "")
		})
		It("finds a partition by its mountpoint", func() {
			parts, err := b.GetAllPartitions()
			Expect(err).NotTo(HaveOccurred())
			part := parts.GetByMountPoint("/etc")
			Expect(part).NotTo(BeNil())
			Expect(part.Label).To(Equal("STATE"))
			part = parts.GetByMountPoint("/nonexisting")
			Expect(part).To(BeNil())
		})
	})
	Describe("GetPartitionByMountPoint", func() {
		BeforeEach(func() {
			json = fmt.Sprintf(fullLsblkTmpl, partsPortionLslbkOut, "")
		})
		It("finds a partition by its mountpoint", func() {
			part, err := block.GetPartitionByMountPoint(s, b, "/boot", 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(part).NotTo(BeNil())
			Expect(part.Label).To(Equal("EFI"))
		})
	})
})
