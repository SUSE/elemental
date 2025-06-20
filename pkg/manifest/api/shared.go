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

package api

type MetaData struct {
	Name             string   `yaml:"name"`
	Version          string   `yaml:"version"`
	UpgradePathsFrom []string `yaml:"upgradePathsFrom,omitempty"`
	CreationDate     string   `yaml:"creationDate,omitempty"`
}

type Helm struct {
	Charts       []*HelmChart `yaml:"charts"`
	Repositories []Repository `yaml:"repositories"`
}

type HelmChart struct {
	Name       string         `yaml:"name,omitempty"`
	Chart      string         `yaml:"chart"`
	Version    string         `yaml:"version"`
	Namespace  string         `yaml:"namespace,omitempty"`
	Repository string         `yaml:"repository,omitempty"`
	Values     map[string]any `yaml:"values,omitempty"`
	DependsOn  []string       `yaml:"dependsOn,omitempty"`
	Images     []ChartImage   `yaml:"images,omitempty"`
}

type ChartImage struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

type Repository struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}
