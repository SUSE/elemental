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

package kmod

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

var _ = Describe("Module cache building tests", func() {
	var tfs vfs.FS
	var cleanup func()
	var runner *mock.Runner

	BeforeEach(func() {
		var err error

		tfs, cleanup, err = mock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())

		runner = mock.NewRunner()
	})

	AfterEach(func() {
		cleanup()
	})

	It("Fails to generate module files", func() {
		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command == "depmod" {
				return []byte{}, fmt.Errorf("depmod error")
			}

			return []byte{}, nil
		}

		cache := &ModuleCache{
			KernelDir: "/usr/lib/modules/6.12.0-160000.5-default",
			FS:        tfs,
			Runner:    runner,
		}

		err := cache.Build(context.Background(), "")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("generating modules.dep and map files: depmod error"))
	})

	It("Fails to find kernel path if depmod is successful but the expected path is not built", func() {
		cache := &ModuleCache{
			KernelDir: "/usr/lib/modules/6.12.0-160000.5-default",
			FS:        tfs,
			Runner:    runner,
		}

		err := cache.Build(context.Background(), "/tmp/out")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("reading kernel path: open"))
		Expect(err.Error()).To(ContainSubstring("/tmp/out/usr/lib/modules/6.12.0-160000.5-default: no such file or directory"))
	})

	It("Successfully prepares module cache", func() {
		kernelDir := "/usr/lib/modules/6.12.0-160000.5-default"
		outDir := "/tmp/out"

		runner.SideEffect = func(command string, args ...string) ([]byte, error) {
			if command != "depmod" {
				return []byte{}, fmt.Errorf("unexpected command: %s", command)
			}

			// Simulate successful depmod output
			path := filepath.Join(outDir, kernelDir)
			if err := vfs.MkdirAll(tfs, path, 0755); err != nil {
				return []byte{}, err
			}

			for _, filename := range []string{"modules.dep", "modules.alias", "modules.symbols"} {
				if err := tfs.WriteFile(filepath.Join(path, filename), []byte{}, 0600); err != nil {
					return []byte{}, err
				}
			}

			return []byte{}, nil
		}

		cache := &ModuleCache{
			KernelDir: kernelDir,
			FS:        tfs,
			Runner:    runner,
		}

		Expect(cache.Build(context.Background(), outDir)).To(Succeed())

		Expect(vfs.Exists(tfs, filepath.Join(outDir, "modules.dep"))).To(BeTrue(), "Flattened modules.dep")
		Expect(vfs.Exists(tfs, filepath.Join(outDir, "modules.alias"))).To(BeTrue(), "Flattened modules.alias")
		Expect(vfs.Exists(tfs, filepath.Join(outDir, "modules.symbols"))).To(BeTrue(), "Flattened modules.symbols")
		Expect(vfs.Exists(tfs, filepath.Join(outDir, kernelDir))).To(BeFalse(), "Dropped symlink target")
	})
})
