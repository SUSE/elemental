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

package unpack

import (
	"context"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type umountFunc func() error

type Raw struct {
	s          *sys.System
	path       string
	rsyncFlags []string
}

type RawOpt func(*Raw)

func WithRsyncFlagsRaw(flags ...string) RawOpt {
	return func(r *Raw) {
		r.rsyncFlags = flags
	}
}

func NewRawUnpacker(s *sys.System, path string, opts ...RawOpt) *Raw {
	r := &Raw{s: s, path: path}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r Raw) Unpack(ctx context.Context, destination string, excludes ...string) (digest string, err error) {
	var umount umountFunc
	var mountpoint string

	mountpoint, umount, err = r.mountImage()
	if err != nil {
		return "", err
	}
	defer func() {
		nErr := umount()
		if err == nil && nErr != nil {
			err = nErr
		}
	}()

	unpackD := NewDirectoryUnpacker(r.s, mountpoint, WithRsyncFlagsDir(r.rsyncFlags...))
	return unpackD.Unpack(ctx, destination, excludes...)
}

func (r Raw) SynchedUnpack(ctx context.Context, destination string, excludes []string, deleteExcludes []string) (digest string, err error) {
	var umount umountFunc
	var mountpoint string

	mountpoint, umount, err = r.mountImage()
	if err != nil {
		return "", err
	}
	defer func() {
		nErr := umount()
		if err == nil && nErr != nil {
			err = nErr
		}
	}()

	unpackD := NewDirectoryUnpacker(r.s, mountpoint, WithRsyncFlagsDir(r.rsyncFlags...))
	return unpackD.SynchedUnpack(ctx, destination, excludes, deleteExcludes)
}

func (r Raw) mountImage() (string, umountFunc, error) {
	dir, err := vfs.TempDir(r.s.FS(), "", "elemental_unpack")
	if err != nil {
		r.s.Logger().Error("failed creating a temporary directory to unpack a raw image: %w", err)
		return "", nil, err
	}
	err = r.s.Mounter().Mount(r.path, dir, "auto", []string{"ro"})
	if err != nil {
		r.s.Logger().Error("failed mounting raw image '%s': %w", r.path, err)
		return "", nil, err
	}
	umount := func() error {
		err := r.s.Mounter().Unmount(dir)
		if err != nil {
			return err
		}
		return r.s.FS().RemoveAll(dir)
	}

	return dir, umount, nil
}
