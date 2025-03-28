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

package unpacker

import (
	"context"
	"net/http"
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

type OCI struct {
	fs          vfs.FS
	platformRef string
	local       bool
	verify      bool
}

type OCIOpt func(*OCI)

func WithLocal() OCIOpt {
	return func(o *OCI) {
		o.local = true
	}
}

func WithNoVerify() OCIOpt {
	return func(o *OCI) {
		o.verify = false
	}
}

func WithPlatformRef(platform string) OCIOpt {
	return func(o *OCI) {
		o.platformRef = platform
	}
}

func NewOCIUnpacker(s *sys.System, opts ...OCIOpt) *OCI {
	unpacker := &OCI{
		fs:          s.FS(),
		verify:      true,
		platformRef: s.Platform().String(),
	}
	for _, o := range opts {
		o(unpacker)
	}
	return unpacker
}

func (o OCI) Unpack(ctx context.Context, imageRef, destination string) (string, error) {
	platform, err := containerregistry.ParsePlatform(o.platformRef)
	if err != nil {
		return "", err
	}

	opts := []name.Option{}
	if !o.verify {
		opts = append(opts, name.Insecure)
	}

	ref, err := name.ParseReference(imageRef, opts...)
	if err != nil {
		return "", err
	}

	var img containerregistry.Image

	err = backoff.Retry(func() error {
		img, err = image(ctx, ref, *platform, o.local)
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

	destination, err = o.fs.RawPath(destination)
	if err != nil {
		return "", err
	}

	_, err = archive.Apply(ctx, destination, reader)
	return digest.String(), err
}

func image(ctx context.Context, ref name.Reference, platform containerregistry.Platform, local bool) (containerregistry.Image, error) {
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
