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

package build

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/sys"
)

var _ = Describe("Partition", func() {
	Describe("Prepare partition", func() {
		var b *Builder
		BeforeEach(func() {
			system, err := sys.NewSystem()
			Expect(err).ToNot(HaveOccurred())

			b = &Builder{
				System: system,
			}
		})
		It("is generated when dependency configurations are defined", func() {
			label := "ELEMENTAL-PREPARE"
			mountPoint := "/run/elemental/prepare"
			size := deployment.MiB(2048)
			role := deployment.Data
			fs := deployment.Btrfs
			def := &image.Definition{
				Network: image.Network{
					CustomScript: "/foo/configure-network.sh",
					ConfigDir:    "/foo",
				},
			}

			partition := b.generatePreparePartition(def)
			Expect(partition).ToNot(BeNil())
			Expect(partition.Label).To(Equal(label))
			Expect(partition.MountPoint).To(Equal(mountPoint))
			Expect(partition.Size).To(Equal(size))
			Expect(partition.Role).To(Equal(role))
			Expect(partition.FileSystem).To(Equal(fs))
		})
		It("skips generation when dependency configurations are missing", func() {
			Expect(b.generatePreparePartition(&image.Definition{})).To(BeNil())
		})
	})
})
