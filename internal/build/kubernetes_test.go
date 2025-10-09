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
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type helmConfiguratorMock struct {
	configureFunc func(*image.Definition, *resolver.ResolvedManifest) ([]string, error)
}

func (h *helmConfiguratorMock) Configure(def *image.Definition, manifest *resolver.ResolvedManifest) ([]string, error) {
	if h.configureFunc != nil {
		return h.configureFunc(def, manifest)
	}

	panic("not implemented")
}

var _ = Describe("Kubernetes", func() {
	Describe("Resources trigger", func() {
		It("Skips manifests setup if manifests are not provided", func() {
			def := &image.Definition{}
			Expect(needsManifestsSetup(def)).To(BeFalse())
		})

		It("Requires manifests setup if local manifests are provided", func() {
			def := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					LocalManifests: []string{"/apache.yaml"},
				},
			}
			Expect(needsManifestsSetup(def)).To(BeTrue())
		})

		It("Requires manifests setup if remote manifests are provided", func() {
			def := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					RemoteManifests: []string{"https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml"},
				},
			}
			Expect(needsManifestsSetup(def)).To(BeTrue())
		})

		It("Skips Helm setup if charts are not provided", func() {
			def := &image.Definition{}
			Expect(needsHelmChartsSetup(def)).To(BeFalse())
		})

		It("Requires Helm setup if user charts are provided", func() {
			def := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{Name: "apache", RepositoryName: "apache-repo"},
						},
					},
				},
			}
			Expect(needsHelmChartsSetup(def)).To(BeTrue())
		})

		It("Requires Helm setup if core charts are provided", func() {
			def := &image.Definition{
				Release: release.Release{
					Components: release.Components{
						HelmCharts: []release.HelmChart{
							{
								Name: "metallb",
							},
						},
					},
				},
			}

			Expect(needsHelmChartsSetup(def)).To(BeTrue())
		})

		It("Requires Helm setup if product charts are provided", func() {
			def := &image.Definition{
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

			Expect(needsHelmChartsSetup(def)).To(BeTrue())
		})
	})

	Describe("Configuration", func() {
		const buildDir image.BuildDir = "/_build"

		var system *sys.System
		var fs vfs.FS
		var cleanup func()
		var err error

		BeforeEach(func() {
			fs, cleanup, err = sysmock.TestFS(nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(vfs.MkdirAll(fs, string(buildDir), vfs.DirPerm)).To(Succeed())

			system, err = sys.NewSystem(
				sys.WithLogger(log.New(log.WithDiscardAll())),
				sys.WithFS(fs),
			)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			cleanup()
		})

		It("Fails to configure Helm charts", func() {
			builder := &Builder{
				System: system,
				Helm: &helmConfiguratorMock{
					configureFunc: func(definition *image.Definition, manifest *resolver.ResolvedManifest) ([]string, error) {
						return nil, fmt.Errorf("helm error")
					},
				},
				DownloadFile: func(ctx context.Context, fs vfs.FS, url, path string) error {
					return nil
				},
			}

			manifest := &resolver.ResolvedManifest{}
			def := &image.Definition{
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

			script, confScript, err := builder.configureKubernetes(context.Background(), def, manifest, buildDir)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("configuring helm charts: helm error"))
			Expect(script).To(BeEmpty())
			Expect(confScript).To(BeEmpty())
		})

		It("Succeeds to configure RKE2 with additional resources", func() {
			builder := &Builder{
				System: system,
				Helm: &helmConfiguratorMock{
					configureFunc: func(definition *image.Definition, manifest *resolver.ResolvedManifest) ([]string, error) {
						return []string{"rancher.yaml"}, nil
					},
				},
				DownloadFile: func(ctx context.Context, fs vfs.FS, url, path string) error {
					return nil
				},
			}

			manifest := &resolver.ResolvedManifest{}
			def := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					RemoteManifests: []string{"some-url"},
					Nodes: kubernetes.Nodes{
						{Hostname: "node1", Type: "server"},
					},
				},
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

			script, confScript, err := builder.configureKubernetes(context.Background(), def, manifest, buildDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(script).To(Equal("/var/lib/elemental/kubernetes/k8s_res_deploy.sh"))

			// Verify deployment script contents
			b, err := fs.ReadFile(filepath.Join(buildDir.OverlaysDir(), script))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(ContainSubstring("deployHelmCharts"))
			Expect(string(b)).To(ContainSubstring("rancher.yaml"))
			Expect(string(b)).To(ContainSubstring("deployManifests"))

			_, err = fs.ReadFile(filepath.Join(buildDir.OverlaysDir(), confScript))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Succeeds to configure RKE2 without additional resources", func() {
			builder := &Builder{
				System: system,
				DownloadFile: func(ctx context.Context, fs vfs.FS, url, path string) error {
					return nil
				},
			}

			manifest := &resolver.ResolvedManifest{}
			def := &image.Definition{
				Release: release.Release{
					Components: release.Components{
						SystemdExtensions: []release.SystemdExtension{
							{
								Name: "rke2",
							},
						},
					},
				},
			}

			script, confScript, err := builder.configureKubernetes(context.Background(), def, manifest, buildDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(script).To(BeEmpty())
			Expect(confScript).ToNot(BeEmpty())
		})
	})
})
