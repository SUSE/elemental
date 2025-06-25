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

package installer_test

import (
	"fmt"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sut "github.com/suse/elemental/v3/tests/vm"
)

func getBootCount(s *sut.SUT) int {
	var b int

	bStr, err := s.Command("last | grep -c 'system boot'")
	Expect(err).ToNot(HaveOccurred())

	b, err = strconv.Atoi(strings.TrimSpace(bStr))
	Expect(err).ToNot(HaveOccurred())
	Expect(b).To(BeNumerically(">", 0))

	return b
}

var _ = Describe("Elemental Installer tests", func() {
	var s *sut.SUT
	var bootCount int

	BeforeEach(func() {
		s = sut.NewSUT()
		s.EventuallyConnects()
	})

	Context("HDD image boot", func() {
		// Assert we are booting from HDD image before running the tests
		It("Check that we booted from HDD image", func() {
			s.EventuallyBootedFrom(sut.Active)
		})

		It("Check /etc/os-release file", func() {
			By("checking NAME variable")
			Expect(s.GetOSRelease("NAME")).To(Equal("openSUSE Tumbleweed"))
		})

		It("Extract informations from booted OS", func() {
			osVersion := s.GetOSRelease("VERSION_ID")
			Expect(osVersion).ToNot(BeEmpty())

			osArch := s.GetArch()
			Expect(osVersion).ToNot(BeEmpty())

			// Only for debugging purposes and validation that SSH connection is working
			By(fmt.Sprintf("OS Version: %s", osVersion))
			By(fmt.Sprintf("OS Architecture: %s", osArch))
		})

		It("Test reboot", func() {
			bootCount = getBootCount(s)
			s.Reboot()
		})

		It("Test that the reboot was done", func() {
			c := getBootCount(s)
			Expect(c).To(BeNumerically(">", bootCount))

			By(fmt.Sprintf("Boot count before reboot: %d", bootCount))
			By(fmt.Sprintf("Boot count after reboot: %d", c))
		})
	})
})
