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

package bootloader

import (
	"errors"
	"fmt"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
)

type Bootloader interface {
	Install(rootPath, snapshotID, kernelCmdline string, d *deployment.Deployment) error
}

const (
	BootNone = "none"
	BootGrub = "grub"
)

type None struct {
	s *sys.System
}

func NewNone(s *sys.System) *None {
	return &None{s}
}

func (n *None) Install(_, _, _ string, _ *deployment.Deployment) error {
	n.s.Logger().Info("Skipping bootloader installation")
	return nil
}

func New(name string, s *sys.System) (Bootloader, error) {
	switch name {
	case BootNone:
		return NewNone(s), nil
	case BootGrub:
		return NewGrub(s), nil
	}

	return nil, fmt.Errorf("new bootloader '%s': %w", name, errors.ErrUnsupported)
}
