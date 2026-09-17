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

package hauler

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const Binary = "hauler"

type Hauler struct {
	s      *sys.System
	binary string
}

type Opt func(h *Hauler)

func WithBinary(path string) Opt {
	return func(h *Hauler) {
		h.binary = path
	}
}

func New(s *sys.System, opts ...Opt) *Hauler {
	h := &Hauler{s: s, binary: Binary}

	for _, o := range opts {
		o(h)
	}

	return h
}

func (h *Hauler) BinaryPath() (string, error) {
	path, err := exec.LookPath(h.binary)
	if err != nil {
		return "", fmt.Errorf("locating hauler executable: %w", err)
	}

	return path, nil
}

func (h *Hauler) Login(ctx context.Context, registry, username, password string) error {
	out, err := h.s.Runner().RunContext(ctx, h.binary, "login", registry, "--username", username, "--password", password)
	if err != nil {
		return fmt.Errorf("logging into registry %s: %w: %s", registry, err, strings.TrimSpace(string(out)))
	}

	return nil
}

func (h *Hauler) SaveImage(ctx context.Context, image, platform, archive string) (err error) {
	fs := h.s.FS()

	store, err := vfs.TempDir(fs, "", "hauler-store-")
	if err != nil {
		return fmt.Errorf("creating hauler store directory: %w", err)
	}
	defer func() {
		err = errors.Join(err, fs.RemoveAll(store))
	}()

	storePath, err := fs.RawPath(store)
	if err != nil {
		return fmt.Errorf("resolving hauler store directory: %w", err)
	}

	archivePath, err := fs.RawPath(archive)
	if err != nil {
		return fmt.Errorf("resolving hauler archive path: %w", err)
	}

	out, err := h.s.Runner().RunContext(ctx, h.binary, "store", "add", "image", image, "--platform", platform, "--store", storePath)
	if err != nil {
		return fmt.Errorf("adding image %s to hauler store: %w: %s", image, err, strings.TrimSpace(string(out)))
	}

	out, err = h.s.Runner().RunContext(ctx, h.binary, "store", "save", "--filename", archivePath, "--store", storePath)
	if err != nil {
		return fmt.Errorf("saving hauler store for image %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}

	return nil
}
