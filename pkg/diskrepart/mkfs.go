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
	"strings"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
)

type MkfsCall struct {
	fileSystem string
	label      string
	uuid       string
	customOpts []string
	dev        string
	runner     sys.Runner
	logger     log.Logger
}

func NewMkfsCall(s *sys.System, dev, fileSystem, label, uuid string, customOpts ...string) *MkfsCall {
	return &MkfsCall{
		dev: dev, fileSystem: fileSystem, label: label, uuid: uuid,
		runner: s.Runner(), customOpts: customOpts, logger: s.Logger(),
	}
}

func (mkfs MkfsCall) buildOptions() ([]string, error) {
	opts := []string{}

	f, _ := deployment.ParseFileSystem(mkfs.fileSystem)
	if mkfs.uuid != "" {
		if !checkUUID(mkfs.uuid, f) {
			return nil, fmt.Errorf("invalid uuid %s", mkfs.uuid)
		}
	}

	switch f {
	case deployment.XFS:
		if mkfs.label != "" {
			opts = append(opts, "-L")
			opts = append(opts, mkfs.label)
		}
		if mkfs.uuid != "" {
			opts = append(opts, "-m")
			opts = append(opts, fmt.Sprintf("uuid=%s", mkfs.uuid))
		}
		opts = append(opts, "-f")
	case deployment.Btrfs:
		if mkfs.label != "" {
			opts = append(opts, "-L")
			opts = append(opts, mkfs.label)
		}
		if mkfs.uuid != "" {
			opts = append(opts, "-U")
			opts = append(opts, mkfs.uuid)
		}
		opts = append(opts, "-f")
	case deployment.Ext2, deployment.Ext4:
		if mkfs.label != "" {
			opts = append(opts, "-L")
			opts = append(opts, mkfs.label)
		}
		if mkfs.uuid != "" {
			opts = append(opts, "-U")
			opts = append(opts, mkfs.uuid)
		}
		opts = append(opts, "-F")
	case deployment.VFat:
		if mkfs.label != "" {
			opts = append(opts, "-n")
			opts = append(opts, mkfs.label)
		}
		if mkfs.uuid != "" {
			opts = append(opts, "-i")
			opts = append(opts, strings.Split(mkfs.uuid, "-")[0])
		}
	default:
		return nil, fmt.Errorf("unsupported filesystem: %s", mkfs.fileSystem)
	}

	if len(mkfs.customOpts) > 0 {
		opts = append(opts, mkfs.customOpts...)
	}
	opts = append(opts, mkfs.dev)

	return opts, nil
}

func (mkfs MkfsCall) Apply() error {
	opts, err := mkfs.buildOptions()
	if err != nil {
		mkfs.logger.Error("failed preparing mkfs arguments: %v", err)
		return err
	}
	tool := fmt.Sprintf("mkfs.%s", mkfs.fileSystem)
	out, err := mkfs.runner.Run(tool, opts...)
	if err != nil {
		mkfs.logger.Error("mkfs failed with: %s", string(out))
	}
	return err
}
