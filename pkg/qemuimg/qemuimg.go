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

// Package qemuimg wraps the qemu-img tool to convert raw disk images into
// sparse qcow2 images and to create thin qcow2 overlays on top of them.
package qemuimg

import (
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	qemuImg = "qemu-img"

	FormatRaw   = "raw"
	FormatQCOW2 = "qcow2"
)

// ConvertToQCOW2 converts the raw image src into a sparse qcow2 image at dst.
// All-zero clusters of src are not allocated in dst, so the resulting file only
// holds the data actually written in the raw image.
func ConvertToQCOW2(s *sys.System, src, dst string) error {
	if ok, _ := vfs.Exists(s.FS(), dst); ok {
		return fmt.Errorf("target image '%s' already exists", dst)
	}

	s.Logger().Info("Converting '%s' to sparse qcow2 image '%s'", src, dst)
	_, err := s.Runner().Run(qemuImg, "convert", "-f", FormatRaw, "-O", FormatQCOW2, src, dst)
	if err != nil {
		return fmt.Errorf("converting '%s' to qcow2: %w", src, err)
	}
	return nil
}

// CreateOverlay creates an empty qcow2 image at overlay backed by the qcow2 image base.
// The overlay initially stores no data and only holds the writes done to it afterwards.
// The base path is recorded relative to the directory of overlay, so both images can be
// moved together (e.g. out of a container volume) without breaking the reference.
func CreateOverlay(s *sys.System, base, overlay string) error {
	if ok, _ := vfs.Exists(s.FS(), base); !ok {
		return fmt.Errorf("base image '%s' does not exist", base)
	}
	if ok, _ := vfs.Exists(s.FS(), overlay); ok {
		return fmt.Errorf("overlay image '%s' already exists", overlay)
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("resolving absolute path of '%s': %w", base, err)
	}
	absOverlay, err := filepath.Abs(overlay)
	if err != nil {
		return fmt.Errorf("resolving absolute path of '%s': %w", overlay, err)
	}
	if absBase == absOverlay {
		return fmt.Errorf("overlay image cannot be the same file as its base image")
	}

	relBase, err := filepath.Rel(filepath.Dir(absOverlay), absBase)
	if err != nil {
		return fmt.Errorf("computing base image path relative to '%s': %w", absOverlay, err)
	}

	// A relative backing path is interpreted by qemu relative to the directory of the overlay.
	s.Logger().Info("Creating qcow2 overlay '%s' backed by '%s'", overlay, relBase)
	_, err = s.Runner().Run(qemuImg, "create", "-f", FormatQCOW2, "-b", relBase, "-F", FormatQCOW2, absOverlay)
	if err != nil {
		return fmt.Errorf("creating qcow2 overlay '%s': %w", overlay, err)
	}
	return nil
}
