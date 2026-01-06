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
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type ValuesResolver struct {
	ValuesDir string
	FS        vfs.FS
}

type ValueSource struct {
	Inline map[string]any
	File   string
}

func (r *ValuesResolver) Resolve(source *ValueSource) ([]byte, error) {
	if source.File == "" {
		if len(source.Inline) == 0 {
			return nil, nil
		}

		b, err := yaml.Marshal(source.Inline)
		if err != nil {
			return nil, fmt.Errorf("marshaling inline values: %w", err)
		}

		return b, nil
	}

	valuesPath := filepath.Join(r.ValuesDir, source.File)
	valuesFromFile, err := r.FS.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("reading values file: %w", err)
	}

	if len(valuesFromFile) == 0 {
		return nil, fmt.Errorf("empty values file: %s", valuesPath)
	}

	var fromFile map[string]any

	if err = yaml.Unmarshal(valuesFromFile, &fromFile); err != nil {
		return nil, fmt.Errorf("unmarshaling values from file: %w", err)
	}

	values := mergeMaps(source.Inline, fromFile)
	v, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshaling values: %w", err)
	}

	return v, nil
}

func mergeMaps(m1, m2 map[string]any) map[string]any {
	out := make(map[string]any, len(m1))
	for k, v := range m1 {
		out[k] = v
	}

	for k, v := range m2 {
		if inner, ok := v.(map[string]any); ok {
			if outInner, ok := out[k].(map[string]any); ok {
				out[k] = mergeMaps(outInner, inner)
				continue
			}
		}
		out[k] = v
	}

	return out
}
