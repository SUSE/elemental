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
							{
								Name:       "Endpoint Copier Operator",
								Chart:      "endpoint-copier-operator",
								Version:    "0.3.0",
								Namespace:  "endpoint-copier-operator",
								Repository: "suse-core-oci",
							},
						},
						Repositories: []*api.HelmRepository{
							{
								Name: "suse-core",
								URL:  "https://example.com/suse-core",
							},
							{
								Name: "suse-core-oci",
								URL:  "oci://example-1.com/charts",
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
								DependsOn:  []api.HelmChartDependency{{Name: "neuvector-crd", Type: "helm"}},
							},
							{
								Name:       "NeuVector CRD",
								Chart:      "neuvector-crd",
								Version:    "106.0.0+up2.8.5",
								Namespace:  "neuvector-system",
								Repository: "rancher-charts",
							},
							{
								Name:       "KubeVirt",
								Chart:      "kubevirt",
								Version:    "0.6.0",
								Namespace:  "kubevirt-system",
								Repository: "kubevirt",
							},
						},
						Repositories: []*api.HelmRepository{
							{
								Name: "rancher-charts",
								URL:  "https://charts.rancher.io/",
							},
							{
								Name: "kubevirt",
								URL:  "oci://example-1.com/kv/charts",
							},
						},
					},
				},
			},
		}

		It("Fails resolving values of core Helm chart", func() {
			resolver := &valuesResolverMock{Err: fmt.Errorf("resolving failed")}
			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						HelmCharts: []release.HelmChart{
							{
								Name: "metallb", // chart in core release
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(conf, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting helm charts: resolving values for chart metallb: resolving failed"))
			Expect(charts).To(BeNil())
		})

		It("Fails resolving values of product Helm chart", func() {
			resolver := &valuesResolverMock{Err: fmt.Errorf("resolving failed")}
			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						HelmCharts: []release.HelmChart{
							{
								Name: "neuvector", // chart in product release
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(conf, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting helm charts: resolving values for chart neuvector-crd: resolving failed"))
			Expect(charts).To(BeNil())
		})

		It("Fails resolving values of user Helm chart", func() {
			resolver := &valuesResolverMock{Err: fmt.Errorf("resolving failed")}
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "apache",
								RepositoryName:  "apache",
								TargetNamespace: "web",
								Version:         "10.7.0",
								ValuesFile:      "apache-values.yaml",
							},
						},
						Repositories: []*kubernetes.HelmRepository{
							{
								Name: "apache",
								URL:  "https://example.com/apache",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver}

			charts, err := h.Configure(conf, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting user helm charts: resolving values for chart apache: resolving failed"))
			Expect(charts).To(BeNil())
		})

		It("Fails to collect chart with a missing repository", func() {
			resolver := &valuesResolverMock{}
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "apache",
								RepositoryName:  "apache",
								TargetNamespace: "web",
								Version:         "10.7.0",
								ValuesFile:      "apache-values.yaml",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver}
			charts, err := h.Configure(conf, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: collecting user helm charts: repository not found for chart: apache"))
			Expect(charts).To(BeNil())
		})

		It("Fails enabling a missing release chart", func() {
			resolver := &valuesResolverMock{}
			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						HelmCharts: []release.HelmChart{
							{
								Name: "rancher",
							},
						},
					},
				},
			}

			h := &Helm{ValuesResolver: resolver, Logger: logger}

			charts, err := h.Configure(conf, rm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("retrieving helm charts: filtering enabled helm charts: adding helm chart 'rancher': helm chart does not exist"))
			Expect(charts).To(BeNil())
		})

		It("Fails writing Helm charts to the FS", func() {
			fs, cleanup, err := sysmock.TestFS(nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			fs, err = sysmock.ReadOnlyTestFS(fs)
			Expect(err).NotTo(HaveOccurred())

			conf := &image.Configuration{}

			h := &Helm{
				FS:             fs,
				ValuesResolver: &valuesResolverMock{},
				DestinationDir: overlaysPath,
				RelativePath:   helmPath,
			}

			charts, err := h.Configure(conf, rm)
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

			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						HelmCharts: []release.HelmChart{
							{Name: "metallb", ValuesFile: "metallb-values.yaml"},
							{Name: "endpoint-copier-operator"},
							{Name: "neuvector"},
							{Name: "kubevirt"},
						},
					},
				},
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "apache",
								RepositoryName:  "apache",
								TargetNamespace: "web",
								Version:         "10.7.0",
								ValuesFile:      "apache-values.yaml",
							},
							{
								Name:            "nginx",
								RepositoryName:  "nginx",
								TargetNamespace: "web",
								Version:         "1.29.3",
							},
						},
						Repositories: []*kubernetes.HelmRepository{
							{
								Name: "apache",
								URL:  "https://example.com/apache",
							},
							{
								Name: "nginx",
								URL:  "oci://example.com/web",
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

			charts, err := h.Configure(conf, rm)
			Expect(err).NotTo(HaveOccurred())
			Expect(charts).To(ConsistOf(
				"/helm/metallb.yaml",
				"/helm/endpoint-copier-operator.yaml",
				"/helm/neuvector-crd.yaml",
				"/helm/neuvector.yaml",
				"/helm/kubevirt.yaml",
				"/helm/apache.yaml",
				"/helm/nginx.yaml"))

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
    name: endpoint-copier-operator
    namespace: kube-system
spec:
    chart: oci://example-1.com/charts/endpoint-copier-operator
    version: 0.3.0
    targetNamespace: endpoint-copier-operator
    createNamespace: true
    backOffLimit: 20
`
			b, err = fs.ReadFile(filepath.Join(overlaysPath, helmPath, "endpoint-copier-operator.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(contents))

			contents = `apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
    name: kubevirt
    namespace: kube-system
spec:
    chart: oci://example-1.com/kv/charts/kubevirt
    version: 0.6.0
    targetNamespace: kubevirt-system
    createNamespace: true
    backOffLimit: 20
`
			b, err = fs.ReadFile(filepath.Join(overlaysPath, helmPath, "kubevirt.yaml"))
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

			contents = `apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
    name: nginx
    namespace: kube-system
spec:
    chart: oci://example.com/web/nginx
    version: 1.29.3
    targetNamespace: web
    createNamespace: true
    backOffLimit: 20
`
			b, err = fs.ReadFile(filepath.Join(overlaysPath, helmPath, "nginx.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(contents))
		})
	})

	Describe("Filtering", func() {
		rm := &resolver.ResolvedManifest{
			CorePlatform: &core.ReleaseManifest{
				Components: core.Components{
					Helm: &api.Helm{
						Charts: []*api.HelmChart{
							{
								Name:       "Longhorn",
								Chart:      "longhorn",
								Version:    "106.2.0+up1.8.1",
								Namespace:  "longhorn",
								Repository: "suse-core",
								// Dependency intentionally missing from the charts list
								DependsOn: []api.HelmChartDependency{{Name: "longhorn-crd", Type: "helm"}},
							},
						},
						Repositories: []*api.HelmRepository{
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
								DependsOn:  []api.HelmChartDependency{{Name: "neuvector-crd", Type: "helm"}},
							},
							{
								Name:       "NeuVector CRD",
								Chart:      "neuvector-crd",
								Version:    "106.0.0+up2.8.5",
								Namespace:  "neuvector-system",
								Repository: "rancher-charts",
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

		It("Successfully filters enabled Helm charts with dependency", func() {
			charts, repositories, err := enabledHelmCharts(rm, []release.HelmChart{{Name: "neuvector"}}, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(charts).To(HaveLen(2))
			Expect(repositories).To(HaveLen(2))

			chart := charts[0]
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))

			chart = charts[1]
			Expect(chart.Name).To(Equal("NeuVector"))
			Expect(chart.Chart).To(Equal("neuvector"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))

			Expect(chart.DependsOn).To(HaveLen(1))
			Expect(chart.DependsOn[0].Name).To(Equal("neuvector-crd"))
			Expect(chart.DependsOn[0].Type).To(BeEquivalentTo("helm"))

			Expect(repositories["suse-core"]).To(Equal("https://example.com/suse-core"))
			Expect(repositories["rancher-charts"]).To(Equal("https://charts.rancher.io/"))
		})

		It("Successfully filters enabled Helm chart", func() {
			charts, repositories, err := enabledHelmCharts(rm, []release.HelmChart{{Name: "neuvector-crd"}}, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(charts).To(HaveLen(1))
			Expect(repositories).To(HaveLen(2))

			chart := charts[0]
			Expect(chart.Name).To(Equal("NeuVector CRD"))
			Expect(chart.Chart).To(Equal("neuvector-crd"))
			Expect(chart.Version).To(Equal("106.0.0+up2.8.5"))
			Expect(chart.Namespace).To(Equal("neuvector-system"))
			Expect(chart.Repository).To(Equal("rancher-charts"))
			Expect(chart.DependsOn).To(BeEmpty())

			Expect(repositories["suse-core"]).To(Equal("https://example.com/suse-core"))
			Expect(repositories["rancher-charts"]).To(Equal("https://charts.rancher.io/"))
		})

		It("Fails to find non-existing enabled Helm chart", func() {
			charts, repositories, err := enabledHelmCharts(rm, []release.HelmChart{{Name: "rancher"}}, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("adding helm chart 'rancher': helm chart does not exist"))
			Expect(charts).To(BeNil())
			Expect(repositories).To(BeNil())
		})

		It("Fails to find non-existing dependency Helm chart", func() {
			charts, repositories, err := enabledHelmCharts(rm, []release.HelmChart{{Name: "longhorn"}}, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("adding helm chart 'longhorn': adding dependent helm chart 'longhorn-crd': helm chart does not exist"))
			Expect(charts).To(BeNil())
			Expect(repositories).To(BeNil())
		})
	})
})
