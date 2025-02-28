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

package ghw_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/ghw"
	blockmock "github.com/suse/elemental/v3/pkg/block/mock"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"

	gblock "github.com/jaypipes/ghw/pkg/block"
)

func TestGhwSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ghw test suite")
}

var _ = Describe("BlockDevice", Label("ghw"), func() {
	var ghwTest blockmock.GhwMock
	var ghwBlock block.Device
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var s *sys.System
	var err error
	BeforeEach(func() {
		ghwTest = blockmock.GhwMock{}
		mounter = sysmock.NewMounter()
		runner = sysmock.NewRunner()
		s, err = sys.NewSystem(sys.WithRunner(runner), sys.WithMounter(mounter))
		Expect(err).NotTo(HaveOccurred())
		ghwBlock = ghw.NewGhwDevice(s)
	})
	AfterEach(func() {
		ghwTest.Clean()
	})
	Describe("GetPartitionFS", func() {
		BeforeEach(func() {
			disk := gblock.Disk{Name: "device", Partitions: []*gblock.Partition{
				{
					Name: "device1",
					Type: "xfs",
				},
				{
					Name: "device2",
				},
			}}
			ghwTest.AddDisk(disk)
			ghwTest.CreateDevices()
		})
		It("returns found device with plain partition device", func() {
			pFS, err := ghwBlock.GetPartitionFS("device1")
			Expect(err).To(BeNil())
			Expect(pFS).To(Equal("xfs"))
		})
		It("returns found device with full partition device", func() {
			pFS, err := ghwBlock.GetPartitionFS("/dev/device1")
			Expect(err).To(BeNil())
			Expect(pFS).To(Equal("xfs"))
		})
		It("fails if no partition is found", func() {
			_, err := ghwBlock.GetPartitionFS("device2")
			Expect(err).NotTo(BeNil())
		})
	})
	Describe("Get Partitions", func() {
		BeforeEach(func() {
			disk1 := gblock.Disk{
				Name: "sda",
				Partitions: []*gblock.Partition{
					{
						Name:            "sda1Test",
						MountPoint:      "/mnt",
						FilesystemLabel: "LABEL",
					},
					{
						Name: "sda2Test",
					},
				},
			}
			disk2 := gblock.Disk{
				Name: "sdb",
				Partitions: []*gblock.Partition{
					{
						Name: "sdb1Test",
					},
				},
			}
			mounter.Mount("/dev/sda1Test", "/mnt", "ext4", []string{"defaults"})
			mounter.Mount("/dev/sda1Test", "/data", "ext4", []string{"defaults"})
			ghwTest.AddDisk(disk1)
			ghwTest.AddDisk(disk2)
			ghwTest.CreateDevices()
		})
		It("returns all found partitions and mountpoints", func() {
			parts, err := ghwBlock.GetAllPartitions()
			Expect(err).To(BeNil())
			var devices []string
			for _, p := range parts {
				devices = append(devices, p.Path)
			}
			p := parts.GetByLabel("LABEL")
			Expect(p).NotTo(BeNil())
			Expect(p.MountPoints).To(ContainElements("/mnt", "/data"))
			Expect(devices).To(ContainElement(ContainSubstring("sda1Test")))
			Expect(devices).To(ContainElement(ContainSubstring("sda2Test")))
			Expect(devices).To(ContainElement(ContainSubstring("sdb1Test")))
		})
		It("returns found partitions for /dev/sda", func() {
			parts, err := ghwBlock.GetDevicePartitions("/dev/sda")
			Expect(err).To(BeNil())
			var devices []string
			for _, p := range parts {
				devices = append(devices, p.Path)
			}
			Expect(devices).To(ContainElement(ContainSubstring("sda1Test")))
			Expect(devices).To(ContainElement(ContainSubstring("sda2Test")))
			Expect(devices).NotTo(ContainElement(ContainSubstring("sdb1Test")))
		})
	})
})
