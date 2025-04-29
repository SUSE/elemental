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

package archive

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type cancelableReader struct {
	ctx context.Context
	src io.Reader
}

func (r *cancelableReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, fmt.Errorf("stop reading, context cancelled")
	default:
		return r.src.Read(p)
	}
}

func newCancelableReader(ctx context.Context, src io.Reader) *cancelableReader {
	return &cancelableReader{
		ctx: ctx,
		src: src,
	}
}

type link struct {
	Name string
	Path string
}

// ExtractTarball extracts a .tar, .tar.gz or .tar.bz2 taball file to the given target.
// Compression detection is rudimentary and only based on file name extension.
func ExtractTarball(ctx context.Context, s *sys.System, tarball string, target string) error {
	sourceFile, err := s.FS().OpenFile(tarball, os.O_RDONLY, vfs.FilePerm)
	if err != nil {
		return err
	}
	switch {
	case strings.HasSuffix(tarball, "tar.bz2"):
		return ExtractTarBz2(ctx, s, sourceFile, target)
	case strings.HasSuffix(tarball, "tar.gz"):
		return ExtractTarGz(ctx, s, sourceFile, target)
	default:
		return ExtractTar(ctx, s, sourceFile, target)
	}
}

// ExtractTarGz extracts a ..tar.gz archived stream of data to the given target
func ExtractTarGz(ctx context.Context, s *sys.System, body io.Reader, target string) error {
	reader, err := gzip.NewReader(body)
	if err != nil {
		return fmt.Errorf("gzip error: %w", err)
	}

	return ExtractTar(ctx, s, reader, target)
}

// ExtractTarBz2 extracts a ..tar.bz2 archived stream of data to the given target
func ExtractTarBz2(ctx context.Context, s *sys.System, body io.Reader, target string) error {
	reader := bzip2.NewReader(body)
	return ExtractTar(ctx, s, reader, target)
}

// ExtractTar extracts a .tar archived stream of data to the given target
func ExtractTar(ctx context.Context, s *sys.System, body io.Reader, target string) error {
	links := []*link{}
	symlinks := []*link{}

	tr := tar.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("stop reading tar, context cancelled")
		default:
		}

		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil && !errors.Is(err, tar.ErrInsecurePath) {
			s.Logger().Error("failed reading tar stream: %v", err)
			return err
		}

		path := header.Name
		if errors.Is(err, tar.ErrInsecurePath) {
			s.Logger().Warn("Ignoring non local path '%s': %v", path, err)
			continue
		}

		path, err = sanitizeArchivePath(target, path)
		if err != nil {
			s.Logger().Warn("Ignoring non local path '%s': %v", path, err)
			continue
		}

		info := header.FileInfo()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := vfs.MkdirAll(s.FS(), path, info.Mode()); err != nil {
				s.Logger().Error("failed creating directory from tar: %v", err)
				return err
			}
		case tar.TypeReg:
			if err := copyFile(ctx, s, path, info.Mode(), tr); err != nil {
				s.Logger().Error("failed creating file file %s: %v", path, err)
				return err
			}
		case tar.TypeLink:
			name := header.Linkname
			name, err = sanitizeArchivePath(target, name)
			if err != nil {
				s.Logger().Warn("Ignoring non local path '%s': %v", name, err)
				continue
			}
			links = append(links, &link{Path: path, Name: name})
		case tar.TypeSymlink:
			symlinks = append(symlinks, &link{Path: path, Name: header.Linkname})
		}
	}

	for _, link := range links {
		_ = s.FS().Remove(link.Path)
		if err := s.FS().Link(link.Name, link.Path); err != nil {
			s.Logger().Error("failed creating link %s: %v", link.Path, err)
			return err
		}
	}

	for _, symlink := range symlinks {
		_ = s.FS().Remove(symlink.Path)
		if err := s.FS().Symlink(symlink.Name, symlink.Path); err != nil {
			s.Logger().Error("failed creating link %s: %v", symlink.Path, err)
			return err
		}
	}

	return nil
}

func copyFile(ctx context.Context, s *sys.System, path string, mode os.FileMode, src io.Reader) (err error) {
	dir := filepath.Dir(path)
	info, err := s.FS().Lstat(dir)
	switch {
	case os.IsNotExist(err):
		err = vfs.MkdirAll(s.FS(), dir, vfs.DirPerm)
		if err != nil {
			return err
		}
	case err != nil:
		return err
	case info.Mode()&0200 == 0:
		// Ensure we can feed directories tared without write permission
		err = s.FS().Chmod(dir, vfs.DirPerm)
		if err != nil {
			return err
		}
		defer func() {
			e := s.FS().Chmod(dir, info.Mode())
			if err == nil && e != nil {
				err = e
			}
		}()
	}

	_ = s.FS().Remove(path)

	file, err := s.FS().OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		e := file.Close()
		if err == nil && e != nil {
			err = e
		}
	}()
	_, err = io.Copy(file, newCancelableReader(ctx, src))
	return err
}

func sanitizeArchivePath(root, filename string) (string, error) {
	path := filepath.Join(root, filename)
	if strings.HasPrefix(path, filepath.Clean(root)) {
		return path, nil
	}

	return path, fmt.Errorf("%s: %s", "content filepath is tainted", filename)
}
