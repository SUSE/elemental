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

package vfs

import (
	"io/fs"
	"os"

	"github.com/twpayne/go-vfs/v4"
)

type vfsOS struct {
	osfs vfs.FS
}

func OSFS() *vfsOS { //nolint:revive
	return &vfsOS{osfs: vfs.OSFS}
}

func (f vfsOS) Chmod(name string, mode fs.FileMode) error {
	return f.osfs.Chmod(name, mode)
}

func (f vfsOS) Create(name string) (*os.File, error) {
	return f.osfs.Create(name)
}

func (f vfsOS) Link(oldname, newname string) error {
	return f.osfs.Link(oldname, newname)
}

func (f vfsOS) Lstat(name string) (fs.FileInfo, error) {
	return f.osfs.Lstat(name)
}

func (f vfsOS) Mkdir(name string, perm fs.FileMode) error {
	return f.osfs.Mkdir(name, perm)
}

func (f vfsOS) Open(name string) (fs.File, error) {
	return f.osfs.Open(name)
}

func (f vfsOS) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return f.osfs.OpenFile(name, flag, perm)
}

func (f vfsOS) RawPath(name string) (string, error) {
	return f.osfs.RawPath(name)
}

func (f vfsOS) ReadDir(dirname string) ([]fs.DirEntry, error) {
	return f.osfs.ReadDir(dirname)
}

func (f vfsOS) ReadFile(filename string) ([]byte, error) {
	return f.osfs.ReadFile(filename)
}

func (f vfsOS) Readlink(name string) (string, error) {
	return f.osfs.Readlink(name)
}

func (f vfsOS) Remove(name string) error {
	return f.osfs.Remove(name)
}

func (f vfsOS) RemoveAll(name string) error {
	return f.osfs.RemoveAll(name)
}

func (f vfsOS) Rename(oldpath, newpath string) error {
	return f.osfs.Rename(oldpath, newpath)
}

func (f vfsOS) Stat(name string) (fs.FileInfo, error) {
	return f.osfs.Stat(name)
}

func (f vfsOS) Symlink(oldname, newname string) error {
	return f.osfs.Symlink(oldname, newname)
}

func (f vfsOS) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	return f.osfs.WriteFile(filename, data, perm)
}
