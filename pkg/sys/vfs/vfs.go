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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	gvfs "github.com/twpayne/go-vfs/v4"
)

const (
	DirPerm        = os.ModeDir | os.ModePerm
	FilePerm       = 0666
	NoWriteDirPerm = 0555 | os.ModeDir
	TempDirPerm    = os.ModePerm | os.ModeSticky | os.ModeDir

	// MaxLinkDepth is a maximum number of nested symlinks to resolve
	MaxLinkDepth = 4
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

func New() FS {
	return gvfs.OSFS
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

// ForceRemoveAll removes the specified path.
// If it fails to remove somepaths it tries set the write permission
// to every file or directory and run a removal again
func ForceRemoveAll(vfs FS, path string) error {
	err := vfs.RemoveAll(path)
	if err == nil {
		return nil
	}

	var errs error
	_ = WalkDirFs(vfs, path, func(path string, d fs.DirEntry, err error) error {
		errs = errors.Join(errs, err)

		info, err := d.Info()
		if err != nil {
			return err
		}
		err = vfs.Chmod(path, info.Mode()|0200)
		if err != nil {
			return err
		}
		return nil
	})
	return errors.Join(errs, vfs.RemoveAll(path))
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

// resolveLink attempts to resolve a symlink, if any. Returns the original given path
// if not a symlink. In case of error returns error and the original given path.
func resolveLink(vfs FS, path string, rootDir string, d fs.DirEntry, depth int) (string, error) {
	var err error
	var resolved string
	var f fs.FileInfo

	f, err = d.Info()
	if err != nil {
		return path, err
	}

	if f.Mode()&os.ModeSymlink == os.ModeSymlink {
		if depth <= 0 {
			return path, fmt.Errorf("can't resolve this path '%s', too many nested links", path)
		}
		resolved, err = ReadLink(vfs, path)
		if err == nil {
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			} else {
				resolved = filepath.Join(rootDir, resolved)
			}
			if f, err = vfs.Lstat(resolved); err == nil {
				return resolveLink(vfs, resolved, rootDir, &statDirEntry{f}, depth-1)
			}
			return path, err
		}
		return path, err
	}
	return path, nil
}

// ResolveLink attempts to resolve a symlink, if any. Returns the original given path
// if not a symlink or if it can't be resolved.
func ResolveLink(vfs FS, path string, rootDir string, depth int) (string, error) {
	f, err := vfs.Lstat(path)
	if err != nil {
		return path, err
	}

	return resolveLink(vfs, path, rootDir, &statDirEntry{f}, depth)
}

// FindFile attempts to find a file from a list of patterns on top of a given root path.
// Returns first match if any and returns error otherwise.
func FindFile(vfs FS, rootDir string, patterns ...string) (string, error) {
	var err error
	var found string

	for _, pattern := range patterns {
		found, err = findFile(vfs, rootDir, pattern)
		if err != nil {
			return "", err
		} else if found != "" {
			break
		}
	}
	if found == "" {
		return "", fmt.Errorf("failed to find file matching %v in %v", patterns, rootDir)
	}
	return found, nil
}

// FindFiles attempts to find files from a given pattern on top of a root path.
// Returns empty list if no files are found.
func FindFiles(vfs FS, rootDir string, pattern string) ([]string, error) {
	return findFiles(vfs, rootDir, pattern, false)
}

// findFile attempts to find a file from a given pattern on top of a root path.
// Returns empty path if no file is found.
func findFile(vfs FS, rootDir, pattern string) (string, error) {
	files, err := findFiles(vfs, rootDir, pattern, true)
	if err != nil {
		return "", err
	}

	if len(files) > 0 {
		return files[0], nil
	}
	return "", nil
}

func findFiles(vfs FS, rootDir, pattern string, firstMatchReturn bool) ([]string, error) {
	foundFiles := []string{}

	base := filepath.Join(rootDir, getBaseDir(pattern))
	if ok, _ := Exists(vfs, base); ok {
		err := WalkDirFs(vfs, base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			match, err := filepath.Match(filepath.Join(rootDir, pattern), path)
			if err != nil {
				return err
			}
			if match {
				foundFile, err := resolveLink(vfs, path, rootDir, d, MaxLinkDepth)
				if err != nil {
					return err
				}
				foundFiles = append(foundFiles, foundFile)
				if firstMatchReturn {
					return io.EOF
				}
				return nil
			}
			return nil
		})
		if err != nil && !errors.Is(err, io.EOF) {
			return []string{}, err
		}
	}
	return foundFiles, nil
}

// getBaseDir returns the base directory of a shell path pattern
func getBaseDir(path string) string {
	magicChars := `*?[`
	i := strings.IndexAny(path, magicChars)
	if i > 0 {
		return filepath.Dir(path[:i])
	} else if i == 0 {
		return ""
	}
	return path
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
	return uint32(time.Now().UnixNano() + int64(os.Getpid())) //nolint:gosec // disable G115
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

// TempDir creates a temporary directory in the virtual fs
// dir defines the parent directory to create into, if emtpy it relies on
// the OS default TMP directory. The prefix is used to name new temporary directory.
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
	if errors.Is(err, filepath.SkipDir) {
		return nil
	}
	return err
}

func walkDir(fs FS, path string, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	if err := walkDirFn(path, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, filepath.SkipDir) && d.IsDir() {
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
			if errors.Is(err, filepath.SkipDir) {
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

// CopyFile copies source file to a target file using the FS interface. If the target
// is a directory, the source is copied into that directory using a source name file.
// File mode is preserved.
func CopyFile(fs FS, source string, target string) error {
	return ConcatFiles(fs, []string{source}, target)
}

// ConcatFiles Copies source files to target file using Fs interface.
// Source files are concatenated into target file in the given order.
// If target is a directory source is copied into that directory using
// 1st source name file. The result keeps the file mode of the 1st source.
func ConcatFiles(fs FS, sources []string, target string) (err error) {
	if len(sources) == 0 {
		return fmt.Errorf("empty sources list")
	}
	if dir, _ := IsDir(fs, target); dir {
		target = filepath.Join(target, filepath.Base(sources[0]))
	}
	fInf, err := fs.Stat(sources[0])
	if err != nil {
		return err
	}

	targetFile, err := fs.Create(target)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			err = targetFile.Close()
		} else {
			_ = fs.Remove(target)
		}
	}()

	var sourceFile *os.File
	for _, source := range sources {
		sourceFile, err = fs.OpenFile(source, os.O_RDONLY, FilePerm)
		if err != nil {
			return err
		}
		_, err = io.Copy(targetFile, sourceFile)
		if err != nil {
			return err
		}
		err = sourceFile.Close()
		if err != nil {
			return err
		}
	}

	return fs.Chmod(target, fInf.Mode())
}

// LoadEnvFile will try to parse the file given and return a map with the key/values
func LoadEnvFile(fs FS, file string) (map[string]string, error) {
	var envMap map[string]string
	var err error

	f, err := fs.Open(file)
	if err != nil {
		return envMap, err
	}
	defer f.Close()

	envMap, err = godotenv.Parse(f)
	if err != nil {
		return envMap, err
	}

	return envMap, err
}

// WriteEnvFile will write the given environment file with the given key/values
func WriteEnvFile(fs FS, envs map[string]string, filename string) error {
	var bkFile string

	rawPath, err := fs.RawPath(filename)
	if err != nil {
		return err
	}

	if ok, _ := Exists(fs, filename, true); ok {
		bkFile = filename + ".bk"
		err = fs.Rename(filename, bkFile)
		if err != nil {
			return err
		}
	}

	err = godotenv.Write(envs, rawPath)
	if err != nil {
		if bkFile != "" {
			// try to restore renamed file
			_ = fs.Rename(bkFile, filename)
		}
		return err
	}
	if bkFile != "" {
		_ = fs.Remove(bkFile)
	}
	return nil
}

// FindKernel finds for kernel files inside a given root tree path.
// Returns kernel file and version. It assumes kernel files match certain patterns
func FindKernel(fs FS, rootDir string) (string, string, error) {
	var kernel, version string

	kernelPatterns := []string{
		"/usr/lib/modules/*/uImage*",
		"/usr/lib/modules/*/Image*",
		"/usr/lib/modules/*/zImage*",
		"/usr/lib/modules/*/vmlinuz*",
		"/usr/lib/modules/*/image*",
	}

	kernels := []string{}

	for _, pattern := range kernelPatterns {
		files, err := findFiles(fs, rootDir, pattern, false)
		if err != nil {
			return kernel, version, err
		}

		kernels = slices.Concat(kernels, files)
	}

	if len(kernels) == 0 {
		return kernel, version, fmt.Errorf("no kernels found")
	}

	// Sort descending
	sort.Slice(kernels, func(i, j int) bool {
		return kernels[i] > kernels[j]
	})

	kernel = kernels[0]
	version = filepath.Base(filepath.Dir(kernel))

	if version == "" {
		return "", "", fmt.Errorf("could not determine the version of kernel %s", kernel)
	}
	return kernel, version, nil
}

// FindKernelHmac returns the path to an existing kernel .hmac file
func FindKernelHmac(fs FS, kernel string) (string, error) {
	filename := fmt.Sprintf(".%s.hmac", filepath.Base(kernel))
	hmac := filepath.Join(filepath.Dir(kernel), filename)

	if exists, _ := Exists(fs, hmac); exists {
		return hmac, nil
	}

	return "", fmt.Errorf("%w: %s", os.ErrNotExist, hmac)
}
