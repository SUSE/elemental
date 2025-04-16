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

package install

import (
	"context"
	"os"
	"path/filepath"

	"github.com/suse/elemental/v3/pkg/block"
	"github.com/suse/elemental/v3/pkg/block/lsblk"
	"github.com/suse/elemental/v3/pkg/btrfs"
	"github.com/suse/elemental/v3/pkg/cleanstack"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/diskrepart"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
)

const configFile = "config.sh"

type Option func(*Installer)

type Installer struct {
	ctx context.Context
	s   *sys.System
	t   transaction.Interface
}

func WithTransaction(t transaction.Interface) Option {
	return func(i *Installer) {
		i.t = t
	}
}

func New(ctx context.Context, s *sys.System, opts ...Option) *Installer {
	installer := &Installer{
		s:   s,
		ctx: ctx,
	}
	for _, o := range opts {
		o(installer)
	}
	if installer.t == nil {
		installer.t = transaction.NewSnapperTransaction(ctx, s)
	}
	return installer
}

func (i Installer) Install(d *deployment.Deployment) (err error) {
	cleanup := cleanstack.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	for _, disk := range d.Disks {
		err = diskrepart.PartitionAndFormatDevice(i.s, disk)
		if err != nil {
			i.s.Logger().Error("installation failed, could not partition '%s'", disk.Device)
			return err
		}
		for _, part := range disk.Partitions {
			err = createPartitionVolumes(i.s, cleanup, part)
			if err != nil {
				i.s.Logger().Error("installation failed, could not create rw volumes")
				return err
			}
		}
	}

	err = i.t.Init(*d)
	if err != nil {
		i.s.Logger().Error("installation failed, could not initialize snapper")
		return err
	}

	trans, err := i.t.Start(d.SourceOS)
	if err != nil {
		i.s.Logger().Error("installation failed, could not start snapper transaction")
		return err
	}
	cleanup.PushErrorOnly(func() error { return i.t.Rollback(trans, err) })

	err = d.WriteDeploymentFile(i.s, trans.Path)
	if err != nil {
		i.s.Logger().Error("installation failed, could write deployment file")
		return err
	}

	hook, binds, err := configHook(i.ctx, i.s)
	if err != nil {
		return err
	}

	err = i.t.Commit(trans, hook, binds)
	if err != nil {
		i.s.Logger().Error("installation failed, could not close snapper transaction")
		return err
	}

	return nil
}

func createPartitionVolumes(s *sys.System, cleanStack *cleanstack.CleanStack, part *deployment.Partition) (err error) {
	var mountPoint string

	if len(part.RWVolumes) > 0 || part.Role == deployment.System {
		mountPoint, err = vfs.TempDir(s.FS(), "", "elemental_"+part.Role.String())
		if err != nil {
			s.Logger().Error("failed creating temporary directory to mount system partition")
			return err
		}
		cleanStack.PushSuccessOnly(func() error { return vfs.RemoveAll(s.FS(), mountPoint) })

		bDev := lsblk.NewLsDevice(s)
		bPart, err := block.GetPartitionByUUID(s, bDev, part.UUID, 4)
		if err != nil {
			s.Logger().Error("failed to find partition %d", part.UUID)
			return err
		}
		err = s.Mounter().Mount(bPart.Path, mountPoint, "", []string{})
		if err != nil {
			return err
		}
		cleanStack.Push(func() error { return s.Mounter().Unmount(mountPoint) })

		err = btrfs.SetBtrfsPartition(s, mountPoint)
		if err != nil {
			s.Logger().Error("failed setting brfs partition volumes")
			return err
		}
	}

	for _, rwVol := range part.RWVolumes {
		if rwVol.Snapshotted {
			continue
		}
		subvolume := filepath.Join(mountPoint, btrfs.TopSubVol, rwVol.Path)
		err = btrfs.CreateSubvolume(s, subvolume, true)
		if err != nil {
			s.Logger().Error("failed creating subvolume %s", subvolume)
			return err
		}
	}

	return nil
}

func configHook(ctx context.Context, s *sys.System) (transaction.Hook, transaction.HookBinds, error) {
	dir, err := os.Getwd()
	if err != nil {
		s.Logger().Error("failed to find out current directory")
		return nil, nil, err
	}
	config := filepath.Join(dir, configFile)
	if ok, _ := vfs.Exists(s.FS(), config); ok {
		rootedConfig := filepath.Join("/etc", configFile)
		return func() error {
			var stdOut, stdErr *string
			stdOut = new(string)
			stdErr = new(string)
			defer func() {
				logOutput(s, *stdOut, *stdErr)
			}()
			return s.Runner().RunContextParseOutput(ctx, stdHander(stdOut), stdHander(stdErr), rootedConfig)
		}, map[string]string{config: rootedConfig}, nil
	}
	return nil, nil, nil
}

func stdHander(out *string) func(string) {
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
