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
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

type valuesResolverMock struct {
	Err          error
	SuccessCount int // Number of successful calls allowed when Err is set
	currentCalls int
}

func (v *valuesResolverMock) Resolve(*helm.ValueSource) ([]byte, error) {
	v.currentCalls++

	if v.Err != nil && v.currentCalls > v.SuccessCount {
		return nil, v.Err
	}

	return nil, nil
}

var _ = Describe("Helm tests", Label("helm"), func() {
	logger := log.New(log.WithDiscardAll())
	overlaysPath := "/etc/overlays"
	helmPath := "helm"

	Describe("Complete setup", func() {
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
								Values: map[string]any{
									"frrk8s": map[string]any{
										"enabled": true,
									},
								},
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

		It("Fails resolving values of core Helm chart", func() {
			resolver := &valuesResolverMock{Err: fmt.Errorf("resolving failed")}
			definition := &image.Definition{
				Release: release.Release{
					Core: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "metallb",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting core helm charts: resolving values for chart metallb: resolving failed"))
			Expect(charts).To(BeNil())
		})

		It("Fails resolving values of product Helm chart", func() {
			resolver := &valuesResolverMock{Err: fmt.Errorf("resolving failed"), SuccessCount: 1}
			definition := &image.Definition{
				Release: release.Release{
					Core: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "metallb",
							},
						},
					},
					Product: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "neuvector",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting product helm charts: resolving values for chart neuvector-crd: resolving failed"))
			Expect(charts).To(BeNil())
		})

		It("Fails resolving values of user Helm chart", func() {
			resolver := &valuesResolverMock{Err: fmt.Errorf("resolving failed")}
			definition := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "apache",
								RepositoryName:  "apache-repo",
								TargetNamespace: "web",
								Version:         "10.7.0",
								ValuesFile:      "apache-values.yaml",
							},
						},
						Repositories: []*kubernetes.HelmRepository{
							{
								Name: "apache-repo",
								URL:  "https://example.com/apache",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver}

			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting user helm charts: resolving values for chart apache: resolving failed"))
			Expect(charts).To(BeNil())
		})

		It("Fails to collect chart with a missing repository", func() {
			resolver := &valuesResolverMock{}
			definition := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "apache",
								RepositoryName:  "apache-repo",
								TargetNamespace: "web",
								Version:         "10.7.0",
								ValuesFile:      "apache-values.yaml",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver}
			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting user helm charts: repository not found for chart: apache"))
			Expect(charts).To(BeNil())
		})

		It("Fails enabling a missing core chart", func() {
			resolver := &valuesResolverMock{}
			definition := &image.Definition{
				Release: release.Release{
					Core: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "rancher",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: filtering enabled core helm charts: adding helm chart 'rancher': helm chart does not exist"))
			Expect(charts).To(BeNil())
		})

		It("Fails enabling a missing product chart", func() {
			resolver := &valuesResolverMock{}
			definition := &image.Definition{
				Release: release.Release{
					Product: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "rancher",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: filtering enabled product helm charts: adding helm chart 'rancher': helm chart does not exist"))
			Expect(charts).To(BeNil())
		})

		It("Fails writing Helm charts to the FS", func() {
			fs, cleanup, err := sysmock.TestFS(nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			fs, err = sysmock.ReadOnlyTestFS(fs)
			Expect(err).NotTo(HaveOccurred())

			definition := &image.Definition{}

			h := &Helm{
				FS:             fs,
				ValuesResolver: &valuesResolverMock{},
				DestinationDir: overlaysPath,
				RelativePath:   helmPath,
			}

			charts, err := h.Configure(definition, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("writing helm chart resources: creating directory: Mkdir /etc/overlays/helm: operation not permitted"))
			Expect(charts).To(BeNil())
		})

		It("Collects and writes core, product and user Helm charts to the FS", func() {
			fs, cleanup, err := sysmock.TestFS(map[string]string{
				filepath.Join(overlaysPath, helmPath, "apache-values.yaml"):  "image:\n  debug: true\nreplicaCount: 1\n",
				filepath.Join(overlaysPath, helmPath, "metallb-values.yaml"): "controller:\n  logLevel: warn",
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			resolver := &helm.ValuesResolver{
				ValuesDir: filepath.Join(overlaysPath, helmPath),
				FS:        fs,
			}

			definition := &image.Definition{
				Release: release.Release{
					Core: release.Components{
						Helm: []release.HelmChart{{Name: "metallb", ValuesFile: "metallb-values.yaml"}},
					},
					Product: release.Components{
						Helm: []release.HelmChart{{Name: "neuvector"}},
					},
				},
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "apache",
								RepositoryName:  "apache-repo",
								TargetNamespace: "web",
								Version:         "10.7.0",
								ValuesFile:      "apache-values.yaml",
							},
						},
						Repositories: []*kubernetes.HelmRepository{
							{
								Name: "apache-repo",
								URL:  "https://example.com/apache",
							},
						},
					},
				},
			}

			h := &Helm{
				FS:             fs,
				ValuesResolver: resolver,
				DestinationDir: overlaysPath,
				RelativePath:   helmPath,
				Logger:         logger,
			}

			charts, err := h.Configure(definition, rm)
			Expect(err).NotTo(HaveOccurred())
			Expect(charts).To(ConsistOf(
				"/helm/metallb.yaml",
				"/helm/neuvector-crd.yaml",
				"/helm/neuvector.yaml",
				"/helm/apache.yaml"))

			// Verify the contents of the various written Helm resources
			contents := `apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
    name: neuvector
    namespace: kube-system
spec:
    chart: neuvector
    version: 106.0.0+up2.8.5
    repo: https://charts.rancher.io/
    targetNamespace: neuvector-system
    createNamespace: true
    backOffLimit: 20
`
			b, err := fs.ReadFile(filepath.Join(overlaysPath, helmPath, "neuvector.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(contents))

			contents = `apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
    name: neuvector-crd
    namespace: kube-system
spec:
    chart: neuvector-crd
    version: 106.0.0+up2.8.5
    repo: https://charts.rancher.io/
    targetNamespace: neuvector-system
    createNamespace: true
    backOffLimit: 20
`
			b, err = fs.ReadFile(filepath.Join(overlaysPath, helmPath, "neuvector-crd.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(contents))

			contents = `apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
    name: metallb
    namespace: kube-system
spec:
    chart: metallb
    version: 302.0.0+up0.14.9
    repo: https://example.com/suse-core
    valuesContent: |
        controller:
            logLevel: warn
        frrk8s:
            enabled: true
    targetNamespace: metallb-system
    createNamespace: true
    backOffLimit: 20
`
			b, err = fs.ReadFile(filepath.Join(overlaysPath, helmPath, "metallb.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(contents))

			contents = `apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
    name: apache
    namespace: kube-system
spec:
    chart: apache
    version: 10.7.0
    repo: https://example.com/apache
    valuesContent: |
        image:
            debug: true
        replicaCount: 1
    targetNamespace: web
    createNamespace: true
    backOffLimit: 20
`
			b, err = fs.ReadFile(filepath.Join(overlaysPath, helmPath, "apache.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(contents))
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
			e, err := enabledHelmCharts(h, &release.Components{Helm: []release.HelmChart{{Name: "neuvector"}}})
			Expect(err).NotTo(HaveOccurred())
			Expect(e).To(HaveLen(2))

			chart := e[0].(*api.HelmChart)
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))

			chart = e[1].(*api.HelmChart)
			Expect(chart.Name).To(Equal("NeuVector"))
			Expect(chart.Chart).To(Equal("neuvector"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(ConsistOf("neuvector-crd"))
		})

		It("Successfully filters enabled Helm chart", func() {
			e, err := enabledHelmCharts(h, &release.Components{Helm: []release.HelmChart{{Name: "neuvector-crd"}}})
			Expect(err).NotTo(HaveOccurred())
			Expect(e).To(HaveLen(1))

			chart := e[0].(*api.HelmChart)
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(BeEmpty())
		})

		It("Fails to find non-existing enabled Helm chart", func() {
			e, err := enabledHelmCharts(h, &release.Components{Helm: []release.HelmChart{{Name: "rancher"}}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("adding helm chart 'rancher': helm chart does not exist"))
			Expect(e).To(BeNil())
		})

		It("Fails to find non-existing dependency Helm chart", func() {
			e, err := enabledHelmCharts(h, &release.Components{Helm: []release.HelmChart{{Name: "longhorn"}}})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("adding helm chart 'longhorn': adding dependent helm chart 'longhorn-crd': helm chart does not exist"))
			Expect(e).To(BeNil())
		})
	})
})
