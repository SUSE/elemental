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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"go.yaml.in/yaml/v3"
)

const (
	helmBinary   = "helm"
	ociPrefix    = "oci://"
	authRepoName = "chart-repo"
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

func (p *Puller) Pull(ctx context.Context, opts PullOptions, destDir string) (archive string, err error) {
	fs := p.s.FS()

	rawDestDir, err := fs.RawPath(destDir)
	if err != nil {
		return "", fmt.Errorf("resolving chart destination directory: %w", err)
	}

	args := []string{"pull", "--version", opts.Version, "--destination", rawDestDir}
	if opts.InsecureSkipTLSVerify {
		args = append(args, "--insecure-skip-tls-verify")
	}

	switch {
	case opts.Username != "":
		var authDir string
		var authArgs []string

		if authDir, err = vfs.TempDir(fs, "", "helm-auth-"); err != nil {
			return "", fmt.Errorf("creating helm credentials directory: %w", err)
		}
		defer func() {
			err = errors.Join(err, fs.RemoveAll(authDir))
		}()

		if authArgs, err = p.authenticate(ctx, opts, authDir); err != nil {
			return "", err
		}
		args = append(args, authArgs...)
	case strings.HasPrefix(opts.Repository, ociPrefix):
		args = append(args, strings.TrimSuffix(opts.Repository, "/")+"/"+opts.Chart)
	default:
		args = append(args, opts.Chart, "--repo", opts.Repository)
	}

	if out, err := p.s.Runner().RunContext(ctx, helmBinary, args...); err != nil {
		return "", fmt.Errorf("pulling chart %s version %s: %w: %s", opts.Chart, opts.Version, err, strings.TrimSpace(string(out)))
	}

	archive, err = vfs.FindFile(fs, destDir, fmt.Sprintf("%s-*.tgz", opts.Chart))
	if err != nil {
		return "", fmt.Errorf("locating pulled chart %s archive: %w", opts.Chart, err)
	}

	return archive, nil
}

func (p *Puller) authenticate(ctx context.Context, opts PullOptions, authDir string) ([]string, error) {
	fs := p.s.FS()

	if strings.HasPrefix(opts.Repository, ociPrefix) {
		host := strings.TrimPrefix(opts.Repository, ociPrefix)
		if u, err := url.Parse("https://" + host); err == nil {
			host = u.Host
		}

		auth := base64.StdEncoding.EncodeToString([]byte(opts.Username + ":" + opts.Password))
		config, err := json.Marshal(map[string]any{
			"auths": map[string]any{
				host: map[string]string{"username": opts.Username, "password": opts.Password, "auth": auth},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("encoding registry credentials: %w", err)
		}

		registryConfig := filepath.Join(authDir, "registry.json")
		if err = fs.WriteFile(registryConfig, config, 0o600); err != nil {
			return nil, fmt.Errorf("writing registry credentials: %w", err)
		}

		rawRegistryConfig, err := fs.RawPath(registryConfig)
		if err != nil {
			return nil, fmt.Errorf("resolving registry credentials path: %w", err)
		}

		return []string{strings.TrimSuffix(opts.Repository, "/") + "/" + opts.Chart, "--registry-config", rawRegistryConfig}, nil
	}

	repositories, err := yaml.Marshal(map[string]any{
		"repositories": []map[string]any{{
			"name":                     authRepoName,
			"url":                      opts.Repository,
			"username":                 opts.Username,
			"password":                 opts.Password,
			"insecure_skip_tls_verify": opts.InsecureSkipTLSVerify,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("encoding repository credentials: %w", err)
	}

	repositoryConfig := filepath.Join(authDir, "repositories.yaml")
	if err = fs.WriteFile(repositoryConfig, repositories, 0o600); err != nil {
		return nil, fmt.Errorf("writing repository credentials: %w", err)
	}

	rawAuthDir, err := fs.RawPath(authDir)
	if err != nil {
		return nil, fmt.Errorf("resolving repository credentials path: %w", err)
	}

	configArgs := []string{
		"--repository-config", filepath.Join(rawAuthDir, "repositories.yaml"),
		"--repository-cache", filepath.Join(rawAuthDir, "cache"),
	}

	updateArgs := append([]string{"repo", "update", authRepoName}, configArgs...)
	if out, err := p.s.Runner().RunContext(ctx, helmBinary, updateArgs...); err != nil {
		return nil, fmt.Errorf("updating repository %s index: %w: %s", opts.Repository, err, strings.TrimSpace(string(out)))
	}

	return append([]string{authRepoName + "/" + opts.Chart}, configArgs...), nil
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

	var stdout, stderr bytes.Buffer
	noInput := func(io.Writer) error { return nil }

	err = p.s.Runner().RunContextWithPipe(ctx, noInput, &stdout, &stderr, "", nil, helmBinary, args...)
	if err != nil {
		return nil, fmt.Errorf("templating chart %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}
