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

package image

import (
	"bytes"
	"path/filepath"

	yaml "sigs.k8s.io/yaml/goyaml.v3"
)

type ConfigDir string

func (dir ConfigDir) InstallFilepath() string {
	return filepath.Join(string(dir), "install.yaml")
}

func (dir ConfigDir) OSFilepath() string {
	return filepath.Join(string(dir), "os.yaml")
}

func (dir ConfigDir) ReleaseFilepath() string {
	return filepath.Join(string(dir), "release.yaml")
}

func (dir ConfigDir) KubernetesFilepath() string {
	return filepath.Join(string(dir), "kubernetes.yaml")
}

func (dir ConfigDir) kubernetesDir() string {
	return filepath.Join(string(dir), "kubernetes")
}

func (dir ConfigDir) KubernetesManifestsDir() string {
	return filepath.Join(dir.kubernetesDir(), "manifests")
}

func (dir ConfigDir) HelmValuesDir() string {
	return filepath.Join(dir.kubernetesDir(), "helm", "values")
}

func ParseConfig(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	return decoder.Decode(target)
}
