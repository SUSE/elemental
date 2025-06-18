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
	"fmt"
	"strconv"

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
	ctx        context.Context
	s          *sys.System
	t          transaction.Interface
	bm         *firmware.EfiBootManager
	b          bootloader.Bootloader
	unpackOpts []unpack.Opt
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

func WithUnpackOpts(opts ...unpack.Opt) Option {
	return func(u *Upgrader) {
		u.unpackOpts = opts
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
		return fmt.Errorf("initializing transaction: %w", err)
	}

	trans, err := u.t.Start()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	cleanup.PushErrorOnly(func() error { return u.t.Rollback(trans, err) })

	err = uh.SyncImageContent(d.SourceOS, trans, u.unpackOpts...)
	if err != nil {
		return fmt.Errorf("syncing OS image content: %w", err)
	}

	err = uh.Merge(trans)
	if err != nil {
		return fmt.Errorf("merging RW volumes: %w", err)
	}

	err = uh.UpdateFstab(trans)
	if err != nil {
		return fmt.Errorf("updating fstab: %w", err)
	}

	err = selinux.ChrootedRelabel(u.ctx, u.s, trans.Path, nil)
	if err != nil {
		return fmt.Errorf("relabelling snapshot path '%s': %w", trans.Path, err)
	}

	err = d.WriteDeploymentFile(u.s, trans.Path)
	if err != nil {
		return fmt.Errorf("writing deployment file: %w", err)
	}

	err = uh.Lock(trans)
	if err != nil {
		return fmt.Errorf("locking transaction '%d': %w", trans.ID, err)
	}

	if d.OverlayTree != nil && !d.OverlayTree.IsEmpty() {
		unpacker, err := unpack.NewUnpacker(
			u.s, d.OverlayTree, unpack.WithRsyncFlags(overlayTreeSyncFlags()...),
		)
		if err != nil {
			return fmt.Errorf("initializing unpacker: %w", err)
		}
		_, err = unpacker.Unpack(u.ctx, trans.Path)
		if err != nil {
			return fmt.Errorf("unpacking overlay tree: %w", err)
		}
	}

	if d.CfgScript != "" {
		err = u.configHook(d.CfgScript, trans.Path)
		if err != nil {
			return fmt.Errorf("executing configuration hook: %w", err)
		}
	}

	baseCmdline := ""
	if d.BootConfig != nil {
		baseCmdline = d.BootConfig.KernelCmdline
	}

	kernelCmdline := fmt.Sprintf("%s %s", baseCmdline, uh.GenerateKernelCmdline(trans))

	err = u.b.Install(trans.Path, strconv.Itoa(trans.ID), kernelCmdline, d)
	if err != nil {
		return fmt.Errorf("installing bootloader: %w", err)
	}

	err = u.t.Commit(trans)
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
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

// overlayTreeSyncFlags provides the rsync flags that are used to sync directories or raw images
// during the overlay tree extraction. It does not keep permissions on pre-existing files or
// directories and it does not keep ownership of files and directories.
func overlayTreeSyncFlags() []string {
	return []string{
		"--recursive",
		"--hard-links",
		"--links",
		"--info=progress2",
		"--human-readable",
		"--partial",
	}
}
