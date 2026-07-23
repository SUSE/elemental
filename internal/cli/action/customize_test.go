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

package action

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
)

var _ = Describe("Test validateLifecycleManager", func() {
	const errorMsg = "chart \"elemental-lifecycle-manager\" must be listed under components.helm in release.yaml when Kubernetes is enabled"

	It("passes when Kubernetes is not enabled", func() {
		Expect(validateLifecycleManager(&image.Configuration{})).To(Succeed())
	})

	It("fails when Kubernetes is enabled but LCM is missing", func() {
		conf := image.Configuration{
			Release: release.Release{Components: release.Components{Kubernetes: &struct{}{}}},
		}
		err := validateLifecycleManager(&conf)
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(Equal(errorMsg))
	})

	It("fails when not listed under Helm charts", func() {
		conf := image.Configuration{
			Release: release.Release{
				Components: release.Components{
					HelmCharts: []release.HelmChart{{Name: "not-lcm"}},
				},
			},
		}
		err := validateLifecycleManager(&conf)
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(Equal(errorMsg))
	})

	It("passes when LCM is listed under Helm charts", func() {
		conf := image.Configuration{
			Release: release.Release{
				Components: release.Components{
					HelmCharts: []release.HelmChart{{Name: "elemental-lifecycle-manager"}},
				},
			},
		}
		Expect(validateLifecycleManager(&conf)).To(Succeed())
	})
})
