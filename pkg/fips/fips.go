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

package fips

import (
	"context"
	"fmt"

	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
)

func Enable(ctx context.Context, s *sys.System) error {
	stdOut, err := s.Runner().RunContext(ctx, "/usr/bin/fips-mode-setup", "--enable", "--no-bootcfg")
	s.Logger().Debug("fips-mode-setup: %s", string(stdOut))
	return err
}

func ChrootedEnable(ctx context.Context, s *sys.System, rootDir string) error {
	callback := func() error { return Enable(ctx, s) }
	return chroot.ChrootedCallback(s, rootDir, nil, callback)
}

func AppendCommandLine(cmdline string) string {
	bootFlag := fmt.Sprintf("boot=LABEL=%s", deployment.EfiLabel)
	fipsFlag := "fips=1"

	return fmt.Sprintf("%s %s %s", cmdline, fipsFlag, bootFlag)
}
