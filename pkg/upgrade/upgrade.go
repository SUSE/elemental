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

package upgrade

import (
	"context"

	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/selinux"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/transaction"
)

type Option func(*Upgrader)

type Upgrader struct {
	ctx context.Context
	s   *sys.System
	t   transaction.Interface
}

func WithTransaction(t transaction.Interface) Option {
	return func(i *Upgrader) {
		i.t = t
	}
}

func New(ctx context.Context, s *sys.System, opts ...Option) *Upgrader {
	upgrader := &Upgrader{
		s:   s,
		ctx: ctx,
	}
	for _, o := range opts {
		o(upgrader)
	}
	if upgrader.t == nil {
		upgrader.t = transaction.NewSnapperTransaction(ctx, s)
	}
	return upgrader
}

func (u Upgrader) Upgrade(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	err = u.t.Init(*d)
	if err != nil {
		u.s.Logger().Error("upgrade failed, could not initialize snapper")
		return err
	}

	trans, err := u.t.Start()
	if err != nil {
		u.s.Logger().Error("upgrade failed, could not start snapper transaction")
		return err
	}
	cleanup.PushErrorOnly(func() error { return u.t.Rollback(trans, err) })

	err = u.t.Update(trans, d.SourceOS, u.transactionHook(d, trans.Path))
	if err != nil {
		u.s.Logger().Error("upgrade failed, could not merge snapshotted volumes")
		return err
	}

	err = u.t.Commit(trans)
	if err != nil {
		u.s.Logger().Error("upgrade failed, could not close snapper transaction")
		return err
	}

	return nil
}

func (u Upgrader) transactionHook(d *deployment.Deployment, root string) transaction.UpdateHook {
	return func() error {
		err := selinux.ChrootedRelabel(u.ctx, u.s, root, nil)
		if err != nil {
			u.s.Logger().Error("failed relabelling snapshot path: %s", root)
			return err
		}

		err = d.WriteDeploymentFile(u.s, root)
		if err != nil {
			u.s.Logger().Error("upgrade failed, could not write deployment file")
			return err
		}
		return nil
	}
}
