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
	"errors"

	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/transaction"
)

type Transactioner struct {
	InitErr        error
	StartErr       error
	UpdateErr      error
	CommitErr      error
	Trans          *transaction.Transaction
	SrcDigest      string
	rollbackCalled bool
}

func NewTransaction() transaction.Interface {
	return &Transactioner{}
}

func (m Transactioner) Init(_ deployment.Deployment) error {
	return m.InitErr
}

func (m Transactioner) Start() (*transaction.Transaction, error) {
	return m.Trans, m.StartErr
}

func (m Transactioner) Update(_ *transaction.Transaction, imgsrc *deployment.ImageSource, hook transaction.UpdateHook) error {
	err := m.UpdateErr
	imgsrc.SetDigest(m.SrcDigest)
	if hook != nil {
		err = errors.Join(err, hook())
	}
	return err
}

func (m Transactioner) Commit(_ *transaction.Transaction) error {
	return m.CommitErr
}

func (m *Transactioner) Rollback(_ *transaction.Transaction, err error) error {
	m.rollbackCalled = true
	return err
}

func (m Transactioner) RollbackCalled() bool {
	return m.rollbackCalled
}
