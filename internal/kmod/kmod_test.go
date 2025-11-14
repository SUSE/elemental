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

package kmod

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestKernelModulesSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kernel modules test suite")
}

var _ = Describe("Kernel modules tests", func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()

	BeforeEach(func() {
		var err error

		tfs, cleanup, err = mock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		s, err = sys.NewSystem(sys.WithFS(tfs))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	var _ = Describe("Listing kernel modules", func() {
		It("Fails to list kernel modules", func() {
			modules, err := ListKernelModules(s)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("deployment information not found"))
			Expect(modules).To(BeEmpty())
		})

		It("Kernel modules are not part of the deployment", func() {
			Expect(vfs.MkdirAll(tfs, "/etc/elemental", 0755)).To(Succeed())
			Expect(tfs.WriteFile("/etc/elemental/deployment.yaml", []byte{}, 0600)).To(Succeed())

			modules, err := ListKernelModules(s)
			Expect(err).NotTo(HaveOccurred())
			Expect(modules).To(BeEmpty())
		})

		It("Finds kernel modules in the deployment", func() {
			deployment := []byte(`
extensions:
    kernelModules:
        - nvidia
        - nvidia-drm`)

			Expect(vfs.MkdirAll(tfs, "/etc/elemental", 0755)).To(Succeed())
			Expect(tfs.WriteFile("/etc/elemental/deployment.yaml", deployment, 0600)).To(Succeed())

			modules, err := ListKernelModules(s)
			Expect(err).NotTo(HaveOccurred())
			Expect(modules).To(ConsistOf("nvidia", "nvidia-drm"))
		})
	})
})
