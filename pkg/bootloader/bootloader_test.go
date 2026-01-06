/*
Copyright © 2022-2026 SUSE LLC
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

package bootloader_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
)

func TestBootloaderSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootloader test suite")
}

var _ = Describe("Bootloader tests", Label("bootloader", "grub", "none"), func() {
	var s *sys.System
	BeforeEach(func() {
		var err error
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
	})
	It("Successsfully creates a new bootloader", func() {
		for _, name := range []string{"none", "grub"} {
			b, err := bootloader.New(name, s)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).NotTo(BeNil())
		}
	})
	It("New() returns unsupported error for unknown bootloader", func() {
		b, err := bootloader.New("bogus", s)
		Expect(b).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, errors.ErrUnsupported)).To(BeTrue(), err.Error())
	})
})
