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

type OCIFileExtractor struct {
	// Location to search for the desired file;
	// both globs (e.g. "/foo/file*.yaml")
	// and absolute paths (e.g. "/foo/file.yaml")
	// are supported.
	searchPaths []string
	// Location where all extracted files will be stored.
	// Each file will be stored in a separate directory within
	// this root store path.
	//
	// Defaults to the OS temporary directory.
	store    string
	unpacker OCIUnpacker
	fs       vfs.FS
	ctx      context.Context
	local    bool
}

type OCIFileExtractorOpts func(o *OCIFileExtractor)

func WithOCIUnpacker(u OCIUnpacker) OCIFileExtractorOpts {
	return func(r *OCIFileExtractor) {
		r.unpacker = u
	}
}

func WithStore(store string) OCIFileExtractorOpts {
	return func(r *OCIFileExtractor) {
		r.store = store
	}
}

func WithFS(fs vfs.FS) OCIFileExtractorOpts {
	return func(r *OCIFileExtractor) {
		r.fs = fs
	}
}

func WithContext(ctx context.Context) OCIFileExtractorOpts {
	return func(r *OCIFileExtractor) {
		r.ctx = ctx
	}
}

func WithLocal(local bool) OCIFileExtractorOpts {
	return func(r *OCIFileExtractor) {
		r.local = local
	}
}

func New(searchPaths []string, opts ...OCIFileExtractorOpts) (*OCIFileExtractor, error) {
	extr := &OCIFileExtractor{
		searchPaths: searchPaths,
		fs:          vfs.New(),
		ctx:         context.Background(),
	}

	for _, o := range opts {
		o(extr)
	}

	if extr.store != "" {
		if _, err := extr.fs.Stat(extr.store); err != nil {
			return nil, fmt.Errorf("store path '%s' does not exist in provided filesystem: %w", extr.store, err)
		}
	} else {
		store, err := vfs.TempDir(extr.fs, "", "extracted-files-")
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

// ExtractFrom locates and extracts a file from the given OCI image.
// The first located file will be extracted to the configured store directory
// and its path will be returned, or an error if the file was not found.
// The underlying OCI image is not retained.
func (o *OCIFileExtractor) ExtractFrom(uri string) (path string, err error) {
	unpackDir, err := vfs.TempDir(o.fs, "", "unpacked-oci-")
	if err != nil {
		return "", fmt.Errorf("creating oci image unpack directory: %w", err)
	}
	defer func() {
		_ = o.fs.RemoveAll(unpackDir)
	}()

	digest, err := o.unpacker.Unpack(o.ctx, uri, unpackDir, o.local)
	if err != nil {
		return "", fmt.Errorf("unpacking oci image: %w", err)
	}

	fileInOCI, err := vfs.FindFile(o.fs, unpackDir, o.searchPaths...)
	if err != nil {
		return "", fmt.Errorf("locating file at unpacked OCI filesystem: %w", err)
	}

	fileStorePath, err := o.generateFileStorePath(digest)
	if err != nil {
		return "", fmt.Errorf("generating file store based on digest: %w", err)
	}

	if err := vfs.MkdirAll(o.fs, fileStorePath, 0700); err != nil {
		return "", fmt.Errorf("creating file store directory '%s': %w", fileStorePath, err)
	}

	fileInStore := filepath.Join(fileStorePath, filepath.Base(fileInOCI))
	if err := vfs.CopyFile(o.fs, fileInOCI, fileInStore); err != nil {
		return "", fmt.Errorf("copying file to store: %w", err)
	}

	return fileInStore, nil
}

func (o *OCIFileExtractor) generateFileStorePath(digest string) (string, error) {
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
