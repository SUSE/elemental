/*
Copyright © 2025-2026 SUSE LLC
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

package deployment_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
)

var _ = Describe("Deployment merge", Label("deployment"), func() {
	var dst *deployment.Deployment
	BeforeEach(func() {
		dst = deployment.New(
			deployment.WithPartitions(1, expectedRecoveryPart()),
		)
	})

	It("merges a new partition to dst deployment", func() {
		newPartition := &deployment.Partition{
			Label:      deployment.ConfigLabel,
			MountPoint: deployment.ConfigMnt,
			Role:       deployment.Data,
			FileSystem: deployment.Btrfs,
			Size:       deployment.MiB(1024),
			Hidden:     true,
		}

		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: []*deployment.Partition{
						// Skip EFI
						{},
						// Skip RECOVERY
						{},
						// Remove SYSTEM
						nil,
						newPartition,
						// Redefine SYSTEM
						expectedSysPart(),
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(4))

		Expect(dst.Disks[0].Partitions[0]).To(Equal(expectedEFIPart()))
		Expect(dst.Disks[0].Partitions[1]).To(Equal(expectedRecoveryPart()))
		Expect(dst.Disks[0].Partitions[2]).To(Equal(newPartition))
		Expect(dst.Disks[0].Partitions[3]).To(Equal(expectedSysPart()))
	})

	It("merges an existing partition to dst deployment", func() {
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: []*deployment.Partition{
						// Skip EFI
						{},
						// Make changes to RECOVERY
						{
							Label: "MERGED-RECOVERY",
							Size:  deployment.MiB(4096),
						},
					},
				},
			},
		}

		mergedRecoveryPartition := expectedRecoveryPart()
		mergedRecoveryPartition.Label = "MERGED-RECOVERY"
		mergedRecoveryPartition.Size = deployment.MiB(4096)

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(3))

		Expect(dst.Disks[0].Partitions[0]).To(Equal(expectedEFIPart()))
		Expect(dst.Disks[0].Partitions[1]).To(Equal(mergedRecoveryPartition))
		Expect(dst.Disks[0].Partitions[2]).To(Equal(expectedSysPart()))

	})

	It("removes dst partitions and adds new src partitions", func() {
		newPart1 := &deployment.Partition{
			Label: "NEW-PART-1",
			Size:  deployment.MiB(4096),
		}

		newPart2 := &deployment.Partition{
			Size:       deployment.MiB(2048),
			MountPoint: "/new-part",
			Hidden:     true,
		}

		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: []*deployment.Partition{
						// Remove EFI
						nil,
						// Remove RECOVERY
						nil,
						// Remove SYSTEM
						nil,
						newPart1,
						newPart2,
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(2))

		Expect(dst.Disks[0].Partitions[0]).To(Equal(newPart1))
		Expect(dst.Disks[0].Partitions[1]).To(Equal(newPart2))
	})

	It("mereges a new partition at the end of the dst partition slice", func() {
		newPart1 := &deployment.Partition{
			Label: "NEW-PART-1",
			Size:  deployment.MiB(4096),
		}
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: []*deployment.Partition{
						// Skip EFI
						{},
						// Skip RECOVERY
						{},
						// Skip SYSTEM
						{},
						newPart1,
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(4))

		Expect(dst.Disks[0].Partitions[0]).To(Equal(expectedEFIPart()))
		Expect(dst.Disks[0].Partitions[1]).To(Equal(expectedRecoveryPart()))
		Expect(dst.Disks[0].Partitions[2]).To(Equal(expectedSysPart()))
		Expect(dst.Disks[0].Partitions[3]).To(Equal(newPart1))
	})

	It("removes and merges existing partitions", func() {
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: []*deployment.Partition{
						// Edit EFI
						{
							MountPoint: "/boot-foo",
						},
						// Remove RECOVERY
						nil,
					},
				},
			},
		}

		mergedEFI := expectedEFIPart()
		mergedEFI.MountPoint = "/boot-foo"

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(2))

		Expect(dst.Disks[0].Partitions[0]).To(Equal(mergedEFI))
		Expect(dst.Disks[0].Partitions[1]).To(Equal(expectedSysPart()))
	})

	It("merges full src and dst deployments", func() {
		dst.SourceOS = deployment.NewEmptySrc()
		newPart1 := &deployment.Partition{
			Label:      "NEW-PART-1",
			MountPoint: "/foo/bar",
			MountOpts:  []string{"defaults", "x-systemd.automount"},
		}

		newPart2 := &deployment.Partition{
			Label:      "NEW-PART-2",
			MountPoint: "/boot/part-2",
			MountOpts:  []string{"defaults", "x-systemd.automount"},
		}

		src := &deployment.Deployment{
			SourceOS: deployment.NewOCISrc("domain.org/image/repo:tag"),
			Disks: []*deployment.Disk{
				{
					Device: "/dev/sda",
					Partitions: []*deployment.Partition{
						// Edit EFI
						{
							MountPoint: "/boot/efi/foo",
						},
						// Remove RECOVERY
						nil,
						// Remove SYSTEM
						nil,
						// Add custom partitions that
						// need to be present early at
						// the partition order
						newPart1,
						newPart2,
						// Redefine RECOVERY
						expectedRecoveryPart(),
						// Redefine SYSTEM
						expectedSysPart(),
					},
				},
				{
					Device: "/dev/device",
					Partitions: []*deployment.Partition{
						{
							Label: "foo",
						},
					},
				},
			},
			CfgScript: "script",
			BootConfig: &deployment.BootConfig{
				KernelCmdline: "new cmdline",
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(dst.SourceOS.String()).To(Equal("oci://domain.org/image/repo:tag"))

		Expect(len(dst.Disks)).To(Equal(2))
		mergedEFI := expectedEFIPart()
		mergedEFI.MountPoint = "/boot/efi/foo"
		Expect(dst.Disks[0].Device).To(Equal("/dev/sda"))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(5))
		Expect(dst.Disks[0].Partitions[0]).To(Equal(mergedEFI))
		Expect(dst.Disks[0].Partitions[1]).To(Equal(newPart1))
		Expect(dst.Disks[0].Partitions[2]).To(Equal(newPart2))
		Expect(dst.Disks[0].Partitions[3]).To(Equal(expectedRecoveryPart()))
		Expect(dst.Disks[0].Partitions[4]).To(Equal(expectedSysPart()))

		Expect(dst.Disks[1].Device).To(Equal("/dev/device"))
		Expect(len(dst.Disks[1].Partitions)).To(Equal(1))
		Expect(dst.Disks[1].Partitions[0].Label).To(Equal("foo"))

		Expect(dst.CfgScript).To(Equal("script"))
		Expect(dst.BootConfig.Bootloader).To(Equal(bootloader.BootNone))
		Expect(dst.BootConfig.KernelCmdline).To(Equal("new cmdline"))

	})
})

func expectedSysPart() *deployment.Partition {
	return &deployment.Partition{
		Label:      deployment.SystemLabel,
		Role:       deployment.System,
		MountPoint: deployment.SystemMnt,
		FileSystem: deployment.Btrfs,
		Size:       deployment.AllAvailableSize,
		MountOpts:  []string{"ro=vfs"},
		RWVolumes: []deployment.RWVolume{
			{Path: "/var", NoCopyOnWrite: true, MountOpts: []string{"x-initrd.mount"}},
			{Path: "/root", MountOpts: []string{"x-initrd.mount"}},
			{Path: "/etc", Snapshotted: true, MountOpts: []string{"x-initrd.mount"}},
			{Path: "/opt"}, {Path: "/srv"}, {Path: "/home"}, {Path: "/usr/local"},
		},
	}
}

func expectedRecoveryPart() *deployment.Partition {
	return &deployment.Partition{
		Role:      deployment.Recovery,
		Label:     deployment.RecoveryLabel,
		Size:      2048,
		MountOpts: []string{"defaults", "ro"},
	}
}

func expectedEFIPart() *deployment.Partition {
	return &deployment.Partition{
		Label:      deployment.EfiLabel,
		Role:       deployment.EFI,
		MountPoint: deployment.EfiMnt,
		FileSystem: deployment.VFat,
		Size:       deployment.EfiSize,
		MountOpts:  []string{"defaults", "x-systemd.automount"},
	}
}
