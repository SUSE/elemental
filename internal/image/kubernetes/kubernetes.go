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

package kubernetes

import (
	"fmt"

	"github.com/suse/elemental/v3/pkg/helm"
)

const (
	NodeTypeServer = "server"
	NodeTypeAgent  = "agent"
)

type Kubernetes struct {
	// RemoteManifests - manifest URLs specified under config/kubernetes.yaml
	RemoteManifests []string `yaml:"manifests,omitempty" validate:"dive,required,url"`
	// Helm - charts specified under config/kubernetes.yaml
	Helm *Helm `yaml:"helm,omitempty" validate:"omitempty"`
	// LocalManifests - local manifest files specified under config/kubernetes/manifests
	LocalManifests []string
	Nodes          Nodes   `yaml:"nodes,omitempty" validate:"dive"`
	Network        Network `yaml:"network,omitempty"`
	Config         Config  `yaml:"-"`
}

type Config struct {
	// AgentFilePath path to agent.yaml rke2 configuration file
	AgentFilePath string
	// ServerFilePath path to server.yaml rke2 configuration file
	ServerFilePath string
}

type Helm struct {
	Charts       []*HelmChart      `yaml:"charts" validate:"dive"`
	Repositories []*HelmRepository `yaml:"repositories" validate:"dive"`
}

func (h *Helm) ChartRepositories() map[string]string {
	m := map[string]string{}
	for _, repo := range h.Repositories {
		m[repo.Name] = repo.URL
	}

	return m
}

func (h *Helm) ValueFiles() map[string]string {
	m := map[string]string{}
	for _, chart := range h.Charts {
		m[chart.Name] = chart.ValuesFile
	}

	return m
}

type HelmChart struct {
	Name            string `yaml:"name" validate:"required"`
	RepositoryName  string `yaml:"repositoryName" validate:"required"`
	Version         string `yaml:"version" validate:"required"`
	TargetNamespace string `yaml:"targetNamespace" validate:"required"`
	ValuesFile      string `yaml:"valuesFile"`
}

func (c *HelmChart) GetName() string {
	return c.Name
}

func (c *HelmChart) GetInlineValues() map[string]any {
	return nil
}

func (c *HelmChart) GetRepositoryName() string {
	return c.RepositoryName
}

func (c *HelmChart) ToCRD(values []byte, repository string) *helm.CRD {
	return helm.NewCRD(c.TargetNamespace, c.Name, c.Version, string(values), repository)
}

type HelmRepository struct {
	Name string `yaml:"name" validate:"required"`
	URL  string `yaml:"url" validate:"required,url"`
}

type Node struct {
	Hostname string `yaml:"hostname" validate:"required,hostname"`
	Type     string `yaml:"type" validate:"required,oneof=server agent"`
	Init     bool   `yaml:"init"`
}

type Nodes []Node

// FindInitNode loops through the nodes and returns the first one with init field set to true, or if none found, pick the first server Node.
func FindInitNode(nodes Nodes) (*Node, error) {
	var pick *Node
	for _, n := range nodes {
		if n.Init {
			return &n, nil
		}

		if pick == nil && n.Type == NodeTypeServer {
			pick = &n
		}
	}

	if pick == nil {
		return nil, fmt.Errorf("finding suitable init-node")
	}

	return pick, nil
}

type Network struct {
	APIHost string `yaml:"apiHost"`
	APIVIP4 string `yaml:"apiVIP" validate:"omitempty"`
	APIVIP6 string `yaml:"apiVIP6" validate:"omitempty,ipv6"`
}

func (n Network) IsHA() bool {
	return n.APIVIP4 != "" || n.APIVIP6 != ""
}
