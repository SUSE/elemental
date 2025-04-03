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
)

type Transactioner struct {
	InitErr            error
	StartErr           error
	MergeErr           error
	CloseErr           error
	Trans              *transaction.Transaction
	SrcDigest          string
	closeOnErrorCalled bool
}

func NewTransaction() transaction.Interface {
	return &Transactioner{}
}

func (m Transactioner) Init(_ deployment.Deployment) error {
	return m.InitErr
}

func (m Transactioner) Start(imgsrc *deployment.ImageSource) (*transaction.Transaction, error) {
	imgsrc.SetDigest(m.SrcDigest)
	return m.Trans, m.StartErr
}

func (m Transactioner) Merge(_ *transaction.Transaction) error {
	return m.MergeErr
}

func (m Transactioner) Close(_ *transaction.Transaction, _ transaction.Hook, _ transaction.HookBinds) error {
	return m.CloseErr
}

func (m *Transactioner) CloseOnError(_ *transaction.Transaction, err error) error {
	m.closeOnErrorCalled = true
	return err
}

func (m Transactioner) CloseOnErrorCalled() bool {
	return m.closeOnErrorCalled
}
