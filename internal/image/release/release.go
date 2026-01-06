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

package release

type Release struct {
	Name        string     `yaml:"name,omitempty"`
	ManifestURI string     `yaml:"manifestURI"`
	Components  Components `yaml:"components,omitempty"`
}
type Components struct {
	SystemdExtensions []SystemdExtension `yaml:"systemd,omitempty"`
	HelmCharts        []HelmChart        `yaml:"helm,omitempty"`
}

func (c *Components) HelmValueFiles() map[string]string {
	m := map[string]string{}
	for _, chart := range c.HelmCharts {
		m[chart.Name] = chart.ValuesFile
	}

	return m
}

type SystemdExtension struct {
	Name string `yaml:"extension"`
}

type HelmChart struct {
	Name       string `yaml:"chart"`
	ValuesFile string `yaml:"valuesFile,omitempty"`
}
