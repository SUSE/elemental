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

package filesystem_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/filesystem"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

const validUUID = "236dacf0-b37e-4bca-a21a-59e4aef3ea4c"
const invalidUUID = "236dacf0-b37e-a21a-59e4aef3ea4c"
const vfatUUID = "236dacf0"

var _ = Describe("mkfs", Label("mkfs"), func() {
	var runner *sysmock.Runner
	var s *sys.System
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(sys.WithRunner(runner), sys.WithLogger(log.New(log.WithDiscardAll())))
		Expect(err).ToNot(HaveOccurred())
	})
	It("Successfully formats a partition with xfs", func() {
		mkfs := filesystem.NewMkfsCall(s, "/dev/device", "xfs", "OEM", validUUID)
		Expect(mkfs.Apply()).To(Succeed())
		cmds := [][]string{{"mkfs.xfs", "-L", "OEM", "-m", fmt.Sprintf("uuid=%s", validUUID), "-f", "/dev/device"}}
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Successfully formats a partition with btrfs", func() {
		mkfs := filesystem.NewMkfsCall(s, "/dev/device", "btrfs", "", validUUID, "--customopt")
		Expect(mkfs.Apply()).To(Succeed())
		cmds := [][]string{{"mkfs.btrfs", "-U", validUUID, "-f", "--customopt", "/dev/device"}}
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Successfully formats a partition with vfat", func() {
		mkfs := filesystem.NewMkfsCall(s, "/dev/device", "vfat", "EFI", strings.Split(validUUID, "-")[0])
		Expect(mkfs.Apply()).To(Succeed())
		cmds := [][]string{{"mkfs.vfat", "-n", "EFI", "-i", vfatUUID, "/dev/device"}}
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Fails for unsupported filesystem", func() {
		mkfs := filesystem.NewMkfsCall(s, "/dev/device", "zfs", "OEM", validUUID)
		Expect(mkfs.Apply()).ToNot(Succeed())
	})
})
