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
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

func TestDeploymentSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deployment test suite")
}

var _ = Describe("Deployment", Label("deployment"), func() {
	var s *sys.System
	var tfs sys.FS
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
	It("fails if non last partitition is set to use all space available", func() {
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
					{Role: deployment.System},
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
})
