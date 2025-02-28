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
	"fmt"
	"regexp"

	"github.com/suse/elemental/v3/pkg/sys"
)

type MkfsCall struct {
	fileSystem string
	label      string
	customOpts []string
	dev        string
	runner     sys.Runner
}

func NewMkfsCall(s *sys.System, dev string, fileSystem string, label string, customOpts ...string) *MkfsCall {
	return &MkfsCall{dev: dev, fileSystem: fileSystem, label: label, runner: s.Runner(), customOpts: customOpts}
}

func (mkfs MkfsCall) buildOptions() ([]string, error) {
	opts := []string{}

	linuxFS, _ := regexp.MatchString("ext[2-4]|xfs|btrfs", mkfs.fileSystem)
	fatFS, _ := regexp.MatchString("fat|vfat", mkfs.fileSystem)

	switch {
	case linuxFS:
		if mkfs.label != "" {
			opts = append(opts, "-L")
			opts = append(opts, mkfs.label)
		}
		if len(mkfs.customOpts) > 0 {
			opts = append(opts, mkfs.customOpts...)
		}
		if mkfs.fileSystem == "btrfs" {
			opts = append(opts, "-f")
		}
		opts = append(opts, mkfs.dev)
	case fatFS:
		if mkfs.label != "" {
			opts = append(opts, "-n")
			opts = append(opts, mkfs.label)
		}
		if len(mkfs.customOpts) > 0 {
			opts = append(opts, mkfs.customOpts...)
		}
		opts = append(opts, mkfs.dev)
	default:
		return []string{}, fmt.Errorf("unsupported filesystem: %s", mkfs.fileSystem)
	}
	return opts, nil
}

func (mkfs MkfsCall) Apply() (string, error) {
	opts, err := mkfs.buildOptions()
	if err != nil {
		return "", err
	}
	tool := fmt.Sprintf("mkfs.%s", mkfs.fileSystem)
	out, err := mkfs.runner.Run(tool, opts...)
	return string(out), err
}
