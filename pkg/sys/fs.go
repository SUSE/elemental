/*
Copyright © 2022 - 2025 SUSE LLC

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

package sys

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DirPerm        = os.ModeDir | os.ModePerm
	FilePerm       = 0666
	NoWriteDirPerm = 0555 | os.ModeDir
	TempDirPerm    = os.ModePerm | os.ModeSticky | os.ModeDir
)

type FS interface {
	Chmod(name string, mode fs.FileMode) error
	Create(name string) (*os.File, error)
	Link(oldname, newname string) error
	Lstat(name string) (fs.FileInfo, error)
	Mkdir(name string, perm fs.FileMode) error
	Open(name string) (fs.File, error)
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
	RawPath(name string) (string, error)
	ReadDir(dirname string) ([]fs.DirEntry, error)
	ReadFile(filename string) ([]byte, error)
	Readlink(name string) (string, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldpath, newpath string) error
	Stat(name string) (fs.FileInfo, error)
	Symlink(oldname, newname string) error
	WriteFile(filename string, data []byte, perm fs.FileMode) error
}

// DirSize returns the accumulated size of all files in folder. Result in bytes
func DirSize(fs FS, path string, excludes ...string) (int64, error) {
	var size int64
	err := WalkDirFs(fs, path, func(loopPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			for _, exclude := range excludes {
				if strings.HasPrefix(loopPath, exclude) {
					return filepath.SkipDir
				}
			}
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// DirSizeMB returns the accumulated size of all files in folder. Result in Megabytes
func DirSizeMB(fs FS, path string, excludes ...string) (uint, error) {
	size, err := DirSize(fs, path, excludes...)
	if err != nil {
		return 0, err
	}

	MB := int64(1024 * 1024)
	sizeMB := (size/MB*MB + MB) / MB
	if sizeMB > 0 {
		return uint(sizeMB), nil
	}
	return 0, fmt.Errorf("negative size calculation: %d", sizeMB)
}

// Check if a file or directory exists, follow flag determines to
// follow or not symlinks to check files existance.
func Exists(fs FS, path string, follow ...bool) (bool, error) {
	var err error
	if len(follow) > 0 && follow[0] {
		_, err = fs.Stat(path)
	} else {
		_, err = fs.Lstat(path)
	}
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// RemoveAll removes the specified path.
// It silently drop NotExists errors.
func RemoveAll(fs FS, path string) error {
	err := fs.RemoveAll(path)
	if !os.IsNotExist(err) {
		return err
	}

	return nil
}

// IsDir check if the path is a dir. follow flag determines to
// follow symlinks.
func IsDir(f FS, path string, follow ...bool) (bool, error) {
	var err error
	var fi fs.FileInfo

	if len(follow) > 0 && follow[0] {
		fi, err = f.Stat(path)
	} else {
		fi, err = f.Lstat(path)
	}
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

// MkdirAll is equivalent to os.MkdirAll but operates on fileSystem.
// Code ported from go-vfs library
func MkdirAll(fileSystem FS, path string, perm fs.FileMode) error {
	err := fileSystem.Mkdir(path, perm)
	switch {
	case err == nil:
		// Mkdir was successful.
		return nil
	case errors.Is(err, fs.ErrExist):
		// path already exists, but we don't know whether it's a directory or
		// something else. We get this error if we try to create a subdirectory
		// of a non-directory, for example if the parent directory of path is a
		// file. There's a race condition here between the call to Mkdir and the
		// call to Stat but we can't avoid it because there's not enough
		// information in the returned error from Mkdir. We need to distinguish
		// between "path already exists and is already a directory" and "path
		// already exists and is not a directory". Between the call to Mkdir and
		// the call to Stat path might have changed.
		info, statErr := fileSystem.Stat(path)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() {
			return err
		}
		return nil
	case errors.Is(err, fs.ErrNotExist):
		// Parent directory does not exist. Create the parent directory
		// recursively, then try again.
		parentDir := filepath.Dir(path)
		if parentDir == "/" || parentDir == "." {
			// We cannot create the root directory or the current directory, so
			// return the original error.
			return err
		}
		if err := MkdirAll(fileSystem, parentDir, perm); err != nil {
			return err
		}
		return fileSystem.Mkdir(path, perm)
	default:
		// Some other error.
		return err
	}
}

// ReadLink calls fs.Readlink but trims temporary prefix on Readlink result
func ReadLink(fs FS, name string) (string, error) {
	res, err := fs.Readlink(name)
	if err != nil {
		return res, err
	}
	raw, err := fs.RawPath(name)
	return strings.TrimPrefix(res, strings.TrimSuffix(raw, name)), err
}

// Random number state.
// We generate random temporary file names so that there's a good
// chance the file doesn't exist yet - keeps the number of tries in
// TempFile to a minimum.
var (
	randSeed uint32
	randmu   sync.Mutex
)

func reseed() uint32 {
	return uint32(time.Now().UnixNano() + int64(os.Getpid()))
}

func nextRandom() string {
	randmu.Lock()
	r := randSeed
	if r == 0 {
		r = reseed()
	}
	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	randSeed = r
	randmu.Unlock()
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}

// TempDir creates a temp file in the virtual fs
// Took from afero.FS code and adapted
func TempDir(fs FS, dir, prefix string) (name string, err error) {
	var raw string
	if dir == "" {
		dir = os.TempDir()
	}

	// This skips adding random stuff on test fs. This makes unit tests predictable
	try := filepath.Join(dir, prefix)
	raw, err = fs.RawPath(try)
	if err == nil && raw != try {
		err = MkdirAll(fs, try, 0700)
		if err == nil {
			name = try
		}
		return
	}

	nconflict := 0
	for range 10000 {
		try = filepath.Join(dir, prefix+nextRandom())
		err = MkdirAll(fs, try, 0700)
		if os.IsExist(err) {
			if nconflict++; nconflict > 10 {
				randmu.Lock()
				randSeed = reseed()
				randmu.Unlock()
			}
			continue
		}
		if err == nil {
			name = try
		}
		break
	}
	return
}

// TempFile creates a temp file in the virtual fs
// Took from afero.FS code and adapted
func TempFile(fs FS, dir, pattern string) (f *os.File, err error) {
	if dir == "" {
		dir = os.TempDir()
	}

	var prefix, suffix string
	if pos := strings.LastIndex(pattern, "*"); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:]
	} else {
		prefix = pattern
	}

	nconflict := 0
	for i := 0; i < 10000; i++ {
		name := filepath.Join(dir, prefix+nextRandom()+suffix)
		f, err = fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			if nconflict++; nconflict > 10 {
				randmu.Lock()
				randSeed = reseed()
				randmu.Unlock()
			}
			continue
		}
		break
	}
	return
}

// Walkdir with an FS implementation
type statDirEntry struct {
	info fs.FileInfo
}

func (d *statDirEntry) Name() string               { return d.info.Name() }
func (d *statDirEntry) IsDir() bool                { return d.info.IsDir() }
func (d *statDirEntry) Type() fs.FileMode          { return d.info.Mode().Type() }
func (d *statDirEntry) Info() (fs.FileInfo, error) { return d.info, nil }

// WalkDirFs is the same as filepath.WalkDir but accepts a types.Fs so it can be run on any types.Fs type
func WalkDirFs(fs FS, root string, fn fs.WalkDirFunc) error {
	info, err := fs.Stat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(fs, root, &statDirEntry{info}, fn)
	}
	if err == filepath.SkipDir {
		return nil
	}
	return err
}

func walkDir(fs FS, path string, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	if err := walkDirFn(path, d, nil); err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}

	dirs, err := readDir(fs, path)
	if err != nil {
		// Second call, to report ReadDir error.
		err = walkDirFn(path, d, err)
		if err != nil {
			return err
		}
	}

	for _, d1 := range dirs {
		path1 := filepath.Join(path, d1.Name())
		if err := walkDir(fs, path1, d1, walkDirFn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

func readDir(vfs FS, dirname string) ([]fs.DirEntry, error) {
	dirs, err := vfs.ReadDir(dirname)
	if err != nil {
		return nil, err
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	return dirs, nil
}
