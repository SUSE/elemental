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

package extensions

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/manifest/api"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"go.yaml.in/yaml/v3"
)

const (
	File = "/etc/elemental/extensions.yaml"
)

func Parse(s *sys.System, root string) ([]api.SystemdExtension, error) {
	path := filepath.Join(root, File)
	if ok, _ := vfs.Exists(s.FS(), path); !ok {
		s.Logger().Warn("Extensions file not found '%s'", path)
		return nil, os.ErrNotExist
	}

	data, err := s.FS().ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading extensions file '%s': %w", path, err)
	}

	var extensions []api.SystemdExtension

	if err = yaml.Unmarshal(data, &extensions); err != nil {
		return nil, fmt.Errorf("unmarshalling extensions file '%s': %w", path, err)
	}

	return extensions, nil
}

func Serialize(extensions []api.SystemdExtension) (string, error) {
	ext := make([]api.SystemdExtension, 0, len(extensions))

	for _, e := range extensions {
		copied := e // shallow copy
		copied.Required = false

		ext = append(ext, copied)
	}

	data, err := yaml.Marshal(ext)
	if err != nil {
		return "", err
	}

	dataStr := string(data)
	dataStr = "# self-generated content, do not edit\n\n" + dataStr

	return dataStr, err
}
