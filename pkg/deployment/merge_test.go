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

package deployment_test

import (
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/deployment"
)

var _ = Describe("Deployment merge", Label("deployment"), func() {
	It("merges an empty source", func() {
		src := &deployment.Deployment{}
		dst := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Device: "/dev/sda",
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(dst.Disks[0].Device).To(Equal("/dev/sda"))
	})

	It("merges an empty src partition", func() {
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{},
				},
			},
		}
		dst := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{
						{
							Label: "SYSTEM",
						},
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(1))
		Expect(dst.Disks[0].Partitions[0].Label).To(Equal("SYSTEM"))
	})

	It("merges deployment disks", func() {
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Device: "/dev/sda",
				},
			},
		}
		dst := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{
						{
							Label: "SYSTEM",
						},
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(1))
		Expect(dst.Disks[0].Partitions[0].Label).To(Equal("SYSTEM"))
		Expect(dst.Disks[0].Device).To(Equal("/dev/sda"))
	})

	It("merges deployment partitions", func() {
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{
						{
							Label:     "SYSTEM",
							MountOpts: []string{"ro=vfs"},
						},
					},
				},
			},
		}

		dst := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{
						{
							Label:      "SYSTEM",
							MountPoint: "/",
							RWVolumes: deployment.RWVolumes{
								{
									Path: "/foo/bar",
								},
							},
						},
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(1))
		Expect(dst.Disks[0].Partitions[0].Label).To(Equal("SYSTEM"))
		Expect(dst.Disks[0].Partitions[0].MountPoint).To(Equal("/"))
		Expect(dst.Disks[0].Partitions[0].MountOpts).To(Equal([]string{"ro=vfs"}))
		Expect(dst.Disks[0].Partitions[0].RWVolumes[0].Path).To(Equal("/foo/bar"))

	})

	// TODO: duplicate partitions
	It("merges src deployment with duplicate partitions", func() {
		src := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{
						{
							Label: "SYSTEM",
							Role:  deployment.Data,
							Size:  deployment.MiB(2048),
						},
						{
							Label:      "SYSTEM",
							Size:       deployment.MiB(4096),
							FileSystem: deployment.Btrfs,
							MountPoint: deployment.SystemMnt,
						},
					},
				},
			},
		}

		dst := &deployment.Deployment{
			Disks: []*deployment.Disk{
				{
					Partitions: deployment.Partitions{
						{
							Label: "SYSTEM",
							Size:  deployment.MiB(1024),
							Role:  deployment.System,
							RWVolumes: []deployment.RWVolume{
								{Path: "/var", NoCopyOnWrite: true, MountOpts: []string{"x-initrd.mount"}},
								{Path: "/root", MountOpts: []string{"x-initrd.mount"}},
								{Path: "/etc", Snapshotted: true, MountOpts: []string{"x-initrd.mount"}},
								{Path: "/opt"}, {Path: "/srv"}, {Path: "/home"},
							},
						},
					},
				},
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(len(dst.Disks)).To(Equal(1))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(1))
		Expect(dst.Disks[0].Partitions[0].Label).To(Equal("SYSTEM"))
		Expect(dst.Disks[0].Partitions[0].Size).To(Equal(deployment.MiB(4096)))
		Expect(dst.Disks[0].Partitions[0].Role).To(Equal(deployment.System))
		Expect(dst.Disks[0].Partitions[0].RWVolumes).To(Equal(
			deployment.RWVolumes{
				{Path: "/var", NoCopyOnWrite: true, MountOpts: []string{"x-initrd.mount"}},
				{Path: "/root", MountOpts: []string{"x-initrd.mount"}},
				{Path: "/etc", Snapshotted: true, MountOpts: []string{"x-initrd.mount"}},
				{Path: "/opt"}, {Path: "/srv"}, {Path: "/home"},
			},
		))
		Expect(dst.Disks[0].Partitions[0].FileSystem).To(Equal(deployment.Btrfs))
		Expect(dst.Disks[0].Partitions[0].MountPoint).To(Equal(deployment.SystemMnt))
	})

	It("merges full deployments", func() {
		dst := deployment.New(
			deployment.WithPartitions(1, &deployment.Partition{
				Role:      deployment.Recovery,
				Label:     deployment.RecoveryLabel,
				Size:      2048,
				MountOpts: []string{"defaults", "ro"},
			}),
			deployment.WithConfigPartition(1024),
		)
		dst.SourceOS = deployment.NewEmptySrc()

		src := &deployment.Deployment{
			SourceOS: deployment.NewOCISrc("domain.org/image/repo:tag"),
			Disks: []*deployment.Disk{
				{
					Device: "/dev/sda",
					Partitions: deployment.Partitions{
						{
							Label: deployment.RecoveryLabel,
							Role:  deployment.Recovery,
							Size:  4096,
							RWVolumes: deployment.RWVolumes{
								{
									Path: "/tmp",
								},
							},
						},
						{
							Label:     "CUSTOM-PARTITION",
							Role:      deployment.Data,
							Size:      2048,
							MountOpts: []string{"defaults", "ro"},
						},
						{
							Role:      deployment.Data,
							Size:      2048,
							MountOpts: []string{"defaults", "x-systemd.automount"},
						},
						nil,
						{
							Label: "newLabel",
							Size:  4096,
						},
						nil,
					},
				},
				{
					Device: "/dev/device",
					Partitions: deployment.Partitions{
						{
							Label: "foo",
						},
					},
				},
			},
			CfgScript: "script",
			BootConfig: &deployment.BootConfig{
				Bootloader:    "grub",
				KernelCmdline: "new cmdline",
			},
		}

		Expect(deployment.Merge(dst, src)).To(Succeed())
		Expect(dst.SourceOS).To(Equal(deployment.NewOCISrc("domain.org/image/repo:tag")))

		Expect(len(dst.Disks)).To(Equal(2))
		Expect(dst.Disks[0].Device).To(Equal("/dev/sda"))
		Expect(len(dst.Disks[0].Partitions)).To(Equal(7))

		mappedParts := getPartitionMap(dst.Disks[0].Partitions)
		Expect(len(dst.Disks[0].Partitions)).To(Equal(len(mappedParts)))

		Expect(mappedParts["unknown-1"].Partition).ToNot(BeNil())
		Expect(mappedParts["unknown-1"].Index).To(Equal(0))
		Expect(mappedParts["unknown-1"].Partition.Role).To(Equal(deployment.Data))
		Expect(mappedParts["unknown-1"].Partition.Size).To(Equal(deployment.MiB(2048)))
		Expect(mappedParts["unknown-1"].Partition.MountOpts).To(Equal([]string{"defaults", "x-systemd.automount"}))

		Expect(mappedParts["CUSTOM-PARTITION"].Partition).ToNot(BeNil())
		Expect(mappedParts["CUSTOM-PARTITION"].Partition.Label).To(Equal("CUSTOM-PARTITION"))
		Expect(mappedParts["CUSTOM-PARTITION"].Partition.Role).To(Equal(deployment.Data))
		Expect(mappedParts["CUSTOM-PARTITION"].Partition.Size).To(Equal(deployment.MiB(2048)))
		Expect(mappedParts["CUSTOM-PARTITION"].Partition.MountOpts).To(Equal([]string{"defaults", "ro"}))

		Expect(mappedParts["newLabel"].Partition).ToNot(BeNil())
		Expect(mappedParts["newLabel"].Partition.Label).To(Equal("newLabel"))
		Expect(mappedParts["newLabel"].Partition.Size).To(Equal(deployment.MiB(4096)))

		Expect(mappedParts[deployment.ConfigLabel].Partition).ToNot(BeNil())
		Expect(mappedParts[deployment.ConfigLabel].Partition.Label).To(Equal(deployment.ConfigLabel))
		Expect(mappedParts[deployment.ConfigLabel].Partition.MountPoint).To(Equal(deployment.ConfigMnt))
		Expect(mappedParts[deployment.ConfigLabel].Partition.Role).To(Equal(deployment.Data))
		Expect(mappedParts[deployment.ConfigLabel].Partition.FileSystem).To(Equal(deployment.Btrfs))
		Expect(mappedParts[deployment.ConfigLabel].Partition.Size).To(Equal(deployment.MiB(1280)))
		Expect(mappedParts[deployment.ConfigLabel].Partition.Hidden).To(Equal(true))

		Expect(mappedParts[deployment.RecoveryLabel].Partition).ToNot(BeNil())
		Expect(mappedParts[deployment.RecoveryLabel].Partition.Label).To(Equal(deployment.RecoveryLabel))
		Expect(mappedParts[deployment.RecoveryLabel].Partition.Role).To(Equal(deployment.Recovery))
		Expect(mappedParts[deployment.RecoveryLabel].Partition.Size).To(Equal(deployment.MiB(4096)))
		Expect(mappedParts[deployment.RecoveryLabel].Partition.MountOpts).To(Equal([]string{"defaults", "ro"}))
		Expect(mappedParts[deployment.RecoveryLabel].Partition.RWVolumes).ToNot(BeEmpty())
		Expect(mappedParts[deployment.RecoveryLabel].Partition.RWVolumes[0].Path).To(Equal("/tmp"))

		Expect(mappedParts[deployment.EfiLabel].Partition).ToNot(BeNil())
		Expect(mappedParts[deployment.EfiLabel].Partition.Label).To(Equal(deployment.EfiLabel))
		Expect(mappedParts[deployment.EfiLabel].Partition.Role).To(Equal(deployment.EFI))
		Expect(mappedParts[deployment.EfiLabel].Partition.Size).To(Equal(deployment.EfiSize))
		Expect(mappedParts[deployment.EfiLabel].Partition.MountOpts).To(Equal([]string{"defaults", "x-systemd.automount"}))
		Expect(mappedParts[deployment.EfiLabel].Partition.MountPoint).To(Equal(deployment.EfiMnt))
		Expect(mappedParts[deployment.EfiLabel].Partition.FileSystem).To(Equal(deployment.VFat))

		Expect(mappedParts[deployment.EfiLabel].Partition).ToNot(BeNil())
		Expect(mappedParts[deployment.EfiLabel].Partition.Label).To(Equal(deployment.EfiLabel))
		Expect(mappedParts[deployment.EfiLabel].Partition.Role).To(Equal(deployment.EFI))
		Expect(mappedParts[deployment.EfiLabel].Partition.Size).To(Equal(deployment.EfiSize))
		Expect(mappedParts[deployment.EfiLabel].Partition.MountOpts).To(Equal([]string{"defaults", "x-systemd.automount"}))
		Expect(mappedParts[deployment.EfiLabel].Partition.MountPoint).To(Equal(deployment.EfiMnt))
		Expect(mappedParts[deployment.EfiLabel].Partition.FileSystem).To(Equal(deployment.VFat))

		Expect(mappedParts[deployment.SystemLabel].Partition).ToNot(BeNil())
		Expect(mappedParts[deployment.SystemLabel].Partition.Label).To(Equal(deployment.SystemLabel))
		Expect(mappedParts[deployment.SystemLabel].Index).To(Equal(len(dst.Disks[0].Partitions) - 1))
		Expect(mappedParts[deployment.SystemLabel].Partition.Role).To(Equal(deployment.System))
		Expect(mappedParts[deployment.SystemLabel].Partition.MountPoint).To(Equal(deployment.SystemMnt))
		Expect(mappedParts[deployment.SystemLabel].Partition.FileSystem).To(Equal(deployment.Btrfs))
		Expect(mappedParts[deployment.SystemLabel].Partition.Size).To(Equal(deployment.AllAvailableSize))
		Expect(mappedParts[deployment.SystemLabel].Partition.MountOpts).To(Equal([]string{"ro=vfs"}))
		Expect(mappedParts[deployment.SystemLabel].Partition.RWVolumes).ToNot(BeEmpty())
		Expect(mappedParts[deployment.SystemLabel].Partition.RWVolumes).To(Equal(
			deployment.RWVolumes{
				{Path: "/var", NoCopyOnWrite: true, MountOpts: []string{"x-initrd.mount"}},
				{Path: "/root", MountOpts: []string{"x-initrd.mount"}},
				{Path: "/etc", Snapshotted: true, MountOpts: []string{"x-initrd.mount"}},
				{Path: "/opt"}, {Path: "/srv"}, {Path: "/home"},
			}))

		Expect(dst.Disks[1].Device).To(Equal("/dev/device"))
		Expect(len(dst.Disks[1].Partitions)).To(Equal(1))
		Expect(dst.Disks[1].Partitions[0].Label).To(Equal("foo"))

		Expect(dst.CfgScript).To(Equal("script"))
		Expect(dst.BootConfig.Bootloader).To(Equal("grub"))
		Expect(dst.BootConfig.KernelCmdline).To(Equal("new cmdline"))

		Expect(dst.Snapshotter.Name).To(Equal("snapper"))
	})
})

type mappedPartition struct {
	Index     int
	Partition *deployment.Partition
}

func getPartitionMap(parts []*deployment.Partition) map[string]*mappedPartition {
	mappedParts := map[string]*mappedPartition{}
	var unknownLabelCounter int

	for i, p := range parts {
		if p.Label == "" {
			unknownLabelCounter++
			unknown := fmt.Sprintf("unknown-%s", strconv.Itoa(unknownLabelCounter))
			mappedParts[unknown] = &mappedPartition{
				Index:     i,
				Partition: p,
			}
			continue
		}

		mappedParts[p.Label] = &mappedPartition{
			Index:     i,
			Partition: p,
		}
	}

	return mappedParts
}
