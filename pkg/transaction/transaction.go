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

import "github.com/suse/elemental/v3/pkg/deployment"

const FstabFile = "/etc/fstab"

type Hook func() error

type HookBinds map[string]string

type Merge struct {
	Old      string // old unmodified tree
	New      string // new base tree where modifications should be applied
	Modified string // modified tree on top of the old tree
}

type Transaction struct {
	ID         int
	Path       string
	Merges     map[string]*Merge
	inProgress bool
}

type Interface interface {
	Init(deployment.Deployment) error
	Start(*deployment.ImageSource) (*Transaction, error)
	Merge(*Transaction) error
	Commit(*Transaction, Hook, HookBinds) error
	Rollback(*Transaction, error) error
}
