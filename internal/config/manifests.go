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
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func addManifest(fs vfs.FS, butaneCfg *butane.Config, path, name string) error {
	data, err := fs.ReadFile(path)
	if err != nil {
		return err
	}

	resources, err := parseManifests(bytes.NewReader(data))
	if err != nil {
		return err
	}

	if len(resources) == 0 {
		return fmt.Errorf("no Kubernetes resources found")
	}

	for _, resource := range resources {
		kind, _ := resource["kind"].(string)
		apiVersion, _ := resource["apiVersion"].(string)
		if kind == "" || apiVersion == "" {
			return fmt.Errorf("resource without 'kind' or 'apiVersion'")
		}
	}

	content := string(data)
	butaneCfg.AddFileInline(filepath.Join("/", image.KubernetesManifestsPath(), name), &content, 0o644)
	return nil
}

func readManifest(fs vfs.FS, path string) ([]map[string]any, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	return parseManifests(file)
}

// parseManifests decodes a stream of YAML documents into generic resources, skipping empty ones.
func parseManifests(r io.Reader) ([]map[string]any, error) {
	var resources []map[string]any

	decoder := yaml.NewDecoder(r)
	for {
		var resource map[string]any

		if err := decoder.Decode(&resource); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("unmarshalling manifest: %w", err)
		}

		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// extractManifestImages collects every "image" value found in workload resources.
func extractManifestImages(resource map[string]any, images map[string]struct{}) {
	workloadKinds := []string{"Pod", "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob"}

	kind, _ := resource["kind"].(string)
	if kind == "List" {
		items, _ := resource["items"].([]any)
		for _, item := range items {
			if child, ok := item.(map[string]any); ok {
				extractManifestImages(child, images)
			}
		}
		return
	}

	if !slices.Contains(workloadKinds, kind) {
		return
	}

	var findImages func(data any)

	findImages = func(data any) {
		switch t := data.(type) {
		case map[string]any:
			for k, v := range t {
				if k == "image" {
					if img, ok := v.(string); ok {
						images[img] = struct{}{}
					}
				}
				findImages(v)
			}
		case []any:
			for _, v := range t {
				findImages(v)
			}
		}
	}

	findImages(resource)
}
