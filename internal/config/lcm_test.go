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

package config

import (
	"fmt"

	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/solution"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"go.yaml.in/yaml/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/helm"
)

type lcmResolver struct {
	valuesFileContent string
}

func NewLcmResolver() *lcmResolver {
	return &lcmResolver{}
}

func (l *lcmResolver) AddValuesFile(content string) {
	if l != nil {
		l.valuesFileContent = content
	}
}

func (l *lcmResolver) Resolve(_ *helm.ValueSource) ([]byte, error) {
	var yamlContent map[string]any

	err := yaml.Unmarshal([]byte(l.valuesFileContent), &yamlContent)
	Expect(err).ToNot(HaveOccurred())

	v, err := yaml.Marshal(yamlContent)
	if err != nil {
		return nil, fmt.Errorf("marshaling values: %w", err)
	}

	return v, nil
}

var _ = Describe("Evaluate LCM dependencies", func() {
	var rm *resolver.ResolvedManifest
	var enabled []release.HelmChart
	var crdDep, sucDep, cmDep api.HelmChartDependency
	var valuesFileMap map[string]string
	var lResolver *lcmResolver

	BeforeEach(func() {
		crdDep = api.HelmChartDependency{Name: "elemental-lifecycle-manager-crds", Type: "helm"}
		sucDep = api.HelmChartDependency{Name: "system-upgrade-controller", Type: "helm"}
		cmDep = api.HelmChartDependency{Name: "cert-manager", Type: "helm"}
		rm = &resolver.ResolvedManifest{
			CorePlatform: &core.ReleaseManifest{
				Components: core.Components{
					Helm: &api.Helm{
						Charts: []*api.HelmChart{
							{
								Name:       "Elemental Lifecycle Manager",
								Chart:      "elemental-lifecycle-manager",
								Version:    "0.1.1",
								Namespace:  "elemental-system",
								Repository: "elemental-charts",
								DependsOn:  []api.HelmChartDependency{crdDep, sucDep, cmDep},
							},
							{
								Name:       "Elemental Lifecycle Manager CRDs",
								Chart:      "elemental-lifecycle-manager-crds",
								Version:    "0.1.1",
								Namespace:  "elemental-system",
								Repository: "elemental-charts",
							},
							{
								Name:       "System Upgrade Controller",
								Chart:      "system-upgrade-controller",
								Version:    "109.0.2",
								Namespace:  "cattle-system",
								Repository: "rancher-charts",
							},
							{
								Name:       "Cert Manager",
								Chart:      "cert-manager",
								Version:    "v1.20.3",
								Namespace:  "cert-manager",
								Repository: "jetstack",
							},
						},
						Repositories: []*api.HelmRepository{
							{
								Name: "elemental-charts",
								URL:  "oci://registry.suse.com/elemental/charts",
							},
							{
								Name: "rancher-charts",
								URL:  "https://charts.rancher.io/",
							},
							{
								Name: "jetstack",
								URL:  "https://charts.jetstack.io",
							},
						},
					},
				},
			},
			SolutionExtension: &solution.ReleaseManifest{
				Components: solution.Components{
					Helm: &api.Helm{
						Charts: []*api.HelmChart{
							{
								Name:       "Rancher",
								Chart:      "rancher",
								Version:    "2.14.0",
								Namespace:  "cattle-system",
								Repository: "rancher-charts",
								DependsOn:  []api.HelmChartDependency{{Name: "cert-manager", Type: "helm"}},
							},
						},
						Repositories: []*api.HelmRepository{
							{
								Name: "rancher-charts",
								URL:  "https://charts.rancher.io/",
							},
						},
					},
				},
			},
		}
		valuesFileMap = map[string]string{"elemental-lifecycle-manager": "values.yaml"}
		lResolver = NewLcmResolver()
	})

	When("rancher is not enabled", func() {
		When("values file is not provided", func() {
			It("should include cert-manager and system-upgrade-controller dependencies", func() {
				enabled = []release.HelmChart{{Name: "elemental-lifecycle-manager"}}

				err := evaluateLCMDeps(enabled, rm.CorePlatform, valuesFileMap, lResolver)
				Expect(err).ToNot(HaveOccurred())

				chart := rm.CorePlatform.Components.Helm.Charts[0]
				Expect(chart.GetName()).To(Equal("elemental-lifecycle-manager"))
				Expect(len(chart.DependsOn)).To(Equal(3))
				Expect(chart.DependsOn).To(ContainElements(sucDep, crdDep, cmDep))
			})
		})

		When("values file is provided without values required for custom certificate", func() {
			It("should include cert-manager in final dependencies", func() {
				By("having no value related to cert-manager")

				enabled = []release.HelmChart{{Name: "elemental-lifecycle-manager"}}
				var valuesFile = "replicaCount: 2"
				lResolver.AddValuesFile(valuesFile)

				err := evaluateLCMDeps(enabled, rm.CorePlatform, valuesFileMap, lResolver)
				Expect(err).ToNot(HaveOccurred())

				chart := rm.CorePlatform.Components.Helm.Charts[0]
				Expect(chart.GetName()).To(Equal("elemental-lifecycle-manager"))
				Expect(len(chart.DependsOn)).To(Equal(3))
				Expect(chart.DependsOn).To(ContainElements(sucDep, crdDep, cmDep))

				By("having incomplete values for cert-manager")
				valuesFile = `
webhook:
  cert:
    createDefault: false
`
				lResolver.AddValuesFile(valuesFile)
				err = evaluateLCMDeps(enabled, rm.CorePlatform, valuesFileMap, lResolver)
				Expect(err).ToNot(HaveOccurred())

				chart = rm.CorePlatform.Components.Helm.Charts[0]
				Expect(chart.GetName()).To(Equal("elemental-lifecycle-manager"))
				Expect(len(chart.DependsOn)).To(Equal(3))
				Expect(chart.DependsOn).To(ContainElements(sucDep, crdDep, cmDep))
			})
		})

		When("values file provided with values for custom certificate", func() {
			It("should not include cert-manager in dependencies", func() {
				enabled = []release.HelmChart{{Name: "elemental-lifecycle-manager"}}
				var valuesFile = `
webhook:
  cert:
    createDefault: false
    existingSecret: custom-secret
    caBundle: ""
`
				lResolver.AddValuesFile(valuesFile)

				err := evaluateLCMDeps(enabled, rm.CorePlatform, valuesFileMap, lResolver)
				Expect(err).ToNot(HaveOccurred())

				chart := rm.CorePlatform.Components.Helm.Charts[0]
				Expect(chart.GetName()).To(Equal("elemental-lifecycle-manager"))
				Expect(len(chart.DependsOn)).To(Equal(2))
				Expect(chart.DependsOn).To(ContainElements(sucDep, crdDep))
				Expect(chart.DependsOn).ToNot(ContainElement(cmDep))
			})
		})
	})

	When("rancher is enabled", func() {
		When("values file is not provided", func() {
			It("should not include SUC, but still include cert-manager dependency", func() {
				enabled = []release.HelmChart{
					{Name: "elemental-lifecycle-manager"},
					{Name: "rancher"},
				}
				err := evaluateLCMDeps(enabled, rm.CorePlatform, valuesFileMap, lResolver)
				Expect(err).ToNot(HaveOccurred())

				chart := rm.CorePlatform.Components.Helm.Charts[0]
				Expect(chart.GetName()).To(Equal("elemental-lifecycle-manager"))
				Expect(len(chart.DependsOn)).To(Equal(2))
				Expect(chart.DependsOn).To(ContainElements(crdDep, cmDep))
				Expect(chart.DependsOn).ToNot(ContainElement(sucDep))
			})
		})

		When("values file with certificate configurations is provided", func() {
			It("should neither include SUC nor cert-manager as LCM dependency", func() {
				enabled = []release.HelmChart{
					{Name: "elemental-lifecycle-manager"},
					{Name: "rancher"},
				}
				var valuesFile = `
webhook:
  cert:
    createDefault: false
    existingSecret: custom-secret
    caBundle: ""
`
				lResolver.AddValuesFile(valuesFile)
				err := evaluateLCMDeps(enabled, rm.CorePlatform, valuesFileMap, lResolver)
				Expect(err).ToNot(HaveOccurred())

				chart := rm.CorePlatform.Components.Helm.Charts[0]
				Expect(chart.GetName()).To(Equal("elemental-lifecycle-manager"))
				Expect(len(chart.DependsOn)).To(Equal(1))
				Expect(chart.DependsOn).To(ContainElement(crdDep))
				Expect(chart.DependsOn).ToNot(ContainElements(sucDep, cmDep))
			})
		})
	})
})
