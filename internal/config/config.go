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

package config

import (
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/deployment"
)

type Dir string

func (dir Dir) InstallFilepath() string {
	return filepath.Join(string(dir), "install.yaml")
}

func (dir Dir) ReleaseFilepath() string {
	return filepath.Join(string(dir), "release.yaml")
}

func (dir Dir) KubernetesFilepath() string {
	return filepath.Join(string(dir), "kubernetes.yaml")
}

func (dir Dir) ButaneFilepath() string {
	return filepath.Join(string(dir), "butane.yaml")
}

func (dir Dir) kubernetesDir() string {
	return filepath.Join(string(dir), "kubernetes")
}

func (dir Dir) KubernetesConfigDir() string {
	return filepath.Join(dir.kubernetesDir(), "config")
}

func (dir Dir) KubernetesManifestsDir() string {
	return filepath.Join(dir.kubernetesDir(), "manifests")
}

func (dir Dir) HelmValuesDir() string {
	return filepath.Join(dir.kubernetesDir(), "helm", "values")
}

func (dir Dir) NetworkDir() string {
	return filepath.Join(string(dir), "network")
}

type OutputDir string

func (dir OutputDir) OverlaysDir() string {
	return filepath.Join(string(dir), "overlays")
}

func (dir OutputDir) FirstbootConfigDir() string {
	return filepath.Join(dir.OverlaysDir(), deployment.ConfigMnt)
}

func (dir OutputDir) CatalystConfigDir() string {
	return filepath.Join(dir.OverlaysDir(), deployment.ConfigMnt, "catalyst")
}

func (dir OutputDir) ReleaseManifestsDir() string {
	return filepath.Join(string(dir), "release-manifests")
}
