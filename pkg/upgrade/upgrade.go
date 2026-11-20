/*
Copyright © 2025-2026 SUSE LLC
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
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/fips"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/hardening"
	"github.com/suse/elemental/v3/pkg/rsync"
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

func WithSnapshotter(s transaction.Interface) Option {
	return func(u *Upgrader) {
		u.t = s
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
		up.t = transaction.NewSnapper(ctx, s)
	}
	if up.b == nil {
		up.b = bootloader.NewNone(s)
	}
	return up
}

//nolint:gocyclo
func (u Upgrader) Upgrade(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	var uh transaction.UpgradeHelper

	esp := d.GetEfiPartition()
	if esp == nil {
		return fmt.Errorf("no %s partition defined in deployment", deployment.EfiLabel)
	}

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

	if d.IsFipsEnabled() {
		err = fips.ChrootedEnable(u.ctx, u.s, trans.Path)
		if err != nil {
			return fmt.Errorf("enabling FIPS: %w", err)
		}
	}

	if d.IsHardeningEnabled() {
		err = u.applyHardening(d, trans.Path)
		if err != nil {
			return fmt.Errorf("applying hardening profile: %w", err)
		}
	}

	err = selinux.ChrootedRefreshPolicy(u.ctx, u.s, trans.Path, selinux.SelinuxTargetedPolicyType)
	if err != nil {
		return fmt.Errorf("refreshing SE Linux policy: %w", err)
	}

	shared, snapshotted := parsePersistentPaths(d)
	err = selinux.ChrootedSystemRelabel(u.ctx, u.s, trans.Path, snapshotted, shared)
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
			u.s, d.OverlayTree, unpack.WithRsyncFlags(rsync.OverlayTreeSyncFlags()...),
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

	cmdline := ""
	initrdExts := []string{}
	if d.BootConfig != nil {
		cmdline = d.BootConfig.KernelCmdline
		initrdExts = d.BootConfig.InitrdExtensions
	}

	kernelCmdline := strings.TrimSpace(fmt.Sprintf("%s %s %s", d.BaseKernelCmdline(), uh.GenerateKernelCmdline(trans), cmdline))
	recKernelCmdline := ""
	if d.GetRecoveryPartition() != nil {
		recKernelCmdline = strings.TrimSpace(fmt.Sprintf("%s %s", d.RecoveryKernelCmdline(), d.Installer.KernelCmdline))
	}

	espDir := filepath.Join(trans.Path, esp.MountPoint)
	err = u.b.Install(bootloader.InstallCtx{
		RootDir:          trans.Path,
		Target:           espDir,
		ESPLabel:         esp.Label,
		EntryID:          strconv.Itoa(trans.ID),
		KernelCmdline:    kernelCmdline,
		RecKernelCmdline: recKernelCmdline,
		InitrdExtensions: initrdExts,
	})
	if err != nil {
		return fmt.Errorf("installing bootloader: %w", err)
	}

	if d.Firmware != nil {
		err = u.bm.CreateBootEntries(d.Firmware.BootEntries)
		if err != nil {
			return fmt.Errorf("creating EFI boot entries: %w", err)
		}
	}

	commitCleanup := func() error {
		snapshots, err := u.t.GetActiveSnapshotIDs()
		if err != nil {
			return fmt.Errorf("get active snapshots: %w", err)
		}

		return u.b.Prune(trans.Path, filepath.Join(trans.Path, esp.MountPoint), snapshots)
	}

	err = u.t.Commit(trans, commitCleanup)
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// applyHardening remediates the snapshot at root against the configured profile
// and adds the kernel arguments the profile requires to the deployment's boot
// configuration. It runs before the deployment file is written and before the
// bootloader is installed, so both pick up the changes. It runs again on every
// upgrade, which re-applies the profile to the new snapshot.
func (u Upgrader) applyHardening(d *deployment.Deployment, root string) error {
	res, err := hardening.Apply(u.ctx, u.s, root, d.Security.Hardening)
	if err != nil {
		return err
	}

	// From now on the tailoring file lives in the image (and is carried over by
	// the /etc merge on upgrades), not on the build host.
	if res.Tailoring != "" {
		d.Security.Hardening.Tailoring = res.Tailoring
	}

	if res.Plan.RequiresFIPS && !d.IsFipsEnabled() {
		u.s.Logger().Warn("Hardening profile %s requires FIPS mode, which is not enabled; set 'cryptoPolicy: fips' to comply", res.Profile)
	}

	if d.BootConfig == nil {
		d.BootConfig = &deployment.BootConfig{}
	}
	if err := hardening.CheckForbiddenKernelArgs(d.BootConfig.KernelCmdline, res.Plan.ForbiddenArgs); err != nil {
		return err
	}
	cmdline, appended, err := hardening.AppendKernelArgs(d.BootConfig.KernelCmdline, res.Plan.KernelArgs)
	if err != nil {
		return err
	}
	if len(appended) > 0 {
		u.s.Logger().Warn("Hardening profile %s requires kernel arguments not in the configured command line, appending: %s",
			res.Profile, strings.Join(appended, " "))
	}
	d.BootConfig.KernelCmdline = cmdline
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

func parsePersistentPaths(d *deployment.Deployment) (shared, snapshotted []string) {
	isRO := func(opts []string) bool {
		return slices.ContainsFunc(opts, func(s string) bool {
			return s == "ro" || strings.HasPrefix(s, "ro=")
		})
	}

	for _, part := range d.GetSELinuxSupportedPartitions() {
		if part.MountPoint != "" && !isRO(part.MountOpts) {
			shared = append(shared, part.MountPoint)
		}

		for _, rwVol := range part.RWVolumes {
			if !rwVol.Snapshotted {
				shared = append(shared, rwVol.Path)
			} else {
				snapshotted = append(snapshotted, rwVol.Path)
			}
		}
	}

	return shared, snapshotted
}
