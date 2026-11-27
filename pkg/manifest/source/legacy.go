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

package source

// Support for the deprecated "file://./<path>" form for relative manifest URIs

import (
	"fmt"
	"net/url"
	"strings"
)

func RewriteLegacyRelativeURI(ref string) (string, bool) {
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != File.String() || u.Host != "." {
		return ref, false
	}

	rel := *u
	rel.Scheme = ""
	rel.Host = ""
	rel.Path = "./" + strings.TrimPrefix(u.Path, "/")
	rel.RawPath = ""
	return rel.String(), true
}

func LegacyRelativeWarning(ref, rewritten string) string {
	return fmt.Sprintf("%q uses the deprecated %s://./<path> form and will stop working in a future release; use the relative reference %q instead", ref, File, rewritten)
}
