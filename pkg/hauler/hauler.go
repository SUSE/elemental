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
	"bytes"
	"container/ring"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/platform"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	HaulerBinary      = "hauler"
	HaulerDir         = "/usr/lib/elemental/hauler"
	registryListening = "listening on"
	outputLines       = 10
)

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
	h := &Hauler{s: s, binary: HaulerBinary}

	for _, o := range opts {
		o(h)
	}

	return h
}

func (h *Hauler) HaulerPath(platformRef string) (string, error) {
	p, err := platform.Parse(platformRef)
	if err != nil {
		return "", fmt.Errorf("parsing platform %q: %w", platformRef, err)
	}

	bundled := filepath.Join(HaulerDir, p.GolangArch, HaulerBinary)
	exists, err := vfs.Exists(h.s.FS(), bundled)
	if err != nil {
		return "", fmt.Errorf("checking bundled hauler binary '%s': %w", bundled, err)
	}
	if exists {
		return bundled, nil
	}

	if host := h.s.Platform().String(); p.String() != host {
		return "", fmt.Errorf("no hauler binary available for platform %s on a %s host", p.String(), host)
	}

	path, err := exec.LookPath(h.binary)
	if err != nil {
		return "", fmt.Errorf("locating hauler executable: %w", err)
	}

	return path, nil
}

func (h *Hauler) Login(ctx context.Context, registry, username, password string) error {
	var out bytes.Buffer

	writePassword := func(stdin io.Writer) error {
		_, err := io.WriteString(stdin, password)
		return err
	}

	err := h.s.Runner().RunContextWithPipe(ctx, writePassword, &out, &out, "", nil,
		h.binary, "login", registry, "--username", username, "--password-stdin")
	if err != nil {
		return fmt.Errorf("logging into registry %s: %w: %s", registry, err, strings.TrimSpace(out.String()))
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

	args := []string{"store", "add", "image", image}
	if !strings.Contains(image, "@") {
		args = append(args, "--platform", platform)
	}
	args = append(args, "--store", storePath)

	out, err := h.s.Runner().RunContext(ctx, h.binary, args...)
	if err != nil {
		return fmt.Errorf("adding image %s to hauler store: %w: %s", image, err, strings.TrimSpace(string(out)))
	}

	out, err = h.s.Runner().RunContext(ctx, h.binary, "store", "save", "--filename", archivePath, "--store", storePath)
	if err != nil {
		return fmt.Errorf("saving hauler store for image %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}

	return nil
}

func (h *Hauler) LoadStore(ctx context.Context, store string, archives ...string) error {
	fs := h.s.FS()

	storePath, err := fs.RawPath(store)
	if err != nil {
		return fmt.Errorf("resolving hauler store path: %w", err)
	}

	args := []string{"store", "load", "--store", storePath, "--tempdir", filepath.Dir(storePath)}
	for _, archive := range archives {
		archivePath, err := fs.RawPath(archive)
		if err != nil {
			return fmt.Errorf("resolving hauler archive path: %w", err)
		}

		args = append(args, "--filename", archivePath)
	}

	out, err := h.s.Runner().RunContext(ctx, h.binary, args...)
	if err != nil {
		return fmt.Errorf("loading archives into hauler store: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func (h *Hauler) ExportRegistry(ctx context.Context, store, dir string) error {
	fs := h.s.FS()

	storePath, err := fs.RawPath(store)
	if err != nil {
		return fmt.Errorf("resolving hauler store path: %w", err)
	}

	dirPath, err := fs.RawPath(dir)
	if err != nil {
		return fmt.Errorf("resolving registry directory path: %w", err)
	}

	ctx, stopRegistry := context.WithCancel(ctx)
	defer stopRegistry()

	stdOut := ring.New(outputLines)
	stdErr := ring.New(outputLines)
	exported := false

	onHaulerOutput := func(line string) {
		stdOut.Value = line
		stdOut = stdOut.Next()
	}

	onRegistryOutput := func(line string) {
		stdErr.Value = line
		stdErr = stdErr.Next()
		if strings.Contains(line, registryListening) {
			exported = true
			stopRegistry()
		}
	}

	err = h.s.Runner().RunContextParseOutput(ctx, onHaulerOutput, onRegistryOutput, h.binary,
		"store", "serve", "registry", "--store", storePath, "--directory", dirPath, "--port", "0")
	if exported {
		return nil
	}

	logOutput(h.s, stdOut, stdErr)
	return fmt.Errorf("exporting hauler store to registry: %w", err)
}

func logOutput(s *sys.System, stdOut, stdErr *ring.Ring) {
	output := "\n------- stdOut -------\n"
	stdOut.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	output += "------- stdErr -------\n"
	stdErr.Do(func(v any) {
		if v != nil {
			output += v.(string) + "\n"
		}
	})
	s.Logger().Warn("Hauler registry output: %s", output)
}

func (h *Hauler) WriteEmptyStore(dir string) error {
	fs := h.s.FS()

	if err := vfs.MkdirAll(fs, dir, vfs.DirPerm); err != nil {
		return fmt.Errorf("creating hauler store directory: %w", err)
	}

	files := map[string]string{
		"index.json": `{"schemaVersion":2,"manifests":[]}`,
		"oci-layout": `{"imageLayoutVersion":"1.0.0"}`,
	}
	for name, content := range files {
		if err := fs.WriteFile(filepath.Join(dir, name), []byte(content), vfs.FilePerm); err != nil {
			return fmt.Errorf("writing hauler store file %s: %w", name, err)
		}
	}

	return nil
}
