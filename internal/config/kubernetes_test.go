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
	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/auth"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
)

var _ = Describe("Kubernetes", func() {
	Describe("Resources trigger", func() {
		It("Skips manifests setup if manifests are not provided", func() {
			conf := &image.Configuration{}
			Expect(needsManifestsSetup(conf)).To(BeFalse())
		})

		It("Requires manifests setup if local manifests are provided", func() {
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					LocalManifests: []string{"/apache.yaml"},
				},
			}
			Expect(needsManifestsSetup(conf)).To(BeTrue())
		})

		It("Requires manifests setup if remote manifests are provided", func() {
			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					RemoteManifests: []string{"https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml"},
				},
			}
			Expect(needsManifestsSetup(conf)).To(BeTrue())
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

		It("Requires Helm setup if solution charts are provided", func() {
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
		var system *sys.System
		var err error
		var config *butane.Config
		var defaultManifest *resolver.ResolvedManifest
		var defaultUnpack func(ctx context.Context, imageRef, destDir string) error
		var defaultHelm *helmConfiguratorMock

		BeforeEach(func() {
			config = &butane.Config{}

			system, err = sys.NewSystem(
				sys.WithLogger(log.New(log.WithDiscardAll())),
			)
			Expect(err).ToNot(HaveOccurred())

			defaultManifest = &resolver.ResolvedManifest{
				CorePlatform: &core.ReleaseManifest{
					Components: core.Components{
						Kubernetes: &core.Kubernetes{
							Version: "v1.35.0+rke2r1",
							Image:   "registry.example.com/rke2:1.35_1.0",
						},
					},
				},
			}

			defaultUnpack = func(ctx context.Context, imageRef, destDir string) error {
				return nil
			}

			defaultHelm = &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest, _ *butane.Config) ([]string, error) {
					return []string{}, nil
				},
			}
		})

		It("Fails to configure Helm charts", func() {
			helmMock := &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest, _ *butane.Config) ([]string, error) {
					return nil, fmt.Errorf("helm error")
				},
			}

			m := NewManager(
				system,
				helmMock,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

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

			err := m.configureKubernetes(context.Background(), conf, defaultManifest, config)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("configuring helm charts: helm error"))
		})

		It("Succeeds to configure RKE2 with additional resources", func() {
			helmMock := &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest, _ *butane.Config) ([]string, error) {
					return []string{"rancher.yaml"}, nil
				},
			}

			m := NewManager(
				system,
				helmMock,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

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

			Expect(m.configureKubernetes(context.Background(), conf, defaultManifest, config)).To(Succeed())

			// Verify deployment script contents
			data := findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sResDeployScriptName))
			Expect(data).NotTo(BeNil())

			Expect(*data).To(ContainSubstring("deployHelmCharts"))
			Expect(*data).To(ContainSubstring("rancher.yaml"))
			Expect(*data).To(ContainSubstring("deployManifests"))
			Expect(*data).To(ContainSubstring("deployPriorityManifests"))

			// k8s configuration script is generated
			Expect(findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sConfDeployScriptName))).NotTo(BeNil())
		})

		It("Succeeds to configure RKE2 without additional resources", func() {
			m := NewManager(
				system,
				nil,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

			conf := &image.Configuration{
				Release: release.Release{
					Components: release.Components{
						Kubernetes: &struct{}{},
					},
				},
			}

			Expect(m.configureKubernetes(context.Background(), conf, defaultManifest, config)).To(Succeed())

			Expect(findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sConfDeployScriptName))).NotTo(BeNil())
		})

		It("Defaults to a server init node for a single-node cluster with no declared nodes", func() {
			m := NewManager(
				system,
				defaultHelm,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

			conf := &image.Configuration{Kubernetes: kubernetes.Kubernetes{}}

			Expect(m.configureKubernetes(context.Background(), conf, defaultManifest, config)).To(Succeed())

			data := findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sConfDeployScriptName))
			Expect(data).NotTo(BeNil())

			Expect(*data).To(ContainSubstring(`: "${K8S_DIR:=/var/lib/elemental/kubernetes}"`))
			Expect(*data).To(ContainSubstring(`: "${INIT_PATH:=${K8S_DIR}/init.yaml}"`))
			Expect(*data).To(ContainSubstring(`: "${IS_INIT_NODE:=${is_init_node}}"`))
			Expect(*data).To(ContainSubstring(`[[ "${IS_INIT_NODE}" == "true" ]] && CONFIGFILE="${INIT_PATH}"`))
			// Configuration specific to statically defined init node should not be present when no nodes where defined.
			Expect(*data).ToNot(ContainSubstring(`[[ "${HOSTNAME}" == "{{ .InitNode.Hostname }}" ]] && is_init_node=true`))

			data = findFileContentsInConfig(config, filepath.Join("/", image.RuntimeEnvPath()))
			Expect(data).NotTo(BeNil())

			Expect(*data).To(ContainSubstring(`IS_INIT_NODE=true`))
			Expect(*data).To(ContainSubstring(`NODETYPE=server`))
		})

		It("Uses server init config for a single explicitly configured server node", func() {
			m := NewManager(
				system,
				defaultHelm,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					Nodes: kubernetes.Nodes{
						{Hostname: "node01", Type: kubernetes.NodeTypeServer},
					},
				},
			}

			Expect(m.configureKubernetes(context.Background(), conf, defaultManifest, config)).To(Succeed())

			data := findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sConfDeployScriptName))
			Expect(data).NotTo(BeNil())

			Expect(*data).To(ContainSubstring(`: "${K8S_DIR:=/var/lib/elemental/kubernetes}"`))
			Expect(*data).To(ContainSubstring(`: "${INIT_PATH:=${K8S_DIR}/init.yaml}"`))
			Expect(*data).To(ContainSubstring(`: "${IS_INIT_NODE:=${is_init_node}}"`))
			Expect(*data).To(ContainSubstring(`[[ "${HOSTNAME}" == "node01" ]] && is_init_node=true`))
			Expect(*data).To(ContainSubstring(`[[ "${IS_INIT_NODE}" == "true" ]] && CONFIGFILE="${INIT_PATH}"`))

			data = findFileContentsInConfig(config, filepath.Join("/", image.RuntimeEnvPath()))
			Expect(data).To(BeNil())
		})

		It("Fails when there is no server in the static node configuration", func() {
			m := NewManager(
				system,
				defaultHelm,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

			conf := &image.Configuration{
				Kubernetes: kubernetes.Kubernetes{
					Nodes: kubernetes.Nodes{
						{Hostname: "node01", Type: kubernetes.NodeTypeAgent},
					},
				},
			}

			err := m.configureKubernetes(context.Background(), conf, defaultManifest, config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`could not find suitable init node from node list: [{Hostname:node01 Type:agent Init:false}]`))
		})

		It("Succeeds to configure RKE2 with additional resources and auth", func() {
			additionalManifests := make(map[string][]byte)
			additionalManifests["example-auth-priority.yaml"] = []byte("apiVersion: v1\nkind: Secret\nmetadata:\n    namespace: kube-system\n    name: example-auth\ntype: kubernetes.io/dockerconfigjson\ndata:\n    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLmlvIjp7InVzZXJuYW1lIjoiZXhhbXBsZS11c2VyIiwicGFzc3dvcmQiOiJleGFtcGxlLXBhc3MiLCJhdXRoIjoiWlhoaGJYQnNaUzExYzJWeU9tVjRZVzF3YkdVdGNHRnpjdz09In19fQ==\n")
			additionalManifests["endpoint-copier-operator-auth-priority.yaml"] = []byte("apiVersion: v1\nkind: Secret\nmetadata:\n    namespace: kube-system\n    name: endpoint-copier-operator-auth\ntype: kubernetes.io/dockerconfigjson\ndata:\n    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLTEuY29tIjp7InVzZXJuYW1lIjoiZWNvLXVzZXIiLCJwYXNzd29yZCI6ImVjby1wYXNzIiwiYXV0aCI6IlpXTnZMWFZ6WlhJNlpXTnZMWEJoYzNNPSJ9fX0=\n")

			helmMock := &helmConfiguratorMock{
				configureFunc: func(conf *image.Configuration, manifest *resolver.ResolvedManifest, bc *butane.Config) ([]string, error) {
					for k, v := range additionalManifests {
						bc.AddFileInline(filepath.Join("/", image.KubernetesManifestsPath(), k), new(string(v)), 0o644)
					}
					return []string{"rancher.yaml"}, nil
				},
			}

			m := NewManager(
				system,
				helmMock,
				WithDownloader(&downloaderMock{}),
				WithUnpackFunc(defaultUnpack),
			)

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

			Expect(m.configureKubernetes(context.Background(), conf, defaultManifest, config)).To(Succeed())

			// Verify deployment script contents
			data := findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sResDeployScriptName))
			Expect(data).NotTo(BeNil())

			Expect(*data).To(ContainSubstring("deployHelmCharts"))
			Expect(*data).To(ContainSubstring("rancher.yaml"))
			Expect(*data).To(ContainSubstring("deployManifests"))
			Expect(*data).To(ContainSubstring("deployPriorityManifests"))

			// Verify config script contents
			data = findFileContentsInConfig(config, filepath.Join("/", image.KubernetesPath(), k8sConfDeployScriptName))
			Expect(data).NotTo(BeNil())

			expectedECOManifestContents := `apiVersion: v1
kind: Secret
metadata:
    namespace: kube-system
    name: endpoint-copier-operator-auth
type: kubernetes.io/dockerconfigjson
data:
    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLTEuY29tIjp7InVzZXJuYW1lIjoiZWNvLXVzZXIiLCJwYXNzd29yZCI6ImVjby1wYXNzIiwiYXV0aCI6IlpXTnZMWFZ6WlhJNlpXTnZMWEJoYzNNPSJ9fX0=`

			data = findFileContentsInConfig(config, filepath.Join("/", image.KubernetesManifestsPath(), "endpoint-copier-operator-auth-priority.yaml"))
			Expect(data).NotTo(BeNil())
			Expect(*data).To(ContainSubstring(expectedECOManifestContents))

			expectedExampleManifestContents := `apiVersion: v1
kind: Secret
metadata:
    namespace: kube-system
    name: example-auth
type: kubernetes.io/dockerconfigjson
data:
    .dockerconfigjson: eyJhdXRocyI6eyJleGFtcGxlLmlvIjp7InVzZXJuYW1lIjoiZXhhbXBsZS11c2VyIiwicGFzc3dvcmQiOiJleGFtcGxlLXBhc3MiLCJhdXRoIjoiWlhoaGJYQnNaUzExYzJWeU9tVjRZVzF3YkdVdGNHRnpjdz09In19fQ==`

			data = findFileContentsInConfig(config, filepath.Join("/", image.KubernetesManifestsPath(), "example-auth-priority.yaml"))
			Expect(data).NotTo(BeNil())
			Expect(*data).To(ContainSubstring(expectedExampleManifestContents))
		})
	})
})
