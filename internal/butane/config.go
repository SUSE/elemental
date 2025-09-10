/*
Copyright © 2025 SUSE LLC
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

package butane

import (
	"fmt"
	"path/filepath"

	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"

	uc "github.com/suse/elemental/v3/internal/butane/unifiedcore/v0m1"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func WriteIngitionFile(s *sys.System, butaneBytes []byte, ignitionFile string) error {
	ignitionBytes, report, err := config.TranslateBytes(butaneBytes, common.TranslateBytesOptions{Pretty: true})
	if err != nil {
		return fmt.Errorf("failed translating Butane config: %w\nReport: %v", err, report)
	}

	if len(report.Entries) > 0 {
		s.Logger().Warn("translating Butane to Ignition reported non fatal entries: %v", report)
	}

	s.Logger().Debug("Butane configuration translated:\n--- Generated Ignition Config ---\n%s", string(ignitionBytes))

	err = vfs.MkdirAll(s.FS(), filepath.Dir(ignitionFile), vfs.DirPerm)
	if err != nil {
		return fmt.Errorf("failed creating ignition folder in overalay tree: %w", err)
	}

	err = s.FS().WriteFile(ignitionFile, ignitionBytes, vfs.FilePerm)
	if err != nil {
		return fmt.Errorf("filed writing Ignition file (%s): %w", ignitionFile, err)
	}
	s.Logger().Info("Successfully saved Ignition config to %s", ignitionFile)
	return nil
}

func init() {
	config.RegisterTranslator(uc.Variant, uc.Version, uc.ToIgn3_5Bytes)
}
