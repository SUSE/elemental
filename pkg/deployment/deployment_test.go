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
	"bytes"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestDeploymentSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deployment test suite")
}

var _ = Describe("Deployment", Label("deployment"), func() {
	Describe("Main Deployment tests", func() {
		var s *sys.System
		var tfs vfs.FS
		var cleanup func()
		var err error
		var buffer *bytes.Buffer

		BeforeEach(func() {
			buffer = &bytes.Buffer{}
			tfs, cleanup, err = sysmock.TestFS(map[string]string{
				"/dev/device": "device",
			})
			Expect(err).NotTo(HaveOccurred())
			s, err = sys.NewSystem(
				sys.WithFS(tfs),
				sys.WithLogger(log.New(log.WithBuffer(buffer))),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cleanup()
		})

		It("creates a default deployment", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Device = "/dev/device"
			Expect(d.Sanitize(s)).To(Succeed())
		})
		It("fails if device doesn't exist", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Device = "/doesntexist"
			Expect(d.Sanitize(s)).NotTo(Succeed())
		})
		It("fails if multiple efi partitions are set", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Partitions = append(d.Disks[0].Partitions, &deployment.Partition{
				Role: deployment.EFI,
			})
			err = d.Sanitize(s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("multiple 'efi'"))
		})
		It("fails if multiple system partitions are set", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Partitions = append(d.Disks[0].Partitions, &deployment.Partition{
				Role: deployment.System,
			})
			err = d.Sanitize(s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("multiple 'system'"))
		})
		It("fails if multiple recovery partitions are set", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Partitions = append(d.Disks[0].Partitions, []*deployment.Partition{
				{Role: deployment.Recovery}, {Role: deployment.Recovery},
			}...)
			err = d.Sanitize(s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("multiple 'recovery'"))
		})
		It("fails if non last partition is set to use all space available", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Partitions = append(d.Disks[0].Partitions, &deployment.Partition{
				Role: deployment.Recovery,
			})
			err = d.Sanitize(s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only last partition"))
		})
		It("fails if no system partition is defined", func() {
			d := &deployment.Deployment{
				Disks: []*deployment.Disk{
					{Partitions: []*deployment.Partition{{Role: deployment.Data}}},
				},
			}
			err = d.Sanitize(s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no 'system'"))
		})
		It("fails if no efi partition is defined", func() {
			d := &deployment.Deployment{
				Disks: []*deployment.Disk{
					{Partitions: []*deployment.Partition{{Role: deployment.System}}},
				},
			}
			err = d.Sanitize(s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no 'efi'"))
		})
		It("feeds default values even if some where undefined", func() {
			d := &deployment.Deployment{
				Disks: []*deployment.Disk{
					{Partitions: []*deployment.Partition{
						{Role: deployment.System, Size: 1024},
						{Role: deployment.EFI, RWVolumes: []deployment.RWVolume{{Path: "/some/path"}}},
						{Role: deployment.Data, Size: deployment.AllAvailableSize},
					}},
				},
			}
			Expect(d.Sanitize(s)).To(Succeed())
			Expect(d.Disks[0].Partitions[0].FileSystem).To(Equal(deployment.Btrfs))
			Expect(d.Disks[0].Partitions[0].MountPoint).To(Equal(deployment.SystemMnt))
			Expect(d.Disks[0].Partitions[1].FileSystem).To(Equal(deployment.VFat))
			Expect(d.Disks[0].Partitions[1].MountPoint).To(Equal(deployment.EfiMnt))
			Expect(len(d.Disks[0].Partitions[1].RWVolumes)).To(Equal(0))
			Expect(d.Disks[0].Partitions[2].FileSystem).To(Equal(deployment.Btrfs))
		})
		It("writes and reads deployment files", func() {
			d := deployment.DefaultDeployment()
			d.Disks[0].Device = "/dev/device"
			Expect(d.WriteDeploymentFile(s, "/some/dir")).To(Succeed())
			rD, err := deployment.Parse(s, "/some/dir")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(rD.Disks)).To(Equal(1))
			Expect(rD.Disks[0].Device).To(BeEmpty())
			Expect(len(rD.Disks[0].Partitions)).To(Equal(2))
			Expect(rD.Sanitize(s)).To(Succeed())
		})
		It("overwrites any pre-existing deployment file", func() {
			d := deployment.DefaultDeployment()
			Expect(d.WriteDeploymentFile(s, "/some/dir")).To(Succeed())
			d.Disks[0].Partitions[0].Label = "NEWEFI"
			Expect(d.WriteDeploymentFile(s, "/some/dir")).To(Succeed())
			rD, err := deployment.Parse(s, "/some/dir")
			Expect(err).NotTo(HaveOccurred())
			Expect(rD.Disks[0].Partitions[0].Label).To(Equal("NEWEFI"))
		})
		It("throws a warning trying to read a non existing deployment", func() {
			_, err := deployment.Parse(s, "/some/dir")
			Expect(err).NotTo(HaveOccurred())
			Expect(buffer.String()).To(ContainSubstring("deployment file not found"))
		})
	})

	Describe("Deployment utilities", Label("yaml"), func() {
		It("Un/marshals FileSystem", func() {
			filesystems := []string{"btrfs", "xfs", "ext2", "ext4", "vfat"}
			var t deployment.FileSystem

			for _, fs := range filesystems {
				Expect(yaml.Unmarshal([]byte(fs), &t)).To(Succeed())
				Expect(t.String()).To(Equal(fs))

				actual, err := yaml.Marshal(t)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(actual)).To(ContainSubstring(fs))
			}

			err := yaml.Unmarshal([]byte("not an fs"), &t)
			Expect(err).To(HaveOccurred())
		})
		It("Un/marshals PartRole", func() {
			roles := []string{"efi", "system", "recovery", "data"}
			var r deployment.PartRole

			for _, role := range roles {
				Expect(yaml.Unmarshal([]byte(role), &r)).To(Succeed())
				Expect(r.String()).To(Equal(role))

				actual, err := yaml.Marshal(r)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(actual)).To(ContainSubstring(role))
			}

			err := yaml.Unmarshal([]byte("not a partition role"), &r)
			Expect(err).To(HaveOccurred())
		})
	})
})
