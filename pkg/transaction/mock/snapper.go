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

package mock

import (
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/transaction"
	"github.com/suse/elemental/v3/pkg/unpack"
)

type Transactioner struct {
	InitErr        error
	StartErr       error
	CommitErr      error
	RollbackErr    error
	Trans          *transaction.Transaction
	UpgradeHelper  UpgradeHelper
	SrcDigest      string
	rollbackCalled bool
}

type UpgradeHelper struct {
	SyncError     error
	MergeError    error
	FstabError    error
	LockError     error
	srcDigest     string
	kernelCmdline string
}

func (u UpgradeHelper) SyncImageContent(imgSrc *deployment.ImageSource, _ *transaction.Transaction, _ ...unpack.Opt) error {
	imgSrc.SetDigest(u.srcDigest)
	return u.SyncError
}

func (u UpgradeHelper) Merge(_ *transaction.Transaction) error {
	return u.MergeError
}

func (u UpgradeHelper) UpdateFstab(_ *transaction.Transaction) error {
	return u.FstabError
}

func (u UpgradeHelper) Lock(_ *transaction.Transaction) error {
	return u.LockError
}

func (u UpgradeHelper) GenerateKernelCmdline(_ *transaction.Transaction) string {
	return u.kernelCmdline
}

func NewTransaction() transaction.Interface {
	return &Transactioner{}
}

func (t Transactioner) Init(_ deployment.Deployment) (transaction.UpgradeHelper, error) {
	t.UpgradeHelper.srcDigest = t.SrcDigest
	return t.UpgradeHelper, t.InitErr
}

func (t Transactioner) Start() (*transaction.Transaction, error) {
	return t.Trans, t.StartErr
}

func (t Transactioner) Commit(_ *transaction.Transaction) error {
	return t.CommitErr
}

func (t *Transactioner) Rollback(_ *transaction.Transaction, _ error) error {
	t.rollbackCalled = true
	return t.RollbackErr
}

func (t Transactioner) RollbackCalled() bool {
	return t.rollbackCalled
}
