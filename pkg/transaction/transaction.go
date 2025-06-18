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

package transaction

import (
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/unpack"
)

type transactionState int

const FstabFile = "/etc/fstab"

const (
	started transactionState = iota + 1
	committed
	failed
)

type Merge struct {
	Old      string // old unmodified tree
	New      string // new base tree where modifications should be applied
	Modified string // modified tree on top of the old tree
}

type Transaction struct {
	ID     int
	Path   string
	Merges map[string]*Merge

	status transactionState
}

type Interface interface {
	Init(deployment.Deployment) (UpgradeHelper, error)
	Start() (*Transaction, error)
	Commit(*Transaction) error
	Rollback(*Transaction, error) error
}

type UpgradeHelper interface {
	SyncImageContent(*deployment.ImageSource, *Transaction, ...unpack.Opt) error
	Merge(*Transaction) error
	UpdateFstab(*Transaction) error
	Lock(*Transaction) error
	GenerateKernelCmdline(*Transaction) string
}
