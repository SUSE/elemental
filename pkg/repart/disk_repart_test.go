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

package repart_test

import (
	"bytes"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/repart"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestRepartSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repart test suite")
}

const systemdRepartJson = `[
	{"uuid" : "c60d1845-7b04-4fc4-8639-8c49eb7277d5", "file" : "/tmp/elemental-repart.d/0-efi.conf"},
	{"uuid" : "ddb334a8-48a2-c4de-ddb3-849eb2443e92", "file" : "/tmp/elemental-repart.d/1-system.conf"}
]`

const sectorSizeJson = `{
   "blockdevices": [
      {
         "name": "device",
         "phy-sec": 512
      }
   ]
}`

var _ = Describe("Systemd-repart tests", Label("systemd-repart"), func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var tempDir string

	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		tempDir = "/temp/dir"
		Expect(vfs.MkdirAll(fs, tempDir, vfs.DirPerm)).To(Succeed())
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd == "systemd-repart" {
				return []byte(systemdRepartJson), runner.ReturnError
			}
			if cmd == "lsblk" {
				return []byte(sectorSizeJson), runner.ReturnError
			}
			return []byte{}, runner.ReturnError
		}
		Expect(vfs.MkdirAll(fs, "/dev", vfs.DirPerm)).To(Succeed())
		Expect(fs.WriteFile("/dev/device", []byte{}, vfs.FilePerm)).To(Succeed())
	})

	AfterEach(func() {
		cleanup()
	})

	It("creates a partition configuration", func() {
		var buffer bytes.Buffer
		part := &deployment.Partition{
			Label: "SYSTEM",
			Role:  deployment.System,
		}

		Expect(repart.CreatePartitionConf(&buffer, repart.Partition{Partition: part})).To(Succeed())
		Expect(buffer.String()).To(ContainSubstring("Type=root"))
		Expect(buffer.String()).ToNot(ContainSubstring("Format"))
		Expect(buffer.String()).ToNot(ContainSubstring("CopyFiles"))
		Expect(buffer.String()).ToNot(ContainSubstring("ExcludeFiles"))
		Expect(buffer.String()).ToNot(ContainSubstring("SizeMinBytes"))
		Expect(buffer.String()).ToNot(ContainSubstring("UUID"))
		Expect(buffer.String()).ToNot(ContainSubstring("ReadOnly"))

		buffer.Reset()
		part.Size = 1024
		part.FileSystem = deployment.Btrfs
		part.MountOpts = []string{"ro=vfs"}

		Expect(repart.CreatePartitionConf(
			&buffer, repart.Partition{
				Partition: part,
				CopyFiles: []string{"/some/root:/", "/some/other/root"},
				Excludes:  []string{"/some/root/excludeme"},
			},
		)).To(Succeed())

		Expect(buffer.String()).To(ContainSubstring("Type=root"))
		Expect(buffer.String()).To(ContainSubstring("SizeMinBytes=1024M"))
		Expect(buffer.String()).To(ContainSubstring("SizeMaxBytes=1024M"))
		Expect(buffer.String()).To(ContainSubstring("Format=btrfs"))
		Expect(buffer.String()).To(ContainSubstring("CopyFiles=/some/root:/"))
		Expect(buffer.String()).To(ContainSubstring("CopyFiles=/some/other/root"))
		Expect(buffer.String()).To(ContainSubstring("ExcludeFiles=/some/root/excludeme"))
		Expect(buffer.String()).To(ContainSubstring("ReadOnly=on"))
		Expect(buffer.String()).ToNot(ContainSubstring("UUID"))
	})

	It("creates a partition configuration file", func() {
		part := &deployment.Partition{
			Label: "SYSTEM",
			Role:  deployment.System,
		}
		configFile := filepath.Join(tempDir, "01-partition.conf")
		Expect(repart.CreatePartitionConfFile(s, configFile, repart.Partition{Partition: part})).To(Succeed())
		Expect(vfs.Exists(fs, configFile)).To(BeTrue())
	})

	It("creates a disk image with the given partitions", func() {
		diskImg := filepath.Join(tempDir, "image.raw")
		parts := []repart.Partition{
			{
				Partition: &deployment.Partition{
					Label: "EFI",
					Role:  deployment.EFI,
				},
				CopyFiles: []string{"/efi/path/in/host:/"},
			}, {
				Partition: &deployment.Partition{
					Label: "SYSTEM",
					Role:  deployment.System,
				},
				CopyFiles: []string{"/system/path/in/host:/"},
			},
		}

		Expect(repart.CreateDiskImage(s, diskImg, 1024, parts)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"systemd-repart", "--json=pretty", "--definitions=/tmp/elemental-repart.d",
			"--dry-run=no", "--empty=create", "--size=1024M", "/temp/dir/image.raw",
		}}))
		runner.ClearCmds()

		// Disk size set to auto if size is set to 0
		Expect(repart.CreateDiskImage(s, diskImg, 0, parts)).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"systemd-repart", "--json=pretty", "--definitions=/tmp/elemental-repart.d",
			"--dry-run=no", "--empty=create", "--size=auto", "/temp/dir/image.raw",
		}}))
	})

	It("reparts a disk with force flag and feeds partition UUIDs", func() {
		d := deployment.DefaultDeployment()
		Expect(len(d.Disks)).To(Equal(1))
		Expect(len(d.Disks[0].Partitions)).To(Equal(2))
		d.Disks[0].Device = "/dev/device"
		Expect(repart.PartitionAndFormatDevice(s, d.Disks[0])).To(Succeed())
		Expect(d.Disks[0].Partitions[0].UUID).To(Equal("c60d1845-7b04-4fc4-8639-8c49eb7277d5"))
		Expect(d.Disks[0].Partitions[1].UUID).To(Equal("ddb334a8-48a2-c4de-ddb3-849eb2443e92"))
		Expect(runner.MatchMilestones([][]string{{
			"systemd-repart", "--json=pretty", "--definitions=/tmp/elemental-repart.d",
			"--dry-run=no", "--empty=force", "--sector-size=512", "/dev/device",
		}}))
	})

	It("fails if systemd-repart reports partitions not matching the deployment", func() {
		d := deployment.DefaultDeployment()
		deployment.WithConfigPartition(0)(d)
		Expect(len(d.Disks)).To(Equal(1))
		d.Disks[0].Device = "/dev/device"
		Expect(repart.PartitionAndFormatDevice(s, d.Disks[0])).To(
			MatchError(ContainSubstring("matching partitions and systemd-repart JSON output")),
		)
	})

	It("reparts a disk with allow flag", func() {
		d := deployment.DefaultDeployment()
		Expect(len(d.Disks)).To(Equal(1))
		Expect(len(d.Disks[0].Partitions)).To(Equal(2))
		d.Disks[0].Device = "/dev/device"
		Expect(repart.ReconcileDevicePartitions(s, d.Disks[0])).To(Succeed())
		Expect(runner.MatchMilestones([][]string{{
			"systemd-repart", "--json=pretty", "--definitions=/tmp/elemental-repart.d",
			"--dry-run=no", "--empty=allow", "--sector-size=512", "/dev/device",
		}}))
	})

	It("fails if systemd-repart does not return a valid json", func() {
		runner.SideEffect = func(cmd string, args ...string) ([]byte, error) {
			if cmd == "lsblk" {
				return []byte(sectorSizeJson), runner.ReturnError
			}
			return []byte{}, runner.ReturnError
		}
		d := deployment.DefaultDeployment()
		Expect(len(d.Disks)).To(Equal(1))
		Expect(len(d.Disks[0].Partitions)).To(Equal(2))
		d.Disks[0].Device = "/dev/device"
		Expect(repart.ReconcileDevicePartitions(s, d.Disks[0])).To(
			MatchError(ContainSubstring("failed parsing systemd-repart JSON")),
		)
	})

	It("fails to create partition configuration with invalid data", func() {
		var buffer bytes.Buffer
		part := &deployment.Partition{
			Label: "SYSTEM",
		}
		Expect(repart.CreatePartitionConf(&buffer, repart.Partition{Partition: part})).To(
			MatchError(ContainSubstring("invalid partition role")),
		)

		part.Role = deployment.Data
		Expect(repart.CreatePartitionConf(
			&buffer, repart.Partition{
				Partition: part,
				CopyFiles: []string{"relative/path:/"},
			},
		)).To(
			MatchError(ContainSubstring("requires an absolute path")),
		)
	})
})
