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
	"encoding/base64"
	"fmt"
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
	"github.com/suse/elemental/v3/pkg/helm"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
)

type helmValuesResolver interface {
	Resolve(*helm.ValueSource) ([]byte, error)
}

type helmChart interface {
	GetName() string
	GetInlineValues() map[string]any
	GetRepositoryName() string
	ToCRD(values []byte, repository string, hasAuth, skipTLSVerify bool) *helm.CRD
}

type Helm struct {
	RelativePath   string
	ValuesResolver helmValuesResolver
	Logger         log.Logger
}

func NewHelm(valuesResolver helmValuesResolver, logger log.Logger) *Helm {
	return &Helm{
		RelativePath:   image.HelmPath(),
		ValuesResolver: valuesResolver,
		Logger:         logger,
	}
}

func (h *Helm) Configure(conf *image.Configuration, rm *resolver.ResolvedManifest, butaneCfg *butane.Config) ([]string, error) {
	if len(conf.Release.Components.HelmCharts) > 0 {
		var charts []string
		for _, c := range conf.Release.Components.HelmCharts {
			charts = append(charts, c.Name)
		}

		h.Logger.Info("Enabling the following Helm components: %s", strings.Join(charts, ", "))
	}

	charts, secrets, err := h.retrieveHelmCharts(rm, conf)
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

func (h *Helm) retrieveHelmCharts(rm *resolver.ResolvedManifest, conf *image.Configuration) ([]*helm.CRD, []*helm.Secret, error) {
	var crds []*helm.CRD

	valueFiles := conf.Release.Components.HelmValueFiles()

	err := evaluateLCMDeps(conf.Release.Components.HelmCharts, rm.CorePlatform, valueFiles, h.ValuesResolver)
	if err != nil {
		return nil, nil, fmt.Errorf("evaluating %s dependencies: %w", elementalLifecycleManager, err)
	}

	charts, repositories, err := enabledHelmCharts(rm, conf.Release.Components.HelmCharts, h.Logger)
	if err != nil {
		return nil, nil, fmt.Errorf("filtering enabled helm charts: %w", err)
	}

	authMap, err := createAuthMap(charts, repositories, conf)
	if err != nil {
		return nil, nil, fmt.Errorf("creating helm chart auth map: %w", err)
	}

	for _, chart := range charts {
		a := authMap[chart.Chart]
		needsAuth := a != nil
		skipTLSVerify := needsAuth && a.InsecureSkipTLSVerify
		if err = h.appendHelmChart(chart, repositories, valueFiles, &crds, needsAuth, skipTLSVerify); err != nil {
			return nil, nil, fmt.Errorf("collecting helm charts: %w", err)
		}
	}

	if conf.Kubernetes.Helm != nil {
		repositories = conf.Kubernetes.Helm.ChartRepositories()
		valueFiles = conf.Kubernetes.Helm.ValueFiles()

		for _, chart := range conf.Kubernetes.Helm.Charts {
			a := authMap[chart.Name]
			needsAuth := a != nil
			skipTLSVerify := needsAuth && a.InsecureSkipTLSVerify
			if err = h.appendHelmChart(chart, repositories, valueFiles, &crds, needsAuth, skipTLSVerify); err != nil {
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

func (h *Helm) appendHelmChart(chart helmChart, repositories, valueFiles map[string]string, crds *[]*helm.CRD, needsAuth, skipTLSVerify bool) error {
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

	crd := chart.ToCRD(values, repository, needsAuth, skipTLSVerify)
	*crds = append(*crds, crd)

	return nil
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
	addChart = func(name string) error {
		source := "core"

		chart, ok := coreCharts[name]
		if !ok {
			chart, ok = solutionCharts[name]
			if !ok {
				return fmt.Errorf("helm chart does not exist")
			}
			source = "solution"
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
