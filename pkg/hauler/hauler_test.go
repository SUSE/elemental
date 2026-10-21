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

package hauler_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/hauler"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestHaulerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hauler test suite")
}

var _ = Describe("Hauler", Label("hauler"), func() {
	var runner *sysmock.Runner
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var buffer *bytes.Buffer
	var h *hauler.Hauler
	var ctx context.Context

	rawPath := func(path string) string {
		raw, err := fs.RawPath(path)
		Expect(err).NotTo(HaveOccurred())
		return raw
	}

	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		buffer = &bytes.Buffer{}
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		logger := log.New(log.WithBuffer(buffer))
		logger.SetLevel(log.DebugLevel())
		s, err = sys.NewSystem(
			sys.WithRunner(runner), sys.WithFS(fs),
			sys.WithLogger(logger), sys.WithPlatform("linux/amd64"),
		)
		Expect(err).NotTo(HaveOccurred())

		h = hauler.New(s)
		ctx = context.Background()
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("HaulerPath", func() {
		It("Prefers the bundled binary for the requested platform", func() {
			bundled := filepath.Join(hauler.HaulerDir, "arm64", hauler.HaulerBinary)
			Expect(vfs.MkdirAll(fs, filepath.Dir(bundled), vfs.DirPerm)).To(Succeed())
			Expect(fs.WriteFile(bundled, []byte{}, vfs.FilePerm)).To(Succeed())

			Expect(h.HaulerPath("linux/arm64")).To(Equal(bundled))
		})

		It("Falls back to the executable in PATH for the host platform", func() {
			h = hauler.New(s, hauler.WithBinary("sh"))

			Expect(h.HaulerPath("linux/amd64")).To(HaveSuffix("/sh"))
		})

		It("Fails for a foreign platform without a bundled binary", func() {
			_, err := h.HaulerPath("linux/arm64")
			Expect(err).To(MatchError(ContainSubstring("no hauler binary available for platform linux/arm64")))
		})

		It("Fails on an invalid platform", func() {
			_, err := h.HaulerPath("amd64")
			Expect(err).To(MatchError(ContainSubstring("parsing platform")))
		})
	})

	Describe("Login", func() {
		It("Logs into the registry", func() {
			Expect(h.Login(ctx, "registry.example.com", "user", "secret")).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"hauler", "login", "registry.example.com", "--username", "user", "--password", "secret"},
			})).To(Succeed())
		})

		It("Reports the command output on failure", func() {
			runner.ReturnError = fmt.Errorf("exit status 1")
			runner.ReturnValue = []byte("unauthorized")

			err := h.Login(ctx, "registry.example.com", "user", "secret")
			Expect(err).To(MatchError(ContainSubstring("logging into registry registry.example.com")))
			Expect(err).To(MatchError(ContainSubstring("unauthorized")))
		})
	})

	Describe("SaveImage", func() {
		It("Adds the image to a temporary store and saves it to the archive", func() {
			Expect(h.SaveImage(ctx, "quay.io/foo/bar:1", "linux/arm64", "/out/bar.tar.zst")).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{"hauler", "store", "add", "image", "quay.io/foo/bar:1", "--platform", "linux/arm64", "--store"},
				{"hauler", "store", "save", "--filename", rawPath("/out/bar.tar.zst"), "--store"},
			})).To(Succeed())

			entries, err := fs.ReadDir("/tmp")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})

		It("Fails when the image cannot be added", func() {
			runner.ReturnError = fmt.Errorf("exit status 1")

			err := h.SaveImage(ctx, "quay.io/foo/bar:1", "linux/amd64", "/out/bar.tar.zst")
			Expect(err).To(MatchError(ContainSubstring("adding image quay.io/foo/bar:1 to hauler store")))
		})
	})

	Describe("LoadStore", func() {
		It("Loads every archive into the store", func() {
			Expect(h.LoadStore(ctx, "/registry/store", "/archives/a.tar.zst", "/archives/b.tar.zst")).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{
					"hauler", "store", "load", "--store", rawPath("/registry/store"), "--tempdir", rawPath("/registry"),
					"--filename", rawPath("/archives/a.tar.zst"), "--filename", rawPath("/archives/b.tar.zst"),
				},
			})).To(Succeed())
		})

		It("Fails when hauler cannot load the archives", func() {
			runner.ReturnError = fmt.Errorf("exit status 1")

			err := h.LoadStore(ctx, "/registry/store", "/archives/a.tar.zst")
			Expect(err).To(MatchError(ContainSubstring("loading archives into hauler store")))
		})
	})

	Describe("ExportRegistry", func() {
		It("Succeeds once the registry reports it is listening", func() {
			runner.ReturnStderr = []byte("level=info msg=\"listening on [::]:40163\"")

			Expect(h.ExportRegistry(ctx, "/registry/store", "/registry/backend")).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{
					"hauler", "store", "serve", "registry", "--store", rawPath("/registry/store"),
					"--directory", rawPath("/registry/backend"), "--port", "0",
				},
			})).To(Succeed())
			Expect(buffer.String()).NotTo(ContainSubstring("Hauler registry output"))
		})

		It("Fails and logs the output when the registry never comes up", func() {
			runner.ReturnValue = []byte("copying store\nsomething went wrong")
			runner.ReturnError = fmt.Errorf("exit status 1")

			err := h.ExportRegistry(ctx, "/registry/store", "/registry/backend")
			Expect(err).To(MatchError(ContainSubstring("exporting hauler store to registry")))
			Expect(buffer.String()).To(ContainSubstring("Hauler registry output"))
			Expect(buffer.String()).To(ContainSubstring("something went wrong"))
		})
	})

	Describe("WriteEmptyStore", func() {
		It("Writes an empty OCI layout", func() {
			Expect(h.WriteEmptyStore("/registry/store")).To(Succeed())
			Expect(fs.ReadFile("/registry/store/index.json")).To(MatchJSON(`{"schemaVersion":2,"manifests":[]}`))
			Expect(fs.ReadFile("/registry/store/oci-layout")).To(MatchJSON(`{"imageLayoutVersion":"1.0.0"}`))
		})
	})
})
