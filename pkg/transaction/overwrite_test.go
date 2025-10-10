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

package transaction_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/block"
	blockmock "github.com/suse/elemental/v3/pkg/block/mock"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
)

var _ = Describe("OverwriteTransaction", Label("transaction", "overwrite"), func() {
	var overwrite transaction.Interface
	var cleanup func()
	var tfs vfs.FS

	BeforeEach(func() {
		mount := sysmock.NewMounter()
		runner := sysmock.NewRunner()
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		logger := log.New(log.WithDiscardAll())
		logger.SetLevel(log.DebugLevel())
		s, err = sys.NewSystem(
			sys.WithFS(tfs), sys.WithLogger(logger),
			sys.WithRunner(runner), sys.WithMounter(mount),
		)
		Expect(err).NotTo(HaveOccurred())

		d := deployment.DefaultDeployment()
		sysPart := d.GetSystemPartition()
		Expect(sysPart).ToNot(BeNil())
		sysPart.FileSystem = deployment.Ext4
		sysPart.RWVolumes = nil

		blk := blockmock.NewBlockDevice([]*block.Partition{
			{
				Name:       "/dev/loop0p1",
				Label:      deployment.EfiLabel,
				UUID:       "1234-ABCD",
				FileSystem: deployment.VFat.String(),
			},
			{
				Name:       "/dev/loop0p2",
				Label:      deployment.SystemLabel,
				UUID:       "2345-ABCD",
				FileSystem: deployment.Ext4.String(),
			},
		}...)

		overwrite = transaction.NewOverwrite(context.TODO(), s, d, blk)
	})

	AfterEach(func() {
		cleanup()
	})

	It("running install transaction", func() {
		_, err := overwrite.Init(deployment.Deployment{})
		Expect(err).To(Succeed())
		tran, err := overwrite.Start()
		Expect(err).To(Succeed())
		err = overwrite.Commit(tran)
		Expect(err).To(Succeed())
	})

	It("fails to rollback", func() {
		err := overwrite.Rollback(nil, nil)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(Equal("cannot rollback transactions using 'overwrite' snapshotter"))
	})
})
