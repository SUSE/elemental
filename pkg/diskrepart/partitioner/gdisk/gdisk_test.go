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

package gdisk_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner/gdisk"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

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

func TestGdiskSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gdisk test suite")
}

var _ = Describe("Parted", Label("parted"), func() {
	var runner *sysmock.Runner
	var gc partitioner.Partitioner
	var s *sys.System
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(sys.WithRunner(runner))
		Expect(err).ToNot(HaveOccurred())
		gc = gdisk.NewGdiskCall(s, "/dev/device")
	})
	It("Write changes does nothing with empty setup", func() {
		gc := gdisk.NewGdiskCall(s, "/dev/device")
		_, err := gc.WriteChanges()
		Expect(err).To(BeNil())
	})
	It("Runs complex command", func() {
		cmds := [][]string{
			{"sgdisk", "-P", "--zap-all", "-n=0:2048:+204800", "-c=0:p.efi", "-t=0:EF00",
				"-n=1:206848:+0", "-c=1:p.root", "-t=1:8300", "/dev/device"},
			{"sgdisk", "--zap-all", "-n=0:2048:+204800", "-c=0:p.efi", "-t=0:EF00",
				"-n=1:206848:+0", "-c=1:p.root", "-t=1:8300", "/dev/device"},
			{"partx", "-u", "/dev/device"},
		}
		part1 := partitioner.Partition{
			Number: 0, StartS: 2048, SizeS: 204800,
			PLabel: "p.efi", FileSystem: "vfat",
		}
		gc.CreatePartition(&part1)
		part2 := partitioner.Partition{
			Number: 1, StartS: 206848, SizeS: 0,
			PLabel: "p.root", FileSystem: "ext4",
		}
		gc.CreatePartition(&part2)
		gc.WipeTable(true)
		_, err := gc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Set a new partition label", func() {
		cmds := [][]string{
			{"sgdisk", "-P", "--zap-all", "/dev/device"},
			{"sgdisk", "--zap-all", "/dev/device"},
			{"partx", "-u", "/dev/device"},
		}
		Expect(gc.SetPartitionTableLabel("gpt")).To(Succeed())
		gc.WipeTable(true)
		_, err := gc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Fails setting a new partition label", func() {
		Expect(gc.SetPartitionTableLabel("msdos")).NotTo(Succeed())
	})
	It("Creates a new partition", func() {
		cmds := [][]string{
			{"sgdisk", "-n=0:2048:+204800", "-c=0:p.root", "-t=0:8300", "/dev/device"},
			{"partx", "-u", "/dev/device"},
			{"sgdisk", "-n=0:2048:+0", "-c=0:p.root", "-t=0:8300", "/dev/device"},
			{"partx", "-u", "/dev/device"},
		}
		partition := partitioner.Partition{
			Number: 0, StartS: 2048, SizeS: 204800,
			PLabel: "p.root", FileSystem: "ext4",
		}
		gc.CreatePartition(&partition)
		_, err := gc.WriteChanges()
		Expect(err).To(BeNil())
		partition = partitioner.Partition{
			Number: 0, StartS: 2048, SizeS: 0,
			PLabel: "p.root", FileSystem: "ext4",
		}
		gc.CreatePartition(&partition)
		_, err = gc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.MatchMilestones(cmds)).To(BeNil())
	})
	It("Deletes a partition", func() {
		cmds := [][]string{
			{"sgdisk", "-P", "-d=1", "-d=2", "/dev/device"},
			{"sgdisk", "-d=1", "-d=2", "/dev/device"},
			{"partx", "-u", "/dev/device"},
		}
		gc.DeletePartition(1)
		gc.DeletePartition(2)
		_, err := gc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Wipes partition table creating a new one", func() {
		cmds := [][]string{
			{"sgdisk", "-P", "--zap-all", "/dev/device"}, {"sgdisk", "--zap-all", "/dev/device"},
			{"partx", "-u", "/dev/device"},
		}
		gc.WipeTable(true)
		_, err := gc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Prints partition table info", func() {
		cmd := []string{"sgdisk", "-p", "-v", "/dev/device"}
		_, err := gc.Print()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch([][]string{cmd})).To(BeNil())
	})
	It("Gets last sector of the disk", func() {
		lastSec, _ := gc.GetLastSector(sgdiskPrint)
		Expect(lastSec).To(Equal(uint(500118158)))
		_, err := gc.GetLastSector("invalid parted print output")
		Expect(err).NotTo(BeNil())
	})
	It("Gets sector size of the disk", func() {
		secSize, _ := gc.GetSectorSize(sgdiskPrint)
		Expect(secSize).To(Equal(uint(512)))
		_, err := gc.GetSectorSize("invalid parted print output")
		Expect(err).NotTo(BeNil())
	})
	It("Gets partition table label", func() {
		label, _ := gc.GetPartitionTableLabel(sgdiskPrint)
		Expect(label).To(Equal("gpt"))
	})
	It("Gets partitions info of the disk", func() {
		parts := gc.GetPartitions(sgdiskPrint)
		// Ignores swap partition
		Expect(len(parts)).To(Equal(2))
		Expect(parts[1].StartS).To(Equal(uint(17303552)))
	})
})
