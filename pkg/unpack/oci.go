/*
Copyright © 2022-2026 SUSE LLC
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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/schollz/progressbar/v3"

	"github.com/suse/elemental/v3/pkg/containerd"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	containerregistry "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	CtrdSockEnv = "CONTAINERD_SOCK"

	workDirSuffix = ".workdir"
	ctrdNamespace = "k8s.io"
)

type OCI struct {
	s           *sys.System
	platformRef string
	local       bool
	verify      bool
	imageRef    string
	rsyncFlags  []string
	ctrdSock    string
	ctrd        containerd.Interface
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

func WithContainerd(ctrd containerd.Interface) OCIOpt {
	return func(o *OCI) {
		o.ctrd = ctrd
	}
}

func NewOCIUnpacker(s *sys.System, imageRef string, opts ...OCIOpt) *OCI {
	unpacker := &OCI{
		s:           s,
		verify:      true,
		platformRef: s.Platform().String(),
		imageRef:    imageRef,
	}

	for _, o := range opts {
		o(unpacker)
	}

	if unpacker.local {
		sock := os.Getenv(CtrdSockEnv)
		if ok, _ := vfs.Exists(unpacker.s.FS(), sock); ok {
			unpacker.ctrdSock = sock
		}
	}

	return unpacker
}

func (o OCI) SynchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (digest string, err error) {
	if o.ctrdSock != "" {
		return o.synchedUnpackContainerd(ctx, destination, excludes, deleteExcludes)
	}
	return o.synchedUnpack(ctx, destination, excludes, deleteExcludes)
}

func (o OCI) Unpack(ctx context.Context, destination string, excludes ...string) (digest string, err error) {
	if o.ctrdSock != "" {
		return o.unpackContainerd(ctx, destination, excludes...)
	}

	return o.unpack(ctx, destination, excludes...)
}

// synchedUnpack for OCI images will extract OCI contents to a destination sibling directory first and
// after that it will sync it to the destination directory. Ideally the destination path should
// not be mountpoint to a different filesystem of the sibling directories in order to benefit of
// copy on write features of the base filesystem.
func (o OCI) synchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (digest string, err error) {
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
	digest, err = o.unpack(ctx, tempDir)
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

func (o OCI) unpack(ctx context.Context, destination string, excludes ...string) (string, error) {
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

	digest, err := img.ConfigName()
	if err != nil {
		return "", err
	}

	reader := mutate.Extract(img)
	defer reader.Close()

	destination, err = o.s.FS().RawPath(destination)
	if err != nil {
		return "", err
	}

	bar := progressbar.DefaultBytes(-1, "Extracting")
	defer bar.Close()

	r := progressbar.NewReader(reader, bar)

	_, err = containerd.Apply(ctx, destination, &r, excludesFilter(destination, excludes...))

	return digest.String(), err
}

func fetchImage(ctx context.Context, ref name.Reference, platform containerregistry.Platform, local bool) (containerregistry.Image, error) {
	if local {
		return daemon.Image(ref,
			daemon.WithContext(ctx),
			daemon.WithUnbufferedOpener())
	}

	return remote.Image(ref,
		remote.WithTransport(http.DefaultTransport),
		remote.WithPlatform(platform),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
}

func (o OCI) synchedUnpackContainerd(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (string, error) {
	callback := func(rootfs string) error {
		unpackD := NewDirectoryUnpacker(o.s, rootfs, WithRsyncFlagsDir(o.rsyncFlags...))
		_, e := unpackD.SynchedUnpack(ctx, destination, excludes, deleteExcludes)
		return e
	}
	return o.onMountedContainerdImage(ctx, callback)
}

func (o OCI) unpackContainerd(ctx context.Context, destination string, excludes ...string) (string, error) {
	callback := func(rootfs string) error {
		unpackD := NewDirectoryUnpacker(o.s, rootfs, WithRsyncFlagsDir(o.rsyncFlags...))
		_, e := unpackD.Unpack(ctx, destination, excludes...)
		return e
	}
	return o.onMountedContainerdImage(ctx, callback)
}

// onMountedContainerdImage mounts as RO the image reference, if found in containerd, and then runs the given callback.
// Returns the image config digest (imageID) of the image on success or error otherwise. Mountpoints are unmounted and
// resources freed after executing the method. The given callback gets as input the mountpoint of the image root-tree.
func (o OCI) onMountedContainerdImage(ctx context.Context, callback func(rootfs string) error) (string, error) {
	if !o.local {
		return "", fmt.Errorf("only unpacked images cand be mounted")
	}

	if o.ctrd == nil {
		ctrd, err := containerd.NewWrapper(o.s, o.ctrdSock)
		if err != nil {
			return "", err
		}
		o.ctrd = ctrd
	}
	ctx = containerd.AddNamespace(ctx, ctrdNamespace)

	img, err := o.ctrd.FindUnpackedImage(ctx, o.imageRef)
	if err != nil {
		return "", err
	}

	err = o.ctrd.RunOnMountedROSnapshot(ctx, img, callback)
	if err != nil {
		return "", err
	}
	return img.Digest, nil
}
