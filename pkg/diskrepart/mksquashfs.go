/*
Copyright © 2022-2025 SUSE LLC
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

package diskrepart

import (
	"context"
	"fmt"

	"github.com/suse/elemental/v3/pkg/sys"
)

// CreateSquashFS creates a squash file at destination from a source, with options
func CreateSquashFS(ctx context.Context, s *sys.System, source string, destination string, options []string) error {
	args := []string{source, destination}

	args = append(args, options...)
	out, err := s.Runner().RunContext(ctx, "mksquashfs", args...)
	if err != nil {
		s.Logger().Error("Error running mksquashfs, stdout and stderr output: %s", out)
		return fmt.Errorf("error creating squashfs from %s to %s: %w", source, destination, err)
	}
	return nil
}

func SquashfsNoCompressionOptions() []string {
	return []string{"-no-compression"}
}

func DefaultSquashfsCompressionOptions() []string {
	return []string{"-b", "1024k"}
}

func SquashfsExcludeOptions(excludes ...string) []string {
	opts := []string{}
	if len(excludes) == 0 {
		return opts
	}
	opts = append(opts, "-wildcards", "-e")
	return append(opts, excludes...)
}
