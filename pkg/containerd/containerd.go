/*
Copyright © 2026 SUSE LLC
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

package containerd

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/pkg/archive"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/platforms"
	"github.com/opencontainers/image-spec/identity"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Interface interface {
	// FindUnpackedImage looks for the given image reference in containerd and then checks it is already unpacked.
	// Returns error if not found or not unpacked.
	FindUnpackedImage(ctx context.Context, imageRef string) (ImgMeta, error)

	// RunOnMountedROSnapshot mounts as RO the unpacked snapshot from containerd deamon for the given image
	// reference and runs the callback with generated mountpoint. Once the callback is executed the mountpoint
	// is unmounted and resources freed. The callback gets as input the mountpoint of the image snapshot.
	RunOnMountedROSnapshot(ctx context.Context, img ImgMeta, callback func(rootfs string) error) (err error)
}

type ImgMeta struct {
	ImgRef      string
	ChainID     string
	Digest      string
	Snapshotter string
}

// AddNamespace sets a namespace for the given context
func AddNamespace(ctx context.Context, namespace string) context.Context {
	return namespaces.WithNamespace(ctx, namespace)
}

// Apply applies a tar stream to the destination folder, accepts a tar filter
func Apply(ctx context.Context, destination string, r io.Reader, filter func(h *tar.Header) (bool, error)) (int64, error) {
	return archive.Apply(ctx, destination, r, archive.WithFilter(filter))
}

type Wrapper struct {
	cli *client.Client
	s   *sys.System
}

func NewWrapper(s *sys.System, address string, opt ...client.Opt) (*Wrapper, error) {
	cli, err := client.New(address, opt...)
	if err != nil {
		return nil, err
	}
	cliWrapper := &Wrapper{
		cli: cli,
		s:   s,
	}
	return cliWrapper, nil
}

func (w Wrapper) FindUnpackedImage(ctx context.Context, imageRef string) (ImgMeta, error) {
	var img ImgMeta
	image, err := w.cli.GetImage(ctx, imageRef)
	if err != nil {
		return img, fmt.Errorf("failed to get image from containerd: %w", err)
	}
	w.s.Logger().Debug("image '%s' found in containerd daemon", imageRef)

	introService := w.cli.IntrospectionService()
	pluginsResp, err := introService.Plugins(ctx, "type==io.containerd.snapshotter.v1")
	if err != nil {
		return img, fmt.Errorf("failed to get containerd snapshotter plugins: %w", err)
	}

	// discover the current containerd snapshotter
	var snapshotterName string
	for _, p := range pluginsResp.Plugins {
		if ok, _ := image.IsUnpacked(ctx, p.ID); !ok {
			w.s.Logger().Debug("image not unpacked in %s snapshotter", p.ID)
			continue
		}
		snapshotterName = p.ID
		break
	}
	if snapshotterName == "" {
		return img, fmt.Errorf("image '%s' is not unpacked", imageRef)
	}

	cs := w.cli.ContentStore()
	manifest, err := images.Manifest(ctx, cs, image.Target(), platforms.Default())
	if err != nil {
		return img, fmt.Errorf("failed to resolve platform-specific manifest: %w", err)
	}
	digest := manifest.Config.Digest.String()

	diffIDs, err := image.RootFS(ctx)
	if err != nil {
		return img, fmt.Errorf("failed to get image rootfs: %w", err)
	}
	chainID := identity.ChainID(diffIDs).String()

	img.ImgRef = imageRef
	img.ChainID = chainID
	img.Digest = digest
	img.Snapshotter = snapshotterName

	return img, nil
}

func (w Wrapper) RunOnMountedROSnapshot(ctx context.Context, img ImgMeta, callback func(rootfs string) error) (err error) {
	tempDir, err := vfs.TempDir(w.s.FS(), "", "elemental-mnt-")
	if err != nil {
		return err
	}
	defer func() {
		e := vfs.ForceRemoveAll(w.s.FS(), tempDir)
		if err == nil && e != nil {
			err = e
		}
	}()

	key := fmt.Sprintf("extraction-view-%s", img.ChainID)

	// protect the snapshot from the garbage collector
	ctx, done, err := w.cli.WithLease(ctx,
		leases.WithID(key),
		leases.WithExpiration(30*time.Minute),
		leases.WithLabel("containerd.io/gc.ref.snapshot."+img.Snapshotter, key),
	)
	defer func() {
		e := done(ctx)
		if err == nil && e != nil {
			err = e
		}
	}()
	snapshotter := w.cli.SnapshotService(img.Snapshotter)

	mounts, err := snapshotter.View(ctx, key, img.ChainID)
	if err != nil {
		return fmt.Errorf("failed to get mounts for chainID %s: %w", img.ChainID, err)
	}
	defer func() {
		e := snapshotter.Remove(ctx, key)
		if err == nil && e != nil {
			err = e
		}
	}()
	w.s.Logger().Debug("created snapshot with key %s", key)

	if err := mount.All(mounts, tempDir); err != nil {
		return fmt.Errorf("failed to mount to %s: %w", tempDir, err)
	}
	defer func() {
		e := mount.UnmountAll(tempDir, 0)
		if err == nil && e != nil {
			err = e
		}
	}()
	w.s.Logger().Debug("snapshot mounted at %s", tempDir)

	err = callback(tempDir)
	if err != nil {
		return err
	}
	return nil
}
