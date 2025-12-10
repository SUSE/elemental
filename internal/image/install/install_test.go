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

package install_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"

	"github.com/docker/go-units"
	"github.com/suse/elemental/v3/internal/image/install"
)

func TestDiskSizeTests(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiskSize test suite")
}

var _ = Describe("DiskSize", func() {
	It("IsValid() correctly handles sizes", func() {
		Expect(install.DiskSize("10G").IsValid()).To(BeTrue())
		Expect(install.DiskSize("4K").IsValid()).To(BeTrue())
		Expect(install.DiskSize("8M").IsValid()).To(BeTrue())
		Expect(install.DiskSize("-8M").IsValid()).To(BeFalse())
		Expect(install.DiskSize(" 8M").IsValid()).To(BeFalse())
	})

	It("ToMiB() tests", func() {
		Expect(install.DiskSize("10G").ToMiB()).To(Equal(uint(10 * units.GiB / units.MiB)))

		Expect(install.DiskSize("8M").ToMiB()).To(Equal(uint(8)))
	})
})
