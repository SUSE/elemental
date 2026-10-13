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

package cache_test

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	containerregistry "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/suse/elemental/v3/pkg/cache"
	"github.com/suse/elemental/v3/pkg/log"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Cache", Label("cache"), func() {
	var fs vfs.FS
	var cleanup func()
	var ctx context.Context
	var dir string
	var filesDir string
	var ociDir string
	var logger log.Logger

	newCache := func(mode cache.Mode) *cache.Cache {
		c, err := cache.New(dir, cache.WithFS(fs), cache.WithLogger(logger), cache.WithPolicy(cache.Policy{Mode: mode}))
		Expect(err).NotTo(HaveOccurred())
		return c
	}

	BeforeEach(func() {
		var err error
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		ctx = context.Background()
		logger = log.New(log.WithDiscardAll())
		dir = "/cache"
		filesDir = filepath.Join(dir, "files")
		ociDir = filepath.Join(dir, "oci")
	})

	AfterEach(func() {
		cleanup()
	})

	It("Fails on an invalid cache mode", func() {
		_, err := cache.New(dir, cache.WithFS(fs), cache.WithPolicy(cache.Policy{Mode: "sometimes"}))
		Expect(err).To(MatchError(ContainSubstring("invalid cache mode")))
	})

	It("Creates the cache directories", func() {
		newCache(cache.Auto)
		Expect(vfs.Exists(fs, filesDir)).To(BeTrue())
		Expect(vfs.Exists(fs, ociDir)).To(BeTrue())
	})

	Describe("Files", func() {
		var fetches int
		var fetch cache.FileFetcher
		var dest string

		BeforeEach(func() {
			fetches = 0
			fetch = func(context.Context) (io.ReadCloser, error) {
				fetches++
				return io.NopCloser(strings.NewReader("payload")), nil
			}
			dest = "/dest/file"
			Expect(vfs.MkdirAll(fs, filepath.Dir(dest), vfs.DirPerm)).To(Succeed())
		})

		It("Downloads a missing file once and serves it from the cache afterwards", func() {
			c := newCache(cache.Auto)

			Expect(c.File(ctx, "key", dest, fetch)).To(Succeed())
			Expect(fs.ReadFile(dest)).To(BeEquivalentTo("payload"))
			Expect(fetches).To(Equal(1))

			Expect(fs.Remove(dest)).To(Succeed())
			Expect(c.File(ctx, "key", dest, fetch)).To(Succeed())
			Expect(fs.ReadFile(dest)).To(BeEquivalentTo("payload"))
			Expect(fetches).To(Equal(1))
		})

		It("Fails for a file missing from the cache in offline mode", func() {
			c := newCache(cache.Offline)

			err := c.File(ctx, "key", dest, fetch)
			Expect(err).To(MatchError(ContainSubstring("downloads are disabled")))
			Expect(fetches).To(Equal(0))
		})

		It("Serves cached files in offline mode", func() {
			Expect(newCache(cache.Auto).File(ctx, "key", dest, fetch)).To(Succeed())
			Expect(fs.Remove(dest)).To(Succeed())

			c := newCache(cache.Offline)
			Expect(c.File(ctx, "key", dest, fetch)).To(Succeed())
			Expect(fs.ReadFile(dest)).To(BeEquivalentTo("payload"))
			Expect(fetches).To(Equal(1))
		})

		It("Always downloads and never writes the cache when disabled", func() {
			c := cache.Disabled(fs)

			Expect(c.File(ctx, "key", dest, fetch)).To(Succeed())
			Expect(c.File(ctx, "key", dest, fetch)).To(Succeed())
			Expect(fetches).To(Equal(2))
			Expect(vfs.Exists(fs, filesDir)).To(BeFalse())
		})

		It("Leaves no partial file behind when the download fails", func() {
			c := newCache(cache.Auto)
			fetch = func(context.Context) (io.ReadCloser, error) {
				return io.NopCloser(failingReader{}), nil
			}

			err := c.File(ctx, "key", dest, fetch)
			Expect(err).To(MatchError(ContainSubstring("writing file")))

			entries, err := fs.ReadDir(filesDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
			Expect(vfs.Exists(fs, dest)).To(BeFalse())
		})
	})

	Describe("Images", func() {
		var fetches int
		var fetch cache.ImageFetcher
		var img containerregistry.Image
		var digest containerregistry.Hash
		var amd64 containerregistry.Platform
		var arm64 containerregistry.Platform
		const ref = "registry.example.com/foo/bar:1.0"

		BeforeEach(func() {
			var err error
			img, err = newImage("linux", "amd64")
			Expect(err).NotTo(HaveOccurred())
			digest, err = img.Digest()
			Expect(err).NotTo(HaveOccurred())

			fetches = 0
			fetch = func(context.Context) (containerregistry.Image, error) {
				fetches++
				return img, nil
			}
			amd64 = containerregistry.Platform{OS: "linux", Architecture: "amd64"}
			arm64 = containerregistry.Platform{OS: "linux", Architecture: "arm64"}
		})

		It("Downloads a missing image once and serves it from the cache afterwards", func() {
			c := newCache(cache.Auto)

			cached, err := c.Image(ctx, ref, amd64, fetch)
			Expect(err).NotTo(HaveOccurred())
			Expect(cached.Digest()).To(Equal(digest))
			Expect(fetches).To(Equal(1))

			cached, err = c.Image(ctx, ref, amd64, fetch)
			Expect(err).NotTo(HaveOccurred())
			Expect(cached.Digest()).To(Equal(digest))
			Expect(fetches).To(Equal(1))
		})

		It("Caches the same reference separately per platform", func() {
			c := newCache(cache.Auto)

			_, err := c.Image(ctx, ref, amd64, fetch)
			Expect(err).NotTo(HaveOccurred())

			_, err = c.Image(ctx, ref, arm64, fetch)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetches).To(Equal(2))
		})

		It("Fails for an image missing from the cache in offline mode", func() {
			c := newCache(cache.Offline)

			_, err := c.Image(ctx, ref, amd64, fetch)
			Expect(err).To(MatchError(ContainSubstring("downloads are disabled")))
			Expect(fetches).To(Equal(0))
		})

		It("Serves images seeded into the layout without a platform", func() {
			Expect(seedImage(fs, ociDir, ref, img)).To(Succeed())
			c := newCache(cache.Offline)

			cached, err := c.Image(ctx, ref, amd64, fetch)
			Expect(err).NotTo(HaveOccurred())
			Expect(cached.Digest()).To(Equal(digest))
			Expect(fetches).To(Equal(0))
		})

		It("Ignores seeded images built for another platform", func() {
			Expect(seedImage(fs, ociDir, ref, img)).To(Succeed())
			c := newCache(cache.Offline)

			_, err := c.Image(ctx, ref, arm64, fetch)
			Expect(err).To(MatchError(ContainSubstring("downloads are disabled")))
		})

		It("Always downloads when disabled", func() {
			c := cache.Disabled(fs)

			_, err := c.Image(ctx, ref, amd64, fetch)
			Expect(err).NotTo(HaveOccurred())
			_, err = c.Image(ctx, ref, amd64, fetch)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetches).To(Equal(2))
			Expect(vfs.Exists(fs, ociDir)).To(BeFalse())
		})
	})

	Describe("Refresh", func() {
		It("Discards cached artifacts but nothing else in the directory", func() {
			Expect(vfs.MkdirAll(fs, filesDir, vfs.DirPerm)).To(Succeed())
			Expect(vfs.MkdirAll(fs, ociDir, vfs.DirPerm)).To(Succeed())
			Expect(fs.WriteFile(filepath.Join(filesDir, "stale"), []byte("old"), vfs.FilePerm)).To(Succeed())
			Expect(fs.WriteFile(filepath.Join(ociDir, "stale"), []byte("old"), vfs.FilePerm)).To(Succeed())
			Expect(fs.WriteFile(filepath.Join(dir, "unrelated"), []byte("keep"), vfs.FilePerm)).To(Succeed())

			newCache(cache.Refresh)

			Expect(vfs.Exists(fs, filepath.Join(filesDir, "stale"))).To(BeFalse())
			Expect(vfs.Exists(fs, filepath.Join(ociDir, "stale"))).To(BeFalse())
			Expect(fs.ReadFile(filepath.Join(dir, "unrelated"))).To(BeEquivalentTo("keep"))
		})
	})
})

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("connection reset")
}

func newImage(os, arch string) (containerregistry.Image, error) {
	img, err := random.Image(256, 1)
	if err != nil {
		return nil, err
	}

	config, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	config = config.DeepCopy()
	config.OS = os
	config.Architecture = arch

	return mutate.ConfigFile(img, config)
}

func seedImage(fs vfs.FS, ociDir, ref string, img containerregistry.Image) error {
	raw, err := fs.RawPath(ociDir)
	if err != nil {
		return err
	}

	path, err := layout.Write(raw, empty.Index)
	if err != nil {
		return err
	}

	return path.AppendImage(img, layout.WithAnnotations(map[string]string{ocispec.AnnotationRefName: ref}))
}
