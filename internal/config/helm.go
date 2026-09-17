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

package config

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image/auth"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/release"
	"github.com/suse/elemental/v3/pkg/cache"
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type helmValuesResolver interface {
	Resolve(*helm.ValueSource) ([]byte, error)
}

type helmChart interface {
	GetName() string
	GetVersion() string
	GetAPIVersions() []string
	GetInlineValues() map[string]any
	GetRepositoryName() string
	ToCRD(values []byte, repository string, hasAuth, skipTLSVerify bool) (*helm.CRD, error)
}

type chartPuller interface {
	Pull(ctx context.Context, opts helm.PullOptions, destDir string) (string, error)
	Template(ctx context.Context, name, archive, namespace, kubeVersion string, apiVersions []string, values []byte) ([]byte, error)
}

type Helm struct {
	RelativePath   string
	ValuesResolver helmValuesResolver
	Logger         log.Logger

	FS        vfs.FS
	Puller    chartPuller
	Cache     *cache.Cache
	ChartsDir string
	images map[string]bool
}

func NewHelm(valuesResolver helmValuesResolver, logger log.Logger) *Helm {
	return &Helm{
		RelativePath:   image.HelmPath(),
		ValuesResolver: valuesResolver,
		Logger:         logger,
	}
}

func (h *Helm) Configure(ctx context.Context, conf *image.Configuration, rm *resolver.ResolvedManifest, butaneCfg *butane.Config) ([]string, error) {
	if len(conf.Release.Components.HelmCharts) > 0 {
		var charts []string
		for _, c := range conf.Release.Components.HelmCharts {
			charts = append(charts, c.Name)
		}

		h.Logger.Info("Enabling the following Helm components: %s", strings.Join(charts, ", "))
	}

	h.images = map[string]bool{}

	charts, secrets, err := h.retrieveHelmCharts(ctx, rm, conf)
	if err != nil {
		return nil, fmt.Errorf("retrieving helm charts: %w", err)
	}

	chartFiles, err := h.writeHelmCharts(butaneCfg, charts)
	if err != nil {
		return nil, fmt.Errorf("writing helm chart resources: %w", err)
	}

	err = h.writeHelmSecrets(secrets, butaneCfg)
	if err != nil {
		return nil, fmt.Errorf("creating helm secrets: %w", err)
	}

	return chartFiles, nil
}

func (h *Helm) writeHelmCharts(butaneCfg *butane.Config, crds []*helm.CRD) ([]string, error) {
	var charts []string

	for _, crd := range crds {
		data, err := yaml.Marshal(crd)
		if err != nil {
			return nil, fmt.Errorf("marshaling helm chart %s: %w", crd.Metadata.Name, err)
		}

		chartName := fmt.Sprintf("%s.yaml", crd.Metadata.Name)
		relativePath := filepath.Join("/", h.RelativePath, chartName)

		butaneCfg.AddFileInline(relativePath, new(string(data)), 0o644)

		charts = append(charts, relativePath)
	}

	return charts, nil
}

func (h *Helm) writeHelmSecrets(secrets []*helm.Secret, butaneCfg *butane.Config) error {

	for _, secret := range secrets {
		data, err := yaml.Marshal(secret)
		if err != nil {
			return fmt.Errorf("marshaling secret %s: %w", secret.Metadata.Name, err)
		}

		secretName := fmt.Sprintf("%s-priority.yaml", secret.Metadata.Name)
		butaneCfg.AddFileInline(filepath.Join("/", image.KubernetesManifestsPath(), secretName), new(string(data)), 0o644)
	}

	return nil
}

// ContainerImages deduplicates the discovered container images
func (h *Helm) ContainerImages() []string {
	return slices.Sorted(maps.Keys(h.images))
}

func (h *Helm) retrieveHelmCharts(ctx context.Context, rm *resolver.ResolvedManifest, conf *image.Configuration) ([]*helm.CRD, []*helm.Secret, error) {
	var crds []*helm.CRD
	embed := conf.Kubernetes.OCIRegistry != nil
	kubeVersion := kubernetesVersion(rm)

	charts, repositories, err := enabledHelmCharts(rm, conf.Release.Components.HelmCharts, h.Logger)
	if err != nil {
		return nil, nil, fmt.Errorf("filtering enabled helm charts: %w", err)
	}

	valueFiles := conf.Release.Components.HelmValueFiles()

	authMap, err := createAuthMap(charts, repositories, conf)
	if err != nil {
		return nil, nil, fmt.Errorf("creating helm chart auth map: %w", err)
	}

	for _, chart := range charts {
		if err = h.appendHelmChart(ctx, chart, repositories, valueFiles, &crds, authMap[chart.Chart], embed, kubeVersion); err != nil {
			return nil, nil, fmt.Errorf("collecting helm charts: %w", err)
		}
	}

	if conf.Kubernetes.Helm != nil {
		repositories = conf.Kubernetes.Helm.ChartRepositories()
		valueFiles = conf.Kubernetes.Helm.ValueFiles()

		for _, chart := range conf.Kubernetes.Helm.Charts {
			if err = h.appendHelmChart(ctx, chart, repositories, valueFiles, &crds, authMap[chart.Name], embed, kubeVersion); err != nil {
				return nil, nil, fmt.Errorf("collecting user helm charts: %w", err)
			}
		}
	}

	return crds, generateHelmSecrets(authMap), nil
}

func createAuthMap(charts []*api.HelmChart, repositories map[string]string, conf *image.Configuration) (map[string]*auth.HelmAuth, error) {
	authMap := make(map[string]*auth.HelmAuth)
	if conf.Release.Components.HelmCharts != nil {
		releaseChartsMap := make(map[string]*api.HelmChart, len(charts))
		for _, c := range charts {
			releaseChartsMap[c.Chart] = c
		}

		for _, rc := range conf.Release.Components.HelmCharts {
			c, ok := releaseChartsMap[rc.Name]
			if !ok || rc.Credentials == nil {
				continue
			}

			repoURL := repositories[c.Repository]
			extractedHost, err := extractHost(repoURL)
			if err != nil {
				return nil, fmt.Errorf("extracting host: %w", err)
			}
			authMap[c.Chart] = &auth.HelmAuth{
				RawURL: repoURL,
				URL:    extractedHost,
				Credentials: auth.Credentials{
					Username: rc.Credentials.Username,
					Password: rc.Credentials.Password,
				},
			}
		}
	}

	if conf.Kubernetes.Helm != nil {
		reposByName := make(map[string]*kubernetes.HelmRepository, len(conf.Kubernetes.Helm.Repositories))
		for _, r := range conf.Kubernetes.Helm.Repositories {
			if _, exists := reposByName[r.Name]; exists {
				return nil, fmt.Errorf("helm repository '%s' defined multiple times", r.Name)
			}
			reposByName[r.Name] = r
		}
		for _, c := range conf.Kubernetes.Helm.Charts {
			r, ok := reposByName[c.RepositoryName]
			if !ok || r.Credentials == nil {
				continue
			}

			repoURL := r.URL
			extractedHost, err := extractHost(repoURL)
			if err != nil {
				return nil, fmt.Errorf("extracting host: %w", err)
			}
			authMap[c.Name] = &auth.HelmAuth{
				RawURL: repoURL,
				URL:    extractedHost,
				Credentials: auth.Credentials{
					Username: r.Credentials.Username,
					Password: r.Credentials.Password,
				},
				InsecureSkipTLSVerify: r.InsecureSkipTLSVerify,
			}
		}
	}

	return authMap, nil
}

func generateHelmSecrets(authMap map[string]*auth.HelmAuth) []*helm.Secret {
	var secrets []*helm.Secret

	for chart, creds := range authMap {
		secrets = append(secrets, NewSecret(chart, creds))
	}

	return secrets
}

func (h *Helm) appendHelmChart(ctx context.Context, chart helmChart, repositories, valueFiles map[string]string, crds *[]*helm.CRD, a *auth.HelmAuth, embed bool, kubeVersion string) error {
	name := chart.GetName()
	repository, ok := repositories[chart.GetRepositoryName()]
	if !ok {
		return fmt.Errorf("repository not found for chart: %s", name)
	}

	source := &helm.ValueSource{Inline: chart.GetInlineValues(), File: valueFiles[name]}
	values, err := h.ValuesResolver.Resolve(source)
	if err != nil {
		return fmt.Errorf("resolving values for chart %s: %w", name, err)
	}

	needsAuth := a != nil
	skipTLSVerify := needsAuth && a.InsecureSkipTLSVerify
	crd, err := chart.ToCRD(values, repository, needsAuth, skipTLSVerify)
	if err != nil {
		return fmt.Errorf("constructing HelmChart custom resource: %w", err)
	}

	if embed {
		content, archive, err := h.chartContent(ctx, chart, repository, a)
		if err != nil {
			return fmt.Errorf("embedding chart %s: %w", name, err)
		}

		crd.Spec.Chart = name
		crd.Spec.Repo = ""
		crd.Spec.ChartContent = content

		if err = h.collectChartImages(ctx, name, archive, crd.Spec.TargetNamespace, kubeVersion, chart.GetAPIVersions(), values); err != nil {
			return fmt.Errorf("discovering images of chart %s: %w", name, err)
		}
	}

	*crds = append(*crds, crd)

	return nil
}

// collectChartImages templates the chart to collect the container images in it.
func (h *Helm) collectChartImages(ctx context.Context, name, archive, namespace, kubeVersion string, apiVersions []string, values []byte) error {
	h.Logger.Debug("Rendering Helm chart %s to discover its container images", name)

	rendered, err := h.Puller.Template(ctx, name, archive, namespace, kubeVersion, apiVersions, values)
	if err != nil {
		return err
	}

	resources, err := parseManifests(bytes.NewReader(rendered))
	if err != nil {
		return fmt.Errorf("parsing rendered chart: %w", err)
	}

	for _, resource := range resources {
		extractManifestImages(resource, h.images)
	}

	return nil
}

func kubernetesVersion(rm *resolver.ResolvedManifest) string {
	if rm == nil || rm.CorePlatform == nil || rm.CorePlatform.Components.Kubernetes == nil {
		return ""
	}

	version := strings.TrimPrefix(rm.CorePlatform.Components.Kubernetes.Version, "v")
	version, _, _ = strings.Cut(version, "+")
	version, _, _ = strings.Cut(version, "_")

	return version
}

func (h *Helm) chartContent(ctx context.Context, chart helmChart, repository string, a *auth.HelmAuth) (string, string, error) {
	if h.Puller == nil || h.Cache == nil || h.FS == nil || h.ChartsDir == "" {
		return "", "", fmt.Errorf("chart embedding not configured")
	}

	name, version := chart.GetName(), chart.GetVersion()
	archive := filepath.Join(h.ChartsDir, fmt.Sprintf("%s-%s.tgz", name, version))
	key := fmt.Sprintf("%s/%s:%s", repository, name, version)

	opts := helm.PullOptions{Repository: repository, Chart: name, Version: version}
	if a != nil {
		opts.Username = a.Credentials.Username
		opts.Password = a.Credentials.Password
		opts.InsecureSkipTLSVerify = a.InsecureSkipTLSVerify
	}

	if err := vfs.MkdirAll(h.FS, h.ChartsDir, vfs.DirPerm); err != nil {
		return "", "", fmt.Errorf("creating charts directory: %w", err)
	}

	h.Logger.Debug("Pulling Helm chart %s version %s", name, version)
	err := h.Cache.File(ctx, key, archive, func(ctx context.Context) (io.ReadCloser, error) {
		return h.pullChart(ctx, opts)
	})
	if err != nil {
		return "", "", err
	}

	data, err := h.FS.ReadFile(archive)
	if err != nil {
		return "", "", fmt.Errorf("reading chart archive: %w", err)
	}

	return base64.StdEncoding.EncodeToString(data), archive, nil
}

// pullChart downloads the chart into a temporary directory
func (h *Helm) pullChart(ctx context.Context, opts helm.PullOptions) (io.ReadCloser, error) {
	tempDir, err := vfs.TempDir(h.FS, "", "helm-pull-")
	if err != nil {
		return nil, fmt.Errorf("creating temporary chart directory: %w", err)
	}

	archive, err := h.Puller.Pull(ctx, opts, tempDir)
	if err != nil {
		_ = h.FS.RemoveAll(tempDir)
		return nil, err
	}

	file, err := h.FS.Open(archive)
	if err != nil {
		_ = h.FS.RemoveAll(tempDir)
		return nil, fmt.Errorf("opening pulled chart archive: %w", err)
	}

	return &tempFile{File: file, fs: h.FS, dir: tempDir}, nil
}

type tempFile struct {
	fs.File
	fs  vfs.FS
	dir string
}

func (t *tempFile) Close() error {
	return errors.Join(t.File.Close(), t.fs.RemoveAll(t.dir))
}

func enabledHelmCharts(rm *resolver.ResolvedManifest, enabled []release.HelmChart, logger log.Logger) ([]*api.HelmChart, map[string]string, error) {
	coreCharts, solutionCharts := map[string]*api.HelmChart{}, map[string]*api.HelmChart{}
	repositories := map[string]string{}

	if rm.CorePlatform.Components.Helm != nil {
		for _, c := range rm.CorePlatform.Components.Helm.Charts {
			coreCharts[c.Chart] = c
		}

		for _, repository := range rm.CorePlatform.Components.Helm.Repositories {
			repositories[repository.Name] = repository.URL
		}
	}

	if rm.SolutionExtension != nil && rm.SolutionExtension.Components.Helm != nil {
		for _, c := range rm.SolutionExtension.Components.Helm.Charts {
			solutionCharts[c.Chart] = c
		}

		for _, repository := range rm.SolutionExtension.Components.Helm.Repositories {
			repositories[repository.Name] = repository.URL
		}
	}

	var charts []*api.HelmChart
	var addChart func(name string) error

	// Add a chart and its direct dependencies, avoiding duplicates.
	// Prioritize charts from solution releases over core ones.
	addChart = func(name string) error {
		source := "solution"

		chart, ok := solutionCharts[name]
		if !ok {
			chart, ok = coreCharts[name]
			if !ok {
				return fmt.Errorf("helm chart does not exist")
			}
			source = "core"
		}

		if logger != nil {
			logger.Info("Using Helm chart %s from %s release", name, source)
		}

		if slices.ContainsFunc(charts, func(c *api.HelmChart) bool {
			return c.GetName() == name
		}) {
			return nil
		}

		// Check for dependencies and add them first.
		for _, d := range chart.DependsOn {
			if d.Type == api.DependencyTypeHelm {
				if err := addChart(d.Name); err != nil {
					return fmt.Errorf("adding dependent helm chart '%s': %w", d.Name, err)
				}
			}
		}

		// Add the main chart.
		charts = append(charts, chart)

		return nil
	}

	for _, e := range enabled {
		if err := addChart(e.Name); err != nil {
			return nil, nil, fmt.Errorf("adding helm chart '%s': %w", e.Name, err)
		}
	}

	return charts, repositories, nil
}

func NewSecret(name string, creds *auth.HelmAuth) *helm.Secret {
	secret := &helm.Secret{
		APIVersion: "v1",
		Kind:       "Secret",
		Metadata: helm.SecretMetadata{
			Name:      fmt.Sprintf("%s-auth", name),
			Namespace: "kube-system",
		},
	}

	if strings.HasPrefix(creds.RawURL, "oci://") {
		a := base64.StdEncoding.EncodeToString(
			[]byte(creds.Credentials.Username + ":" + creds.Credentials.Password))
		dockerConfig := fmt.Sprintf(`{"auths":{"%s":{"username":"%s","password":"%s","auth":"%s"}}}`,
			creds.URL, creds.Credentials.Username, creds.Credentials.Password, a)
		encoded := base64.StdEncoding.EncodeToString([]byte(dockerConfig))

		secret.Type = "kubernetes.io/dockerconfigjson"
		secret.Data = helm.SecretData{
			DockerConfigJSON: &encoded,
		}
	} else {
		encodedUser := base64.StdEncoding.EncodeToString([]byte(creds.Credentials.Username))
		encodedPass := base64.StdEncoding.EncodeToString([]byte(creds.Credentials.Password))
		secret.Type = "kubernetes.io/basic-auth"
		secret.Data = helm.SecretData{
			Username: &encodedUser,
			Password: &encodedPass,
		}
	}

	return secret
}

func extractHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing url %q: %w", rawURL, err)
	}

	return u.Host, nil
}
