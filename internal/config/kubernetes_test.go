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
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/auth"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Kubernetes", func() {
	Describe("Resources trigger", func() {
		It("Skips manifests setup if manifests are not provided", func() {
			conf := &image.Configuration{}
			var additionalManifests map[string][]byte
			Expect(needsManifestsSetup(conf, additionalManifests)).To(BeFalse())
		})

		It("Requires manifests setup if local manifests are provided", func() {
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					LocalManifests: []string{"/apache.yaml"},
				},
			}
			var additionalManifests map[string][]byte
			Expect(needsManifestsSetup(conf, additionalManifests)).To(BeTrue())
		})

		It("Requires manifests setup if there are runtime secrets", func() {
			conf := &image.Configuration{}
			additionalManifests := make(map[string][]byte)
			additionalManifests["example"] = []byte("test")
			Expect(needsManifestsSetup(conf, additionalManifests)).To(BeTrue())
		})

		It("Requires manifests setup if remote manifests are provided", func() {
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					RemoteManifests: []string{"https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml"},
				},
			}
			var additionalManifests map[string][]byte
			Expect(needsManifestsSetup(conf, additionalManifests)).To(BeTrue())
		})

		It("Skips Helm setup if charts are not provided", func() {
			conf := &image.Configuration{}
			Expect(needsHelmChartsSetup(conf)).To(BeFalse())
		})

		It("Requires Helm setup if user charts are provided", func() {
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{Name: "apache", RepositoryName: "apache-repo"},
						},
					},
				},
			}
			Expect(needsHelmChartsSetup(conf)).To(BeTrue())
		})

		It("Requires Helm setup if core charts are provided", func() {
			conf := &image.Configuration{
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

			Expect(needsHelmChartsSetup(conf)).To(BeTrue())
		})

		It("Requires Helm setup if product charts are provided", func() {
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

			Expect(needsHelmChartsSetup(conf)).To(BeTrue())
		})
	})

	Describe("Configuration", func() {
		var output = Output{
			RootPath: "/_out",
		}

		var system *sys.System
		var fs vfs.FS
		var cleanup func()
		var err error

		BeforeEach(func() {
			fs, cleanup, err = sysmock.TestFS(nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(vfs.MkdirAll(fs, output.RootPath, vfs.DirPerm)).To(Succeed())

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
			helmMock := &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest) ([]string, map[string][]byte, error) {
					return nil, nil, fmt.Errorf("helm error")
				},
			}

			dlFunc := func(ctx context.Context, fs vfs.FS, url, path string) error {
				return nil
			}

			unpackFunc := func(ctx context.Context, imageRef, destDir string) error {
				installSh := filepath.Join(destDir, "install.sh")
				return fs.WriteFile(installSh, []byte("#!/bin/sh\necho test"), 0755)
			}

			m := NewManager(
				system,
				helmMock,
				WithDownloadFunc(dlFunc),
				WithUnpackFunc(unpackFunc),
			)

			manifest := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Kubernetes: &core.Kubernetes{
							Version: "v1.35.0+rke2r1",
							Image:   "registry.example.com/rke2:1.35_1.0",
						},
					},
				},
			}
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

			script, confScript, err := m.configureKubernetes(context.Background(), conf, manifest, output)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("configuring helm charts: helm error"))
			Expect(script).To(BeEmpty())
			Expect(confScript).To(BeEmpty())
		})

		It("Fails to unpack Kubernetes artifacts", func() {
			unpackFunc := func(ctx context.Context, imageRef, destDir string) error {
				return fmt.Errorf("unpacking error")
			}

			m := NewManager(
				system,
				nil,
				WithUnpackFunc(unpackFunc),
			)

			manifest := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Kubernetes: &core.Kubernetes{
							Version: "v1.35.0+rke2r1",
							Image:   "registry.example.com/rke2:1.35_1.0",
						},
					},
				},
			}
			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						Kubernetes: &struct{}{},
					},
				},
			}

			script, confScript, err := m.configureKubernetes(context.Background(), conf, manifest, output)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("unpacking kubernetes artifacts: unpacking error"))
			Expect(script).To(BeEmpty())
			Expect(confScript).To(BeEmpty())
		})

		It("Succeeds to configure RKE2 with additional resources", func() {
			helmMock := &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest) ([]string, map[string][]byte, error) {
					return []string{"rancher.yaml"}, nil, nil
				},
			}

			dlFunc := func(ctx context.Context, fs vfs.FS, url, path string) error {
				return nil
			}

			unpackFunc := func(ctx context.Context, imageRef, destDir string) error {
				installSh := filepath.Join(destDir, "install.sh")
				return fs.WriteFile(installSh, []byte("#!/bin/sh\necho test"), 0755)
			}

			m := NewManager(
				system,
				helmMock,
				WithDownloadFunc(dlFunc),
				WithUnpackFunc(unpackFunc),
			)

			manifest := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Kubernetes: &core.Kubernetes{
							Version: "v1.35.0+rke2r1",
							Image:   "registry.example.com/rke2:1.35_1.0",
						},
					},
				},
			}
			conf := &image.Configuration{
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

			script, confScript, err := m.configureKubernetes(context.Background(), conf, manifest, output)
			Expect(err).NotTo(HaveOccurred())
			Expect(script).To(Equal("/var/lib/elemental/kubernetes/k8s_res_deploy.sh"))

			// Verify deployment script contents
			b, err := fs.ReadFile(filepath.Join(output.OverlaysDir(), script))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(ContainSubstring("deployHelmCharts"))
			Expect(string(b)).To(ContainSubstring("rancher.yaml"))
			Expect(string(b)).To(ContainSubstring("deployManifests"))
			Expect(string(b)).To(ContainSubstring("deployPriorityManifests"))

			_, err = fs.ReadFile(filepath.Join(output.OverlaysDir(), confScript))
			Expect(err).NotTo(HaveOccurred())
		})

		It("Succeeds to configure RKE2 without additional resources", func() {
			dlFunc := func(ctx context.Context, fs vfs.FS, url, path string) error {
				return nil
			}

			unpackFunc := func(ctx context.Context, imageRef, destDir string) error {
				installSh := filepath.Join(destDir, "install.sh")
				return fs.WriteFile(installSh, []byte("#!/bin/sh\necho test"), 0755)
			}

			m := NewManager(
				system,
				nil,
				WithDownloadFunc(dlFunc),
				WithUnpackFunc(unpackFunc),
			)

			manifest := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Kubernetes: &core.Kubernetes{
							Version: "v1.35.0+rke2r1",
							Image:   "registry.example.com/rke2:1.35_1.0",
						},
					},
				},
			}
			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						Kubernetes: &struct{}{},
					},
				},
			}

			script, confScript, err := m.configureKubernetes(context.Background(), conf, manifest, output)
			Expect(err).NotTo(HaveOccurred())
			Expect(script).To(BeEmpty())
			Expect(confScript).ToNot(BeEmpty())
		})

		It("Succeeds to configure RKE2 with additional resources and auth", func() {
			additionalManifests := make(map[string][]byte)
			additionalManifests["example-auth-priority.yaml"] = []byte("apiVersion: v1\nkind: Secret\nmetadata:\n    namespace: kube-system\n    name: example-auth\ntype: kubernetes.io/dockerconfigjson\ndata:\n    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLmlvIjp7InVzZXJuYW1lIjoiZXhhbXBsZS11c2VyIiwicGFzc3dvcmQiOiJleGFtcGxlLXBhc3MiLCJhdXRoIjoiWlhoaGJYQnNaUzExYzJWeU9tVjRZVzF3YkdVdGNHRnpjdz09In19fQ==\n")
			additionalManifests["endpoint-copier-operator-auth-priority.yaml"] = []byte("apiVersion: v1\nkind: Secret\nmetadata:\n    namespace: kube-system\n    name: endpoint-copier-operator-auth\ntype: kubernetes.io/dockerconfigjson\ndata:\n    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLTEuY29tIjp7InVzZXJuYW1lIjoiZWNvLXVzZXIiLCJwYXNzd29yZCI6ImVjby1wYXNzIiwiYXV0aCI6IlpXTnZMWFZ6WlhJNlpXTnZMWEJoYzNNPSJ9fX0=\n")
			helmMock := &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest) ([]string, map[string][]byte, error) {
					return []string{"rancher.yaml"}, additionalManifests, nil
				},
			}

			dlFunc := func(ctx context.Context, fs vfs.FS, url, path string) error {
				return nil
			}

			unpackFunc := func(ctx context.Context, imageRef, destDir string) error {
				installSh := filepath.Join(destDir, "install.sh")
				return fs.WriteFile(installSh, []byte("#!/bin/sh\necho test"), 0755)
			}

			m := NewManager(
				system,
				helmMock,
				WithDownloadFunc(dlFunc),
				WithUnpackFunc(unpackFunc),
			)

			manifest := &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Kubernetes: &core.Kubernetes{
							Version: "v1.35.0+rke2r1",
							Image:   "registry.example.com/rke2:1.35_1.0",
						},
					},
				},
			}
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					RemoteManifests: []string{"some-url"},
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{
								Name:            "example",
								RepositoryName:  "example-repo",
								Version:         "1.0",
								TargetNamespace: "exampleNamespace",
							},
						},
						Repositories: []*kubernetes.HelmRepository{
							{
								Name: "example-repo",
								URL:  "https://example.io",
								Credentials: &auth.Credentials{
									Username: "example-user",
									Password: "example-pass",
								},
							},
						},
					},
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
							{
								Name: "endpoint-copier-operator",
								Credentials: &auth.Credentials{
									Username: "eco-user",
									Password: "eco-pass",
								},
							},
						},
					},
				},
			}

			script, confScript, err := m.configureKubernetes(context.Background(), conf, manifest, output)
			Expect(err).NotTo(HaveOccurred())
			Expect(script).To(Equal("/var/lib/elemental/kubernetes/k8s_res_deploy.sh"))

			// Verify deployment script contents
			b, err := fs.ReadFile(filepath.Join(output.OverlaysDir(), script))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(ContainSubstring("deployHelmCharts"))
			Expect(string(b)).To(ContainSubstring("rancher.yaml"))
			Expect(string(b)).To(ContainSubstring("deployManifests"))
			Expect(string(b)).To(ContainSubstring("deployPriorityManifests"))

			_, err = fs.ReadFile(filepath.Join(output.OverlaysDir(), confScript))
			Expect(err).NotTo(HaveOccurred())

			expectedECOManifestContents := `apiVersion: v1
kind: Secret
metadata:
    namespace: kube-system
    name: endpoint-copier-operator-auth
type: kubernetes.io/dockerconfigjson
data:
    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLTEuY29tIjp7InVzZXJuYW1lIjoiZWNvLXVzZXIiLCJwYXNzd29yZCI6ImVjby1wYXNzIiwiYXV0aCI6IlpXTnZMWFZ6WlhJNlpXTnZMWEJoYzNNPSJ9fX0=`
			ecoSecretManifests := "endpoint-copier-operator-auth-priority.yaml"
			relativeManifestsPath := filepath.Join("/", image.KubernetesManifestsPath())
			manifestsDir := filepath.Join(output.OverlaysDir(), relativeManifestsPath)
			b, err = fs.ReadFile(filepath.Join(manifestsDir, ecoSecretManifests))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(ContainSubstring(expectedECOManifestContents))

			expectedExampleManifestContents := `apiVersion: v1
kind: Secret
metadata:
    namespace: kube-system
    name: example-auth
type: kubernetes.io/dockerconfigjson
data:
    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLmlvIjp7InVzZXJuYW1lIjoiZXhhbXBsZS11c2VyIiwicGFzc3dvcmQiOiJleGFtcGxlLXBhc3MiLCJhdXRoIjoiWlhoaGJYQnNaUzExYzJWeU9tVjRZVzF3YkdVdGNHRnpjdz09In19fQ==`
			exampleSecretManifests := "example-auth-priority.yaml"
			b, err = fs.ReadFile(filepath.Join(manifestsDir, exampleSecretManifests))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(ContainSubstring(expectedExampleManifestContents))
		})
	})
})
