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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/auth"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/api/core"
	"github.com/suse/elemental/v3/pkg/manifest/api/solution"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type imageStoreMock struct {
	fs vfs.FS

	logins   [][]string
	saved    [][]string
	loaded   []string
	exported []string
	stores   []string

	loginErr error
	saveErr  error
}

func (m *imageStoreMock) Login(_ context.Context, registry, username, password string) error {
	m.logins = append(m.logins, []string{registry, username, password})
	return m.loginErr
}

func (m *imageStoreMock) SaveImage(_ context.Context, image, platform, archive string) error {
	if m.saveErr != nil {
		return m.saveErr
	}

	m.saved = append(m.saved, []string{image, platform})
	return m.fs.WriteFile(archive, []byte(image), vfs.FilePerm)
}

func (m *imageStoreMock) LoadStore(_ context.Context, store string, archives ...string) error {
	m.loaded = append([]string{store}, archives...)
	return nil
}

func (m *imageStoreMock) ExportRegistry(_ context.Context, store, dir string) error {
	m.exported = []string{store, dir}
	return nil
}

func (m *imageStoreMock) WriteEmptyStore(dir string) error {
	m.stores = append(m.stores, dir)
	return nil
}

func (m *imageStoreMock) HaulerPath(string) (string, error) {
	return "/usr/bin/hauler", nil
}

var _ = Describe("Registry", func() {
	const localManifest = "/config/kubernetes/manifests/workloads.yaml"
	const workloads = `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      initContainers:
      - image: busybox:1.36
      containers:
      - image: docker.io/library/nginx:1.27
      - image: quay.io/app/web:2.0
---
apiVersion: v1
kind: ConfigMap
data:
  image: not-a-workload:1.0
`

	var fs vfs.FS
	var cleanup func()
	var system *sys.System
	var store *imageStoreMock
	var output Output
	var butaneCfg *butane.Config
	var conf *image.Configuration
	var rm *resolver.ResolvedManifest
	var registryDir string

	findUnit := func(name string) *string {
		for _, unit := range butaneCfg.Systemd.Units {
			if unit.Name == name {
				Expect(unit.Enabled).To(HaveValue(BeTrue()))
				return unit.Contents
			}
		}
		return nil
	}

	BeforeEach(func() {
		var err error
		fs, cleanup, err = sysmock.TestFS(map[string]any{
			localManifest:     workloads,
			"/usr/bin/hauler": "hauler",
		})
		Expect(err).ToNot(HaveOccurred())

		system, err = sys.NewSystem(
			sys.WithFS(fs),
			sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).ToNot(HaveOccurred())

		store = &imageStoreMock{fs: fs}
		output = Output{RootPath: "/_out"}
		butaneCfg = &butane.Config{}
		registryDir = filepath.Join(output.OverlaysDir(), image.RegistryPath())

		rm = &resolver.ResolvedManifest{
			CorePlatform: &core.ReleaseManifest{
				Components: core.Components{
					Kubernetes: &core.Kubernetes{},
				},
			},
			SolutionExtension: &solution.ReleaseManifest{
				Components: solution.Components{
					Helm: &api.Helm{
						Charts: []*api.HelmChart{
							{
								Name:   "bar",
								Chart:  "bar",
								Images: []api.HelmChartImage{{Image: "registry.example.com/charts/bar:1.0"}},
							},
							{
								Name:   "disabled",
								Chart:  "disabled",
								Images: []api.HelmChartImage{{Image: "registry.example.com/charts/disabled:1.0"}},
							},
						},
					},
				},
			},
		}

		conf = &image.Configuration{
			Kubernetes: kubernetes.Kubernetes{
				LocalManifests: []string{localManifest},
				ElementalEmbeddedRegistry: &kubernetes.ElementalEmbeddedRegistry{
					ContainerImages: []kubernetes.ContainerImage{{Name: "quay.io/app/web:2.0"}, {Name: "quay.io/app/extra:1.0"}},
					Registries: []kubernetes.Registry{
						{URI: "quay.io", Credentials: &auth.Credentials{Username: "user", Password: "secret"}},
						{URI: "registry.example.com"},
					},
				},
			},
			Release: release.Release{
				Components: release.Components{
					HelmCharts: []release.HelmChart{{Name: "bar"}},
				},
			},
		}
	})

	AfterEach(func() {
		cleanup()
	})

	It("Embeds every collected image and configures the registry", func() {
		helm := &helmConfiguratorMock{containerImages: []string{"ghcr.io/helm/rendered:3.0", "quay.io/app/web:2.0"}}
		m := NewManager(system, helm, WithImageStore(store), WithPlatform("linux/arm64"))

		hosts, err := m.configureElementalRegistry(context.Background(), conf, rm, output, butaneCfg)
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).To(Equal([]string{"docker.io", "ghcr.io", "quay.io", "registry.example.com"}))

		Expect(store.logins).To(Equal([][]string{{"quay.io", "user", "secret"}}))
		Expect(store.saved).To(Equal([][]string{
			{"busybox:1.36", "linux/arm64"},
			{"docker.io/library/nginx:1.27", "linux/arm64"},
			{"ghcr.io/helm/rendered:3.0", "linux/arm64"},
			{"quay.io/app/extra:1.0", "linux/arm64"},
			{"quay.io/app/web:2.0", "linux/arm64"},
			{"registry.example.com/charts/bar:1.0", "linux/arm64"},
		}))

		storeDir := filepath.Join(output.RegistryStoreDir(), "store")
		Expect(store.loaded).To(HaveLen(7))
		Expect(store.loaded[0]).To(Equal(storeDir))
		for _, archive := range store.loaded[1:] {
			Expect(archive).To(HavePrefix(filepath.Join(output.RegistryStoreDir(), "archives")))
			Expect(fs.ReadFile(archive)).NotTo(BeEmpty())
		}

		Expect(store.exported).To(Equal([]string{storeDir, filepath.Join(registryDir, "registry")}))
		Expect(store.stores).To(Equal([]string{filepath.Join(registryDir, "store")}))

		info, err := fs.Stat(filepath.Join(registryDir, "hauler"))
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Mode().Perm()).To(BeEquivalentTo(0o755))

		unit := findUnit("elemental-registry.service")
		Expect(unit).NotTo(BeNil())
		Expect(*unit).To(ContainSubstring("ExecStart=/var/lib/elemental/registry/hauler store serve registry"))
		Expect(*unit).To(ContainSubstring("--store /var/lib/elemental/registry/store"))
		Expect(*unit).To(ContainSubstring("--directory /var/lib/elemental/registry/registry"))
		Expect(*unit).To(ContainSubstring("--port 6545"))
	})

	It("Skips the registry when there are no images to embed", func() {
		conf.Kubernetes = kubernetes.Kubernetes{}
		conf.Release.Components.HelmCharts = nil
		m := NewManager(system, &helmConfiguratorMock{}, WithImageStore(store))

		hosts, err := m.configureElementalRegistry(context.Background(), conf, rm, output, butaneCfg)
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).To(BeNil())
		Expect(store.saved).To(BeEmpty())
		Expect(findUnit("elemental-registry.service")).To(BeNil())
	})

	It("Fails when a registry login fails", func() {
		store.loginErr = fmt.Errorf("unauthorized")
		m := NewManager(system, &helmConfiguratorMock{}, WithImageStore(store))

		_, err := m.configureElementalRegistry(context.Background(), conf, rm, output, butaneCfg)
		Expect(err).To(MatchError(ContainSubstring("unauthorized")))
		Expect(store.saved).To(BeEmpty())
	})

	It("Fails when an image cannot be saved", func() {
		store.saveErr = fmt.Errorf("manifest unknown")
		m := NewManager(system, &helmConfiguratorMock{}, WithImageStore(store))

		_, err := m.configureElementalRegistry(context.Background(), conf, rm, output, butaneCfg)
		Expect(err).To(MatchError(ContainSubstring("embedding image busybox:1.36")))
		Expect(err).To(MatchError(ContainSubstring("manifest unknown")))
	})

	It("Fails on a malformed local manifest", func() {
		Expect(fs.WriteFile(localManifest, []byte("kind: [broken"), vfs.FilePerm)).To(Succeed())
		m := NewManager(system, &helmConfiguratorMock{}, WithImageStore(store))

		_, err := m.configureElementalRegistry(context.Background(), conf, rm, output, butaneCfg)
		Expect(err).To(MatchError(ContainSubstring("reading manifest")))
	})

	Describe("Manifest images", func() {
		It("Collects images from workload resources only", func() {
			resources, err := parseManifests(strings.NewReader(workloads))
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(2))

			images := map[string]struct{}{}
			for _, resource := range resources {
				extractManifestImages(resource, images)
			}
			Expect(images).To(HaveLen(3))
			Expect(images).To(HaveKey("busybox:1.36"))
			Expect(images).To(HaveKey("docker.io/library/nginx:1.27"))
			Expect(images).To(HaveKey("quay.io/app/web:2.0"))
		})

		It("Skips empty documents", func() {
			resources, err := parseManifests(strings.NewReader("---\n\n---\nkind: Pod\n---\n"))
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))
		})

		It("Collects images from the items of a List", func() {
			list := `
kind: List
items:
- kind: Pod
  spec:
    containers:
    - image: busybox:1.36
- kind: ConfigMap
  data:
    image: not-a-workload:1.0
- kind: List
  items:
  - kind: Job
    spec:
      template:
        spec:
          containers:
          - image: quay.io/app/job:2.0
`
			resources, err := parseManifests(strings.NewReader(list))
			Expect(err).ToNot(HaveOccurred())

			images := map[string]struct{}{}
			extractManifestImages(resources[0], images)
			Expect(images).To(HaveLen(2))
			Expect(images).To(HaveKey("busybox:1.36"))
			Expect(images).To(HaveKey("quay.io/app/job:2.0"))
		})
	})

	Describe("Archive names", func() {
		It("Never collide for distinct references", func() {
			Expect(registryArchiveName("quay.io/a/b_c:v1")).NotTo(Equal(registryArchiveName("quay.io/a_b/c:v1")))
			Expect(registryArchiveName("quay.io/a/b:v1")).To(Equal(registryArchiveName("quay.io/a/b:v1")))
			Expect(registryArchiveName("quay.io/a/b:v1")).NotTo(ContainSubstring("/"))
		})
	})

	Describe("Image hostnames", func() {
		It("Always includes Docker Hub and deduplicates registries", func() {
			hosts, err := imageHostnames([]string{"nginx:1.27", "quay.io/a/b:1", "quay.io/c/d:2", "localhost:5000/e:3"})
			Expect(err).ToNot(HaveOccurred())
			Expect(hosts).To(Equal([]string{"docker.io", "localhost:5000", "quay.io"}))
		})

		It("Fails on an invalid reference", func() {
			_, err := imageHostnames([]string{"quay.io/UPPER case:1"})
			Expect(err).To(MatchError(ContainSubstring("parsing image reference")))
		})
	})
})
