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

package config

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/product"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type resolverMock struct {
	resolveFunc func(uri string) (*resolver.ResolvedManifest, error)
}

func (r *resolverMock) Resolve(uri string) (*resolver.ResolvedManifest, error) {
	if r.resolveFunc != nil {
		return r.resolveFunc(uri)
	}

	panic("not implemented")
}

var _ = Describe("Manager", func() {
	var outputDir OutputDir = "/_out"
	var configDir Dir = "/config"
	var fs vfs.FS
	var cleanup func()
	var err error
	var system *sys.System
	var defaultHelmFunc func(c *image.Configuration, rm *resolver.ResolvedManifest) ([]string, error)
	var defaultResolveFunc func(uri string) (*resolver.ResolvedManifest, error)
	var butaneConfigString = `
version: 1.6.0
variant: fcos
passwd:
  usrs:
  - name: pipo
    password_hash: $y$j9T$aUmgEDoFIDPhGxEe2FUjc/$C5A...
`
	var activeReleaseManifest = &resolver.ResolvedManifest{
		CorePlatform: &core.ReleaseManifest{
			Components: core.Components{
				Systemd: api.Systemd{
					Extensions: []api.SystemdExtension{
						{
							Name:  "foo",
							Image: "https://foo.bar/remote-foo-image",
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
							Name:  "bar",
							Chart: "bar",
						},
					},
				},
			},
		},
	}
	var activeConfig = &image.Configuration{
		Network: image.Network{
			ConfigDir: configDir.NetworkDir(),
		},
		Kubernetes: kubernetes.Kubernetes{
			RemoteManifests: []string{"remote-manifest1.yaml"},
			LocalManifests:  []string{filepath.Join(configDir.KubernetesManifestsDir(), "local-manifest1.yaml")},
		},
		Release: release.Release{
			ManifestURI: "https://foo.bar/release-manifest.yaml",
			Components: release.Components{
				SystemdExtensions: []release.SystemdExtension{
					{
						Name: "foo",
					},
				},
				HelmCharts: []release.HelmChart{
					{
						Name: "bar",
					},
				},
			},
		},
	}

	BeforeEach(func() {
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			fmt.Sprintf("%s/local-manifest1.yaml", configDir.KubernetesManifestsDir()): "",
			fmt.Sprintf("%s/nmstate1.yaml", configDir.NetworkDir()):                    "",
		})
		Expect(err).ToNot(HaveOccurred())

		system, err = sys.NewSystem(
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())

		defaultHelmFunc = func(c *image.Configuration, rm *resolver.ResolvedManifest) ([]string, error) {
			return nil, nil
		}

		defaultResolveFunc = func(uri string) (*resolver.ResolvedManifest, error) {
			return nil, nil
		}
	})

	AfterEach(func() {
		cleanup()
	})

	It("Successfully applies configurations to output directory", func() {
		var butane map[string]any
		Expect(parseAny([]byte(butaneConfigString), &butane)).To(Succeed())

		conf := activeConfig
		conf.ButaneConfig = butane

		m := NewManager(
			system,
			&helmConfiguratorMock{configureFunc: func(c *image.Configuration, rm *resolver.ResolvedManifest) ([]string, error) {
				helmPath := filepath.Join(outputDir.OverlaysDir(), image.HelmPath())
				if err := vfs.MkdirAll(fs, helmPath, vfs.DirPerm); err != nil {
					return nil, err
				}

				files := []string{}
				for _, chart := range rm.ProductExtension.Components.Helm.Charts {
					files = append(files, chart.Name)
					_, err := fs.Create(filepath.Join(helmPath, chart.Name))
					if err != nil {
						return nil, err
					}
				}
				return files, nil
			}},
			WithManifestResolver(&resolverMock{resolveFunc: func(uri string) (*resolver.ResolvedManifest, error) {
				if uri == activeConfig.Release.ManifestURI {
					return activeReleaseManifest, nil
				}

				panic("missing release manifest")
			}}),
			WithDownloadFunc(func(ctx context.Context, fs vfs.FS, url, path string) error {
				_, err := fs.Create(filepath.Join(path))
				return err
			}),
		)

		r, err := m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(err).ToNot(HaveOccurred())

		Expect(r).ToNot(BeNil())
		Expect(r).To(Equal(activeReleaseManifest))

		_, err = fs.Stat(filepath.Join(outputDir.OverlaysDir(), image.HelmPath(), "bar"))
		Expect(err).ToNot(HaveOccurred())
		_, err = fs.Stat(filepath.Join(outputDir.OverlaysDir(), image.KubernetesManifestsPath(), "remote-manifest1.yaml"))
		Expect(err).ToNot(HaveOccurred())
		_, err = fs.Stat(filepath.Join(outputDir.OverlaysDir(), image.KubernetesManifestsPath(), "local-manifest1.yaml"))
		Expect(err).ToNot(HaveOccurred())
		_, err = fs.Stat(filepath.Join(outputDir.CatalystConfigDir(), "network", "nmstate1.yaml"))
		Expect(err).ToNot(HaveOccurred())
		_, err = fs.Stat(filepath.Join(outputDir.OverlaysDir(), image.ExtensionsPath(), "remote-foo-image"))
		Expect(err).ToNot(HaveOccurred())
		_, err = fs.Stat(filepath.Join(outputDir.FirstbootConfigDir(), image.IgnitionFilePath()))
		Expect(err).ToNot(HaveOccurred())
	})

	It("Fails to resolve release manifest during configuration", func() {
		By("Using default manifest resolver")
		m := NewManager(
			system,
			&helmConfiguratorMock{configureFunc: defaultHelmFunc},
		)
		conf := &image.Configuration{
			Release: release.Release{
				ManifestURI: "missing",
			},
		}

		r, err := m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/_out/store/release-manifests: no such file or directory"))

		By("Using custom manifest resolver")
		m = NewManager(
			system,
			&helmConfiguratorMock{configureFunc: defaultHelmFunc},
			WithManifestResolver(&resolverMock{resolveFunc: func(uri string) (*resolver.ResolvedManifest, error) {
				return nil, fmt.Errorf("unable to resolve manifest")
			}}),
		)
		conf = &image.Configuration{
			Release: release.Release{
				ManifestURI: "missing",
			},
		}

		r, err = m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("resolving release manifest at uri 'missing': unable to resolve manifest"))
	})

	It("Fails to configure network", func() {
		m := NewManager(
			system,
			&helmConfiguratorMock{configureFunc: defaultHelmFunc},
			WithManifestResolver(&resolverMock{resolveFunc: defaultResolveFunc}),
		)
		conf := &image.Configuration{
			Network: image.Network{
				CustomScript: "/missing/configure-network.sh",
			},
		}

		r, err := m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/missing/configure-network.sh: no such file or directory"))
	})

	It("Fails to configure kubernetes", func() {
		By("Failing helm configuration")
		m := NewManager(
			system,
			&helmConfiguratorMock{configureFunc: func(c *image.Configuration, rm *resolver.ResolvedManifest) ([]string, error) {
				return nil, fmt.Errorf("unable to configure helm charts")
			}},
			WithManifestResolver(&resolverMock{resolveFunc: defaultResolveFunc}),
		)
		conf := &image.Configuration{
			Kubernetes: kubernetes.Kubernetes{
				Helm: &kubernetes.Helm{
					Charts: []*kubernetes.HelmChart{
						{
							Name: "foo",
						},
					},
				},
			},
		}

		r, err := m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("configuring kubernetes: configuring helm charts: unable to configure helm charts"))

		By("Failing to setup local Kubernetes manifests")
		conf = &image.Configuration{
			Kubernetes: kubernetes.Kubernetes{
				LocalManifests: []string{"/missing/foo.yaml"},
			},
		}

		r, err = m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/missing/foo.yaml: no such file or directory"))

		By("Failing to setup remote Kubernetes manfifests")
		m = NewManager(
			system,
			&helmConfiguratorMock{configureFunc: defaultHelmFunc},
			WithManifestResolver(&resolverMock{resolveFunc: defaultResolveFunc}),
			WithDownloadFunc(func(ctx context.Context, fs vfs.FS, url, path string) error {
				return fmt.Errorf("download unavailable")
			}),
		)
		conf = &image.Configuration{
			Kubernetes: kubernetes.Kubernetes{
				RemoteManifests: []string{"https://foo.bar/foo.yaml"},
			},
		}

		r, err = m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("downloading remote Kubernetes manifest 'https://foo.bar/foo.yaml': download unavailable"))
	})

	It("Fails to configure ignition", func() {
		var butane map[string]any
		butaneConfigString := "breaking: breaking"
		Expect(parseAny([]byte(butaneConfigString), &butane)).To(Succeed())

		m := NewManager(
			system,
			&helmConfiguratorMock{configureFunc: defaultHelmFunc},
			WithManifestResolver(&resolverMock{resolveFunc: func(uri string) (*resolver.ResolvedManifest, error) {
				return &resolver.ResolvedManifest{CorePlatform: &core.ReleaseManifest{}}, nil
			}}),
		)

		conf := &image.Configuration{
			ButaneConfig: butane,
		}

		r, err := m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed translating Butane config: error parsing variant; must be specified"))

	})

	It("Fails to configure systemd extensions", func() {
		rMock := &resolverMock{
			resolveFunc: func(uri string) (*resolver.ResolvedManifest, error) {
				return &resolver.ResolvedManifest{
					CorePlatform: &core.ReleaseManifest{
						Components: core.Components{},
					},
					ProductExtension: &product.ReleaseManifest{
						Components: product.Components{},
					},
				}, nil
			},
		}

		m := NewManager(
			system,
			&helmConfiguratorMock{configureFunc: defaultHelmFunc},
			WithManifestResolver(rMock),
		)

		conf := &image.Configuration{
			Release: release.Release{
				Components: release.Components{
					SystemdExtensions: []release.SystemdExtension{
						{Name: "missing"},
					},
				},
			},
		}

		r, err := m.ConfigureComponents(context.Background(), conf, outputDir)
		Expect(r).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("filtering enabled systemd extensions: requested systemd extension(s) not found: [\"missing\"]"))
	})
})
