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

package build

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
)

var _ = Describe("Systemd extensions", func() {
	logger := log.New(log.WithDiscardAll())

	It("Detects remote sources", func() {
		Expect(isRemoteURL("http://example.com/extension.raw")).To(BeTrue(), "http")
		Expect(isRemoteURL("https://example.com/extension.raw")).To(BeTrue(), "https")
		Expect(isRemoteURL("registry.example.com/extension:0.0.1")).To(BeFalse(), "oci")
		Expect(isRemoteURL("raw:///etc/extension.raw")).To(BeFalse(), "custom")
	})

	Describe("Filtering", func() {
		It("Fails to list enabled Helm charts", func() {
			rm := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Helm: &api.Helm{
							Charts: []*api.HelmChart{
								{
									Chart: "longhorn",
									DependsOn: []api.HelmChartDependency{
										{
											Name: "longhorn-crd",
											Type: "helm",
										},
									},
								},
							},
						},
					},
				},
			}

			def := &image.Definition{
				Release: release.Release{
					Components: release.Components{
						HelmCharts: []release.HelmChart{
							{
								Name: "longhorn",
							},
						},
					},
				},
			}

			extensions, err := enabledExtensions(rm, def, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(MatchRegexp("filtering enabled helm charts: adding helm chart 'longhorn': " +
				"adding dependent helm chart 'longhorn-crd': helm chart does not exist")))
			Expect(extensions).To(BeEmpty())
		})

		It("Successfully filters enabled systemd extensions from all sources", func() {
			rm := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Systemd: api.Systemd{
							Extensions: []api.SystemdExtension{
								{
									Name:     "elemental3ctl",
									Image:    "https://example.com/elemental3ctl.raw",
									Required: true,
								},
								{
									Name:  "rke2",
									Image: "https://example.com/rke2.raw",
								},
							},
						},
					},
				},
				ProductExtension: &product.ReleaseManifest{
					Components: product.Components{
						Systemd: api.Systemd{
							Extensions: []api.SystemdExtension{
								{
									Name:  "longhorn",
									Image: "https://example.com/longhorn.raw",
								},
								{
									Name:  "nvidia-toolkit",
									Image: "https://example.com/nvidia-toolkit.raw",
								},
								{
									Name:  "debugging-toolkit", // Not enabled in the release below
									Image: "https://example.com/debugging-toolkit.raw",
								},
							},
						},
						Helm: &api.Helm{
							Charts: []*api.HelmChart{
								{
									Chart: "longhorn-crd",
								},
								{
									Chart: "longhorn",
									DependsOn: []api.HelmChartDependency{
										{
											Name: "longhorn-crd",
											Type: "helm",
										},
										{
											Name: "longhorn",
											Type: "sysext",
										},
									},
								},
							},
						},
					},
				},
			}

			def := &image.Definition{
				Release: release.Release{
					Components: release.Components{
						SystemdExtensions: []release.SystemdExtension{
							{
								Name: "nvidia-toolkit",
							},
						},
						HelmCharts: []release.HelmChart{
							{
								Name: "longhorn",
							},
						},
					},
				},
			}

			extensions, err := enabledExtensions(rm, def, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(extensions).To(HaveLen(4))

			Expect(extensions).To(ContainElement(api.SystemdExtension{Name: "elemental3ctl", Image: "https://example.com/elemental3ctl.raw", Required: true}), "Required by release")
			Expect(extensions).To(ContainElement(api.SystemdExtension{Name: "rke2", Image: "https://example.com/rke2.raw"}), "Required as per Helm chart enablement")
			Expect(extensions).To(ContainElement(api.SystemdExtension{Name: "longhorn", Image: "https://example.com/longhorn.raw"}), "Required as a dependency of enabled Helm chart")
			Expect(extensions).To(ContainElement(api.SystemdExtension{Name: "nvidia-toolkit", Image: "https://example.com/nvidia-toolkit.raw"}), "Explicitly requested")
		})
	})
})
