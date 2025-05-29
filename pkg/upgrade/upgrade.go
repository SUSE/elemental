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

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/selinux"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/transaction"
	"github.com/suse/elemental/v3/pkg/unpack"
)

const configFile = "/etc/elemental/config.sh"

type Interface interface {
	Upgrade(*deployment.Deployment) error
}

type Option func(*Upgrader)

type Upgrader struct {
	ctx context.Context
	s   *sys.System
	t   transaction.Interface
	bm  *firmware.EfiBootManager
	b   bootloader.Bootloader
}

func WithTransaction(t transaction.Interface) Option {
	return func(u *Upgrader) {
		u.t = t
	}
}

func WithBootManager(bm *firmware.EfiBootManager) Option {
	return func(u *Upgrader) {
		u.bm = bm
	}
}

func WithBootloader(b bootloader.Bootloader) Option {
	return func(u *Upgrader) {
		u.b = b
	}
}

func New(ctx context.Context, s *sys.System, opts ...Option) *Upgrader {
	up := &Upgrader{
		s:   s,
		ctx: ctx,
	}
	for _, o := range opts {
		o(up)
	}
	if up.t == nil {
		up.t = transaction.NewSnapperTransaction(ctx, s)
	}
	if up.b == nil {
		up.b = bootloader.NewNone(s)
	}
	return up
}

func (u Upgrader) Upgrade(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	var uh transaction.UpgradeHelper

	uh, err = u.t.Init(*d)
	if err != nil {
		u.s.Logger().Error("could not initialize transaction")
		return err
	}

	trans, err := u.t.Start()
	if err != nil {
		u.s.Logger().Error("could not start transaction")
		return err
	}
	cleanup.PushErrorOnly(func() error { return u.t.Rollback(trans, err) })

	err = uh.SyncImageContent(d.SourceOS, trans)
	if err != nil {
		u.s.Logger().Error("could not dump OS image")
		return err
	}

	err = uh.Merge(trans)
	if err != nil {
		u.s.Logger().Error("could not merge RW volumes")
		return err
	}

	err = uh.UpdateFstab(trans)
	if err != nil {
		u.s.Logger().Error("could not update fstab")
		return err
	}

	err = selinux.ChrootedRelabel(u.ctx, u.s, trans.Path, nil)
	if err != nil {
		u.s.Logger().Error("failed relabelling snapshot path: %s", trans.Path)
		return err
	}

	err = d.WriteDeploymentFile(u.s, trans.Path)
	if err != nil {
		u.s.Logger().Error("could not write deployment file")
		return err
	}

	err = uh.Lock(trans)
	if err != nil {
		u.s.Logger().Error("failed locking snapshot: %s", trans.Path)
		return err
	}

	if d.OverlayTree != nil && !d.OverlayTree.IsEmpty() {
		unpacker, err := unpack.NewUnpacker(u.s, d.OverlayTree)
		if err != nil {
			u.s.Logger().Error("could not initialize unpacker")
			return err
		}
		_, err = unpacker.Unpack(u.ctx, trans.Path)
		if err != nil {
			u.s.Logger().Error("could not unpack overlay tree")
			return err
		}
	}

	if d.CfgScript != "" {
		err = u.configHook(d.CfgScript, trans.Path)
		if err != nil {
			u.s.Logger().Error("configuration hook error")
			return err
		}
	}

	err = u.b.Install(trans.Path, d)
	if err != nil {
		u.s.Logger().Error("could not install bootloader: %s", err.Error())
		return err
	}

	err = u.t.Commit(trans)
	if err != nil {
		u.s.Logger().Error("could not close transaction")
		return err
	}

	return nil
}

func (u Upgrader) configHook(config string, root string) error {
	u.s.Logger().Info("Running transaction hook")
	callback := func() error {
		var stdOut, stdErr *string
		stdOut = new(string)
		stdErr = new(string)
		defer func() {
			logOutput(u.s, *stdOut, *stdErr)
		}()
		return u.s.Runner().RunContextParseOutput(u.ctx, stdHandler(stdOut), stdHandler(stdErr), configFile)
	}
	binds := map[string]string{config: configFile}
	return chroot.ChrootedCallback(u.s, root, binds, callback)
}

func stdHandler(out *string) func(string) {
	return func(line string) {
		*out += line + "\n"
	}
}

func logOutput(s *sys.System, stdOut, stdErr string) {
	output := "------- stdOut -------\n"
	output += stdOut
	output += "------- stdErr -------\n"
	output += stdErr
	output += "----------------------\n"
	s.Logger().Debug("Install config hook output:\n%s", output)
}
