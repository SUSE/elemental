/*
Copyright © 2022-2025 SUSE LLC
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

package unpack

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/containerd/containerd/v2/pkg/archive"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const workDirSuffix = ".workdir"

type OCI struct {
	s           *sys.System
	platformRef string
	local       bool
	verify      bool
	imageRef    string
	rsyncFlags  []string
}

type OCIOpt func(*OCI)

func WithLocalOCI(local bool) OCIOpt {
	return func(o *OCI) {
		o.local = local
	}
}

func WithVerifyOCI(verify bool) OCIOpt {
	return func(o *OCI) {
		o.verify = verify
	}
}

func WithPlatformRefOCI(platform string) OCIOpt {
	return func(o *OCI) {
		o.platformRef = platform
	}
}

func WithRsyncFlagsOCI(flags ...string) OCIOpt {
	return func(o *OCI) {
		o.rsyncFlags = flags
	}
}

func NewOCIUnpacker(s *sys.System, imageRef string, opts ...OCIOpt) *OCI {
	unpacker := &OCI{s: s, imageRef: imageRef, platformRef: s.Platform().String()}
	for _, o := range opts {
		o(unpacker)
	}
	return unpacker
}

// SynchedUnpack for OCI images will extract OCI contents to a destination sibling directory first and
// after that it will sync it to the destination directory. Ideally the destination path should
// not be mountpoint to a different filesystem of the sibling directories in order to benefit of
// copy on write features of the underlaying filesystem.
func (o OCI) SynchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (digest string, err error) {
	tempDir := filepath.Clean(destination) + workDirSuffix
	err = vfs.MkdirAll(o.s.FS(), tempDir, vfs.DirPerm)
	if err != nil {
		return "", err
	}
	defer func() {
		e := vfs.ForceRemoveAll(o.s.FS(), tempDir)
		if err == nil && e != nil {
			err = e
		}
	}()
	digest, err = o.Unpack(ctx, tempDir)
	if err != nil {
		return "", err
	}
	unpackD := NewDirectoryUnpacker(o.s, tempDir, WithRsyncFlagsDir(o.rsyncFlags...))
	_, err = unpackD.SynchedUnpack(ctx, destination, excludes, deleteExcludes)
	if err != nil {
		return "", err
	}
	return digest, nil
}

func (o OCI) Unpack(ctx context.Context, destination string, excludes ...string) (string, error) {
	platform, err := containerregistry.ParsePlatform(o.platformRef)
	if err != nil {
		return "", err
	}

	opts := []name.Option{}
	if !o.verify {
		opts = append(opts, name.Insecure)
	}

	ref, err := name.ParseReference(o.imageRef, opts...)
	if err != nil {
		return "", err
	}

	var img containerregistry.Image

	err = backoff.Retry(func() error {
		img, err = fetchImage(ctx, ref, *platform, o.local)
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(3*time.Second), 3))
	if err != nil {
		return "", err
	}

	digest, err := img.Digest()
	if err != nil {
		return "", err
	}

	reader := mutate.Extract(img)
	defer reader.Close()

	destination, err = o.s.FS().RawPath(destination)
	if err != nil {
		return "", err
	}

	filter := excludesFilter(destination, excludes...)
	_, err = archive.Apply(ctx, destination, reader, archive.WithFilter(filter))
	return digest.String(), err
}

func fetchImage(ctx context.Context, ref name.Reference, platform containerregistry.Platform, local bool) (containerregistry.Image, error) {
	if local {
		return daemon.Image(ref, daemon.WithContext(ctx))
	}

	return remote.Image(ref,
		remote.WithTransport(http.DefaultTransport),
		remote.WithPlatform(platform),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
}
