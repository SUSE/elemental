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

//revive:disable:var-naming
package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

func Parse(name string, contents string, templateData any) (string, error) {
	if templateData == nil {
		return "", fmt.Errorf("template data not provided")
	}

	funcs := template.FuncMap{"join": strings.Join}

	tmpl, err := template.New(name).Funcs(funcs).Parse(contents)
	if err != nil {
		return "", fmt.Errorf("parsing contents: %w", err)
	}

	var buff bytes.Buffer
	if err = tmpl.Execute(&buff, templateData); err != nil {
		return "", fmt.Errorf("applying template: %w", err)
	}

	return buff.String(), nil
}
