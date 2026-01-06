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
	"fmt"
	"strings"
)

const (
	helmChartAPIVersion = "helm.cattle.io/v1"
	helmChartKind       = "HelmChart"
	helmBackoffLimit    = 20
	kubeSystemNamespace = "kube-system"
)

type CRD struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type Spec struct {
	Chart           string `yaml:"chart"`
	Version         string `yaml:"version"`
	Repo            string `yaml:"repo,omitempty"`
	ValuesContent   string `yaml:"valuesContent,omitempty"`
	TargetNamespace string `yaml:"targetNamespace,omitempty"`
	CreateNamespace bool   `yaml:"createNamespace,omitempty"`
	BackOffLimit    int    `yaml:"backOffLimit"`
}

func NewCRD(namespace, chart, version, valuesContent, repository string) *CRD {
	name := chart

	if strings.HasPrefix(repository, "oci://") {
		// The repository is in fact an OCI registry.
		// Use the full path for the chart identifier and drop the "repository" value.
		// The latter is only valid for HTTP(s) repositories.
		chart = fmt.Sprintf("%s/%s", repository, name)
		repository = ""
	}

	return &CRD{
		APIVersion: helmChartAPIVersion,
		Kind:       helmChartKind,
		Metadata: Metadata{
			Name:      name,
			Namespace: kubeSystemNamespace,
		},
		Spec: Spec{
			Chart:           chart,
			Version:         version,
			Repo:            repository,
			ValuesContent:   valuesContent,
			TargetNamespace: namespace,
			CreateNamespace: true,
			BackOffLimit:    helmBackoffLimit,
		},
	}
}
