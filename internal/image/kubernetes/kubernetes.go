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

package kubernetes

type Kubernetes struct {
	// RemoteManifests - manifest URLs specified under config/kubernetes.yaml
	RemoteManifests []string `yaml:"manifests,omitempty"`
	// Helm - charts specified under config/kubernetes.yaml
	Helm *Helm `yaml:"helm,omitempty"`
	// LocalManifests - local manifest files specified under config/kubernetes/manifests
	LocalManifests []string
}

type Helm struct {
	Charts       []*HelmChart      `yaml:"charts"`
	Repositories []*HelmRepository `yaml:"repositories"`
}

type HelmChart struct {
	Name            string `yaml:"name"`
	RepositoryName  string `yaml:"repositoryName"`
	Version         string `yaml:"version"`
	TargetNamespace string `yaml:"targetNamespace"`
	ValuesFile      string `yaml:"valuesFile"`
}

type HelmRepository struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}
