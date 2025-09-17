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

package extractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
)

// Default glob pattern used to search for a release manifest in an OCI image
const globPattern = "release_manifest*.yaml"

type OCIUnpacker interface {
	// Unpack unpacks the file system of a given OCI image to the specified destination
	// and returns its digest
	Unpack(ctx context.Context, uri, dest string, local bool) (digest string, err error)
}

type ociUnpacker struct {
	system *sys.System
}

func (o *ociUnpacker) Unpack(ctx context.Context, uri, dest string, local bool) (digest string, err error) {
	unpacker := unpack.NewOCIUnpacker(o.system, uri, unpack.WithLocalOCI(local))
	return unpacker.Unpack(ctx, dest)
}

type OCIReleaseManifestExtractor struct {
	// Location to search for the release manifest;
	// both globs (e.g. "/foo/release_manifest*.yaml")
	// and absolute paths (e.g. "/foo/release_manifest.yaml")
	// are supported.
	//
	// Defaults to '/release_manifest*.yaml' and '/etc/release-manifest/release_manifest*.yaml'.
	searchPaths []string
	// Location where all extracted release manifests will be stored.
	// Each manifest will be stored in a separate directory within
	// this root store path.
	//
	// Defaults to the OS temporary directory.
	store    string
	unpacker OCIUnpacker
	fs       vfs.FS
	ctx      context.Context
}

type Opts func(o *OCIReleaseManifestExtractor)

func WithOCIUnpacker(u OCIUnpacker) Opts {
	return func(r *OCIReleaseManifestExtractor) {
		r.unpacker = u
	}
}

func WithStore(store string) Opts {
	return func(r *OCIReleaseManifestExtractor) {
		r.store = store
	}
}

func WithSearchPaths(globs []string) Opts {
	return func(r *OCIReleaseManifestExtractor) {
		r.searchPaths = globs
	}
}

func WithFS(fs vfs.FS) Opts {
	return func(r *OCIReleaseManifestExtractor) {
		r.fs = fs
	}
}

func WithContext(ctx context.Context) Opts {
	return func(r *OCIReleaseManifestExtractor) {
		r.ctx = ctx
	}
}

func New(opts ...Opts) (*OCIReleaseManifestExtractor, error) {
	extr := &OCIReleaseManifestExtractor{
		searchPaths: []string{
			globPattern,
			filepath.Join("etc", "release-manifest", globPattern),
		},
		fs:  vfs.New(),
		ctx: context.Background(),
	}

	for _, o := range opts {
		o(extr)
	}

	if extr.store != "" {
		if _, err := extr.fs.Stat(extr.store); err != nil {
			return nil, fmt.Errorf("store path '%s' does not exist in provided filesystem: %w", extr.store, err)
		}
	} else {
		store, err := vfs.TempDir(extr.fs, "", "release-manifests-")
		if err != nil {
			return nil, fmt.Errorf("setting up default store directory: %w", err)
		}

		extr.store = store
	}

	if extr.unpacker == nil {
		s, err := sys.NewSystem(sys.WithFS(extr.fs))
		if err != nil {
			return nil, fmt.Errorf("setting up default system: %w", err)
		}

		extr.unpacker = &ociUnpacker{
			system: s,
		}
	}

	return extr, nil
}

// ExtractFrom locates and extracts a release manifest file from the given OCI image.
// The first located release manifest will be extracted to the configured store directory
// and its path will be returned, or an error if the manifest was not found.
// The underlying OCI image is not retained.
func (o *OCIReleaseManifestExtractor) ExtractFrom(uri string, local bool) (path string, err error) {
	unpackDir, err := vfs.TempDir(o.fs, "", "release-manifest-unpack-")
	if err != nil {
		return "", fmt.Errorf("creating oci image unpack directory: %w", err)
	}
	defer func() {
		_ = o.fs.RemoveAll(unpackDir)
	}()

	digest, err := o.unpacker.Unpack(o.ctx, uri, unpackDir, local)
	if err != nil {
		return "", fmt.Errorf("unpacking oci image: %w", err)
	}

	manifestInOCI, err := vfs.FindFile(o.fs, unpackDir, o.searchPaths...)
	if err != nil {
		return "", fmt.Errorf("locating release manifest at unpacked OCI filesystem: %w", err)
	}

	manifestStorePath, err := o.generateManifestStorePath(digest)
	if err != nil {
		return "", fmt.Errorf("generating manifest store based on digest: %w", err)
	}

	if err := vfs.MkdirAll(o.fs, manifestStorePath, 0700); err != nil {
		return "", fmt.Errorf("creating manifest store directory '%s': %w", manifestStorePath, err)
	}

	manifestInStore := filepath.Join(manifestStorePath, filepath.Base(manifestInOCI))
	if err := vfs.CopyFile(o.fs, manifestInOCI, manifestInStore); err != nil {
		return "", fmt.Errorf("copying release manifest to store: %w", err)
	}

	return manifestInStore, nil
}

func (o *OCIReleaseManifestExtractor) generateManifestStorePath(digest string) (string, error) {
	const maxHashLen = 64
	digestSplit := strings.Split(digest, ":")
	if len(digestSplit) != 2 || digestSplit[0] == "" || digestSplit[1] == "" {
		return "", fmt.Errorf("invalid digest format '%s', expected '<algorithm>:<hash>'", digest)
	}

	hash := digestSplit[1]
	if len(hash) > maxHashLen {
		hash = hash[:maxHashLen]
	}
	return filepath.Join(o.store, hash), nil
}
