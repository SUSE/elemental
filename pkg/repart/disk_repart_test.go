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

package repart_test

import (
	"bytes"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/repart"
)

func TestRepartSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Repart test suite")
}

var _ = Describe("Systemd-repart tests", Label("systemd-repart"), func() {
	It("creates a partition configuration", func() {
		var buffer bytes.Buffer
		part := deployment.Partition{
			Label: "SYSTEM",
			Role:  deployment.System,
		}

		Expect(repart.CreatePartitionConf(&buffer, &part, "")).To(Succeed())
		Expect(buffer.String()).To(ContainSubstring("Type=root"))
		Expect(buffer.String()).ToNot(ContainSubstring("Format"))
		Expect(buffer.String()).ToNot(ContainSubstring("CopyFiles"))
		Expect(buffer.String()).ToNot(ContainSubstring("SizeMinBytes"))
		Expect(buffer.String()).ToNot(ContainSubstring("UUID"))
		Expect(buffer.String()).ToNot(ContainSubstring("ReadOnly"))

		buffer.Reset()
		part.Size = 1024
		part.FileSystem = deployment.Btrfs
		part.MountOpts = []string{"ro=vfs"}

		Expect(repart.CreatePartitionConf(&buffer, &part, "/some/root:/")).To(Succeed())
		Expect(buffer.String()).To(ContainSubstring("Type=root"))
		Expect(buffer.String()).To(ContainSubstring("SizeMinBytes=1024M"))
		Expect(buffer.String()).To(ContainSubstring("SizeMaxBytes=1024M"))
		Expect(buffer.String()).To(ContainSubstring("Format=btrfs"))
		Expect(buffer.String()).To(ContainSubstring("CopyFiles=/some/root:/"))
		Expect(buffer.String()).To(ContainSubstring("ReadOnly=on"))
		Expect(buffer.String()).ToNot(ContainSubstring("UUID"))
	})

	It("fails to create partition configuration with invalid data", func() {
		var buffer bytes.Buffer
		part := deployment.Partition{
			Label: "SYSTEM",
		}
		Expect(repart.CreatePartitionConf(&buffer, &part, "")).To(
			MatchError(ContainSubstring("invalid partition role")),
		)

		part.Role = deployment.Data
		Expect(repart.CreatePartitionConf(&buffer, &part, "relative/path:/")).To(
			MatchError(ContainSubstring("requires an absolute path")),
		)
	})
})
