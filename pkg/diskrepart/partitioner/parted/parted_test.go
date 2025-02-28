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

package parted_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner"
	"github.com/suse/elemental/v3/pkg/diskrepart/partitioner/parted"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

const partedPrint = `BYT;
/dev/loop0:50593792s:loopback:512:512:msdos:Loopback device:;
1:2048s:98303s:96256s:ext4::type=83;
2:98304s:29394943s:29296640s:ext4::boot, type=83;
3:29394944s:45019135s:15624192s:ext4::type=83;
4:45019136s:50331647s:5312512s:ext4::type=83;`

func TestPartedSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Parted test suite")
}

var _ = Describe("Parted", Label("parted"), func() {
	var runner *sysmock.Runner
	var pc partitioner.Partitioner
	var s *sys.System
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(sys.WithRunner(runner))
		Expect(err).ToNot(HaveOccurred())
		pc = parted.NewPartedCall(s, "/dev/device")
	})
	It("Write changes does nothing with empty setup", func() {
		pc = parted.NewPartedCall(s, "/dev/device")
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
	})
	It("Runs complex command", func() {
		cmds := [][]string{{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "mklabel", "gpt", "mkpart", "p.efi", "fat32",
			"2048", "206847", "mkpart", "p.root", "ext4", "206848", "100%",
		}, {
			"partx", "-u", "/dev/device",
		}}
		part1 := partitioner.Partition{
			Number: 0, StartS: 2048, SizeS: 204800,
			PLabel: "p.efi", FileSystem: "vfat",
		}
		pc.CreatePartition(&part1)
		part2 := partitioner.Partition{
			Number: 0, StartS: 206848, SizeS: 0,
			PLabel: "p.root", FileSystem: "ext4",
		}
		pc.CreatePartition(&part2)
		pc.WipeTable(true)
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Set a new partition label", func() {
		cmds := [][]string{{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "mklabel", "msdos",
		}, {
			"partx", "-u", "/dev/device",
		}}
		pc.SetPartitionTableLabel("msdos")
		pc.WipeTable(true)
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Creates a new partition", func() {
		cmds := [][]string{{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "mkpart", "p.root", "ext4", "2048", "206847",
		}, {
			"partx", "-u", "/dev/device",
		}, {
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "mkpart", "p.root", "ext4", "2048", "100%",
		}, {
			"partx", "-u", "/dev/device",
		}}
		partition := partitioner.Partition{
			Number: 0, StartS: 2048, SizeS: 204800,
			PLabel: "p.root", FileSystem: "ext4",
		}
		pc.CreatePartition(&partition)
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
		partition = partitioner.Partition{
			Number: 0, StartS: 2048, SizeS: 0,
			PLabel: "p.root", FileSystem: "ext4",
		}
		pc.CreatePartition(&partition)
		_, err = pc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Deletes a partition", func() {
		cmds := [][]string{{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "rm", "1", "rm", "2",
		}, {
			"partx", "-u", "/dev/device",
		}}
		pc.DeletePartition(1)
		pc.DeletePartition(2)
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Set a partition flag", func() {
		cmds := [][]string{{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "set", "1", "flag", "on", "set", "2", "flag", "off",
		}, {
			"partx", "-u", "/dev/device",
		}}
		pc.SetPartitionFlag(1, "flag", true)
		pc.SetPartitionFlag(2, "flag", false)
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Wipes partition table creating a new one", func() {
		cmds := [][]string{{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "mklabel", "gpt",
		}, {
			"partx", "-u", "/dev/device",
		}}
		pc.WipeTable(true)
		_, err := pc.WriteChanges()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Prints partitin table info", func() {
		cmd := []string{
			"parted", "--script", "--machine", "--", "/dev/device",
			"unit", "s", "print",
		}
		_, err := pc.Print()
		Expect(err).To(BeNil())
		Expect(runner.CmdsMatch([][]string{cmd})).To(BeNil())
	})
	It("Gets last sector of the disk", func() {
		lastSec, _ := pc.GetLastSector(partedPrint)
		Expect(lastSec).To(Equal(uint(50593792)))
		_, err := pc.GetLastSector("invalid parted print output")
		Expect(err).NotTo(BeNil())
	})
	It("Gets sector size of the disk", func() {
		secSize, _ := pc.GetSectorSize(partedPrint)
		Expect(secSize).To(Equal(uint(512)))
		_, err := pc.GetSectorSize("invalid parted print output")
		Expect(err).NotTo(BeNil())
	})
	It("Gets partition table label", func() {
		label, _ := pc.GetPartitionTableLabel(partedPrint)
		Expect(label).To(Equal("msdos"))
		_, err := pc.GetPartitionTableLabel("invalid parted print output")
		Expect(err).NotTo(BeNil())
	})
	It("Gets partitions info of the disk", func() {
		parts := pc.GetPartitions(partedPrint)
		Expect(len(parts)).To(Equal(4))
		Expect(parts[1].StartS).To(Equal(uint(98304)))
	})
})
