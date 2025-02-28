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

package diskrepart_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

var _ = Describe("Parted", Label("parted"), func() {
	var runner *sysmock.Runner
	var s *sys.System
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(sys.WithRunner(runner))
		Expect(err).ToNot(HaveOccurred())
	})
	It("Successfully formats a partition with xfs", func() {
		mkfs := diskrepart.NewMkfsCall(s, "/dev/device", "xfs", "OEM")
		_, err := mkfs.Apply()
		Expect(err).To(BeNil())
		cmds := [][]string{{"mkfs.xfs", "-L", "OEM", "/dev/device"}}
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Successfully formats a partition with btrfs", func() {
		mkfs := diskrepart.NewMkfsCall(s, "/dev/device", "btrfs", "", "--customopt")
		_, err := mkfs.Apply()
		Expect(err).To(BeNil())
		cmds := [][]string{{"mkfs.btrfs", "--customopt", "-f", "/dev/device"}}
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Successfully formats a partition with vfat", func() {
		mkfs := diskrepart.NewMkfsCall(s, "/dev/device", "vfat", "EFI")
		_, err := mkfs.Apply()
		Expect(err).To(BeNil())
		cmds := [][]string{{"mkfs.vfat", "-n", "EFI", "/dev/device"}}
		Expect(runner.CmdsMatch(cmds)).To(BeNil())
	})
	It("Fails for unsupported filesystem", func() {
		mkfs := diskrepart.NewMkfsCall(s, "/dev/device", "zfs", "OEM")
		_, err := mkfs.Apply()
		Expect(err).NotTo(BeNil())
	})
})
