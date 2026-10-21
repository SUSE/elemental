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

package helm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	helmBinary = "helm"
	ociPrefix  = "oci://"
)

type PullOptions struct {
	Repository string
	Chart      string
	Version    string

	Username              string
	Password              string
	InsecureSkipTLSVerify bool
}

type Puller struct {
	s *sys.System
}

func NewPuller(s *sys.System) *Puller {
	return &Puller{s: s}
}

func (p *Puller) Pull(ctx context.Context, opts PullOptions, destDir string) (string, error) {
	rawDestDir, err := p.s.FS().RawPath(destDir)
	if err != nil {
		return "", fmt.Errorf("resolving chart destination directory: %w", err)
	}

	args := []string{"pull", "--version", opts.Version, "--destination", rawDestDir}

	if strings.HasPrefix(opts.Repository, ociPrefix) {
		args = append(args, strings.TrimSuffix(opts.Repository, "/")+"/"+opts.Chart)
	} else {
		args = append(args, opts.Chart, "--repo", opts.Repository)
	}

	if opts.Username != "" {
		args = append(args, "--username", opts.Username, "--password", opts.Password)
	}

	if opts.InsecureSkipTLSVerify {
		args = append(args, "--insecure-skip-tls-verify")
	}

	if out, err := p.s.Runner().RunContext(ctx, helmBinary, args...); err != nil {
		return "", fmt.Errorf("pulling chart %s version %s: %w: %s", opts.Chart, opts.Version, err, strings.TrimSpace(string(out)))
	}

	archive, err := vfs.FindFile(p.s.FS(), destDir, fmt.Sprintf("%s-*.tgz", opts.Chart))
	if err != nil {
		return "", fmt.Errorf("locating pulled chart %s archive: %w", opts.Chart, err)
	}

	return archive, nil
}

func (p *Puller) Template(ctx context.Context, name, archive, namespace, kubeVersion string, apiVersions []string, values []byte) (rendered []byte, err error) {
	fs := p.s.FS()

	archivePath, err := fs.RawPath(archive)
	if err != nil {
		return nil, fmt.Errorf("resolving chart archive path: %w", err)
	}

	args := []string{"template", "--skip-crds", "--skip-tests", name, archivePath}

	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	if kubeVersion != "" {
		args = append(args, "--kube-version", kubeVersion)
	}

	if len(apiVersions) > 0 {
		args = append(args, "--api-versions", strings.Join(apiVersions, ","))
	}

	if len(values) > 0 {
		valuesFile, err := vfs.TempFile(fs, "", "helm-values-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("creating values file: %w", err)
		}
		defer func() {
			err = errors.Join(err, fs.Remove(valuesFile.Name()))
		}()

		if _, err = valuesFile.Write(values); err != nil {
			_ = valuesFile.Close()
			return nil, fmt.Errorf("writing values file: %w", err)
		}

		if err = valuesFile.Close(); err != nil {
			return nil, fmt.Errorf("closing values file: %w", err)
		}

		valuesPath, err := fs.RawPath(valuesFile.Name())
		if err != nil {
			return nil, fmt.Errorf("resolving values file path: %w", err)
		}

		args = append(args, "--values", valuesPath)
	}

	rendered, err = p.s.Runner().RunContext(ctx, helmBinary, args...)
	if err != nil {
		return nil, fmt.Errorf("templating chart %s: %w: %s", name, err, strings.TrimSpace(string(rendered)))
	}

	return rendered, nil
}
