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
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
)

var _ = Describe("Helm tests", Label("helm"), func() {
	Describe("Prioritising", func() {
		rm := &resolver.ResolvedManifest{
			CorePlatform: &core.ReleaseManifest{
				Components: core.Components{
					Helm: &api.Helm{
						Charts: []*api.HelmChart{
							{
								Name:       "MetalLB",
								Chart:      "metallb",
								Version:    "302.0.0+up0.14.9",
								Namespace:  "metallb-system",
								Repository: "suse-core",
							},
						},
						Repositories: []api.Repository{
							{
								Name: "suse-core",
								URL:  "https://example.com/suse-core",
							},
						},
					},
				},
			},
			ProductExtension: &product.ReleaseManifest{
				Components: product.Components{
					Helm: &api.Helm{
						Charts: []*api.HelmChart{
							{
								Name:       "NeuVector",
								Chart:      "neuvector",
								Version:    "106.0.0+up2.8.5",
								Namespace:  "neuvector-system",
								Repository: "rancher-charts",
								DependsOn:  []string{"neuvector-crd"},
							},
							{
								Name:       "NeuVector CRD",
								Chart:      "neuvector-crd",
								Version:    "106.0.0+up2.8.5",
								Namespace:  "neuvector-system",
								Repository: "rancher-charts",
							},
						},
						Repositories: []api.Repository{
							{
								Name: "rancher-charts",
								URL:  "https://charts.rancher.io/",
							},
						},
					},
				},
			},
		}

		It("Successfully returns prioritised Helm charts, with enabled charts", func() {
			definition := &image.Definition{
				Release: image.Release{
					Enable: []string{"neuvector"},
				},
				Kubernetes: image.Kubernetes{
					Helm: &api.Helm{
						Charts:       []*api.HelmChart{{Name: "Foo", Chart: "foo", Repository: "bar"}},
						Repositories: []api.Repository{{Name: "bar", URL: "https://example.com/bar"}},
					},
				},
			}

			configs, err := getPrioritisedHelmConfigs(definition, rm)
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(3))

			Expect(configs[0].Charts).To(HaveLen(1))
			Expect(configs[0].Repositories).To(HaveLen(1))

			chart := configs[0].Charts[0]
			Expect(chart.Name).To(Equal("MetalLB"))
			Expect(chart.Chart).To(Equal("metallb"))
			Expect(chart.Version).To(Equal("302.0.0+up0.14.9"))
			Expect(chart.Namespace).To(Equal("metallb-system"))
			Expect(chart.Repository).To(Equal("suse-core"))
			Expect(chart.DependsOn).To(BeEmpty())

			repository := configs[0].Repositories[0]
			Expect(repository.Name).To(Equal("suse-core"))
			Expect(repository.URL).To(Equal("https://example.com/suse-core"))

			Expect(configs[1].Charts).To(HaveLen(2))
			Expect(configs[1].Repositories).To(HaveLen(1))

			chart = configs[1].Charts[0]
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(BeEmpty())

			chart = configs[1].Charts[1]

			Expect(chart.Name).To(Equal("NeuVector"))
			Expect(chart.Chart).To(Equal("neuvector"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(ConsistOf("neuvector-crd"))

			repository = configs[1].Repositories[0]
			Expect(repository.Name).To(Equal("rancher-charts"))
			Expect(repository.URL).To(Equal("https://charts.rancher.io/"))

			Expect(configs[2].Charts).To(HaveLen(1))
			Expect(configs[2].Repositories).To(HaveLen(1))

			chart = configs[2].Charts[0]
			Expect(chart.Name).To(Equal("Foo"))
			Expect(chart.Chart).To(Equal("foo"))
			Expect(chart.Repository).To(Equal("bar"))

			repository = configs[2].Repositories[0]
			Expect(repository.Name).To(Equal("bar"))
			Expect(repository.URL).To(Equal("https://example.com/bar"))
		})

		It("Successfully returns prioritised Helm charts, without enabled charts", func() {
			definition := &image.Definition{
				Release: image.Release{
					Enable: []string{},
				},
				Kubernetes: image.Kubernetes{
					Helm: &api.Helm{
						Charts:       []*api.HelmChart{{Name: "Foo", Chart: "foo", Repository: "bar"}},
						Repositories: []api.Repository{{Name: "bar", URL: "https://example.com/bar"}},
					},
				},
			}

			configs, err := getPrioritisedHelmConfigs(definition, rm)
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(2))

			Expect(configs[0].Charts).To(HaveLen(1))
			Expect(configs[0].Repositories).To(HaveLen(1))

			chart := configs[0].Charts[0]
			Expect(chart.Name).To(Equal("MetalLB"))
			Expect(chart.Chart).To(Equal("metallb"))
			Expect(chart.Version).To(Equal("302.0.0+up0.14.9"))
			Expect(chart.Namespace).To(Equal("metallb-system"))
			Expect(chart.Repository).To(Equal("suse-core"))
			Expect(chart.DependsOn).To(BeEmpty())

			repository := configs[0].Repositories[0]
			Expect(repository.Name).To(Equal("suse-core"))
			Expect(repository.URL).To(Equal("https://example.com/suse-core"))

			Expect(configs[1].Charts).To(HaveLen(1))
			Expect(configs[1].Repositories).To(HaveLen(1))

			chart = configs[1].Charts[0]
			Expect(chart.Name).To(Equal("Foo"))
			Expect(chart.Chart).To(Equal("foo"))
			Expect(chart.Repository).To(Equal("bar"))

			repository = configs[1].Repositories[0]
			Expect(repository.Name).To(Equal("bar"))
			Expect(repository.URL).To(Equal("https://example.com/bar"))
		})

		It("Fails to prioritise Helm charts due to attempting to enable missing one", func() {
			definition := &image.Definition{
				Release: image.Release{
					Enable: []string{"rancher"},
				},
			}

			configs, err := getPrioritisedHelmConfigs(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("filtering enabled helm charts: adding helm chart 'rancher': helm chart does not exist"))
			Expect(configs).To(BeNil())
		})
	})

	Describe("Filtering", func() {
		h := &api.Helm{
			Charts: []*api.HelmChart{
				{
					Name:       "Longhorn",
					Chart:      "longhorn",
					Version:    "106.2.0+up1.8.1",
					Namespace:  "longhorn",
					Repository: "rancher-charts",
					// Dependency intentionally missing from the charts list
					DependsOn: []string{"longhorn-crd"},
				},
				{
					Name:       "NeuVector",
					Chart:      "neuvector",
					Version:    "106.0.0+up2.8.5",
					Namespace:  "neuvector-system",
					Repository: "rancher-charts",
					DependsOn:  []string{"neuvector-crd"},
				},
				{
					Name:       "NeuVector CRD",
					Chart:      "neuvector-crd",
					Version:    "106.0.0+up2.8.5",
					Namespace:  "neuvector-system",
					Repository: "rancher-charts",
				},
			},
			Repositories: []api.Repository{
				{
					Name: "rancher-charts",
					URL:  "https://charts.rancher.io/",
				},
			},
		}

		It("Successfully filters enabled Helm charts with dependency", func() {
			e, err := enabledHelmCharts(h, []string{"neuvector"})
			Expect(err).NotTo(HaveOccurred())
			Expect(e.Charts).To(HaveLen(2))
			Expect(e.Repositories).To(HaveLen(1))

			chart := e.Charts[0]
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))

			chart = e.Charts[1]
			Expect(chart.Name).To(Equal("NeuVector"))
			Expect(chart.Chart).To(Equal("neuvector"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(ConsistOf("neuvector-crd"))

			repository := e.Repositories[0]
			Expect(repository.Name).To(Equal("rancher-charts"))
			Expect(repository.URL).To(Equal("https://charts.rancher.io/"))
		})

		It("Successfully filters enabled Helm chart", func() {
			e, err := enabledHelmCharts(h, []string{"neuvector-crd"})
			Expect(err).NotTo(HaveOccurred())
			Expect(e.Charts).To(HaveLen(1))
			Expect(e.Repositories).To(HaveLen(1))

			chart := e.Charts[0]
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(BeEmpty())

			repository := e.Repositories[0]
			Expect(repository.Name).To(Equal("rancher-charts"))
			Expect(repository.URL).To(Equal("https://charts.rancher.io/"))
		})

		It("Fails to find non-existing enabled Helm chart", func() {
			e, err := enabledHelmCharts(h, []string{"rancher"})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("adding helm chart 'rancher': helm chart does not exist"))
			Expect(e).To(BeNil())
		})

		It("Fails to find non-existing dependency Helm chart", func() {
			e, err := enabledHelmCharts(h, []string{"longhorn"})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("adding helm chart 'longhorn': adding dependent helm chart 'longhorn-crd': helm chart does not exist"))
			Expect(e).To(BeNil())
		})
	})
})
