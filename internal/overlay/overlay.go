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

package overlay

import (
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/sys/mounter"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Overlay struct {
	lowerDir  string
	upperDir  string
	workDir   string
	mergedDir string

	tempDir string

	mounter mounter.Interface
	fs      vfs.FS
}

type Opts func(*Overlay)

func WithWorkDir(dir string) Opts {
	return func(o *Overlay) {
		o.workDir = dir
	}
}

func WithTarget(dir string) Opts {
	return func(o *Overlay) {
		o.mergedDir = dir
	}
}

func New(lowerDir, upperDir string, mounter mounter.Interface, fs vfs.FS, opts ...Opts) (*Overlay, error) {
	o := &Overlay{
		lowerDir: lowerDir,
		upperDir: upperDir,
		mounter:  mounter,
		fs:       fs,
	}

	for _, opt := range opts {
		opt(o)
	}

	if o.workDir == "" || o.mergedDir == "" {
		tempDir, err := vfs.TempDir(fs, "", "overlay-")
		if err != nil {
			return nil, err
		}

		o.tempDir = tempDir

		if o.workDir == "" {
			o.workDir = filepath.Join(tempDir, ".work")
			if err = vfs.MkdirAll(fs, o.workDir, vfs.DirPerm); err != nil {
				_ = fs.RemoveAll(o.tempDir)
				return nil, err
			}
		}

		if o.mergedDir == "" {
			o.mergedDir = filepath.Join(tempDir, ".merged")
			if err = vfs.MkdirAll(fs, o.mergedDir, vfs.DirPerm); err != nil {
				_ = fs.RemoveAll(o.tempDir)
				return nil, err
			}
		}
	}

	return o, nil
}

func (o *Overlay) Mount() (string, error) {
	if err := o.mounter.Mount("overlay", o.mergedDir, "overlay", []string{
		"lowerdir=" + o.lowerDir,
		"upperdir=" + o.upperDir,
		"workdir=" + o.workDir,
	}); err != nil {
		return "", err
	}

	return o.mergedDir, nil
}

func (o *Overlay) Unmount() error {
	if err := o.mounter.Unmount(o.mergedDir); err != nil {
		return err
	}

	if o.tempDir != "" {
		_ = o.fs.RemoveAll(o.tempDir)
	}

	return nil
}
