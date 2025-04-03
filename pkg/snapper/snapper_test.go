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

package snapper_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/snapper"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestSnapperSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapper test suite")
}

const snapperList = `{
  "root": [
    {
      "number": 0,
      "default": false,
      "active": false,
      "userdata": null
    },
    {
      "number": 192,
      "default": true,
      "active": true,
      "userdata": null
    },
    {
      "number": 336,
      "default": false,
      "active": false,
      "userdata": {
        "important": "no"
      }
    },
    {
      "number": 337,
      "default": false,
      "active": false,
      "userdata": {
        "important": "yes"
      }
    },
	 {
      "number": 338,
      "default": false,
      "active": false,
      "userdata": {
        "important": "yes"
      }
    }
  ]
}`

var _ = Describe("DiskRepart", Label("diskrepart"), func() {
	var runner *sysmock.Runner
	var mounter *sysmock.Mounter
	var fs vfs.FS
	var cleanup func()
	var s *sys.System
	var snap *snapper.Snapper
	BeforeEach(func() {
		var err error
		runner = sysmock.NewRunner()
		mounter = sysmock.NewMounter()
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithMounter(mounter), sys.WithRunner(runner),
			sys.WithFS(fs), sys.WithLogger(log.New(log.WithDiscardAll())),
		)
		Expect(err).NotTo(HaveOccurred())
		snap = snapper.New(s)
	})
	AfterEach(func() {
		cleanup()
	})
	It("configures first root snapshot", func() {
		id, err := snap.FirstRootSnapshot("/some/root", map[string]string{"key": "value"})
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal(1))
		Expect(runner.CmdsMatch([][]string{
			{snapper.SnapperInstaller, "--root-prefix", "/some/root", "--step", "config"},
		})).To(Succeed())

		runner.ReturnError = fmt.Errorf("installation helper failed")
		id, err = snap.FirstRootSnapshot("/some/root", map[string]string{"key": "value"})
		Expect(err).To(HaveOccurred())
		Expect(id).To(Equal(0))
	})
	It("initiates root subvolume", func() {
		Expect(snap.InitSnapperRootVolumes("/some/root")).To(Succeed())
		Expect(runner.CmdsMatch([][]string{
			{snapper.SnapperInstaller, "--root-prefix", "/some/root", "--step", "filesystem"},
		})).To(Succeed())

		runner.ReturnError = fmt.Errorf("installation helper failed")
		Expect(snap.InitSnapperRootVolumes("/some/root")).NotTo(Succeed())
	})
	It("creates a new configuration", func() {
		Expect(snap.CreateConfig("/some/root", "/etc/systemd/")).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"snapper", "--no-dbus", "--root", "/some/root", "-c", "etc_systemd",
			"create-config", "--fstype", "btrfs", "/etc/systemd/",
		}})).To(Succeed())

		runner.ReturnError = fmt.Errorf("snapper create-config failed")
		Expect(snap.CreateConfig("/some/root", "/etc/systemd/")).NotTo(Succeed())
	})
	It("creates a new snapshot", func() {
		snapperCmd := [][]string{{
			"snapper", "--no-dbus", "--root", "/some/root", "-c", "root",
			"create", "--print-number", "-c", "number", "--userdata", "key=value",
			"--description", "description", "--read-write", "--from", "3",
		}}
		runner.ReturnValue = []byte("4")
		id, err := snap.CreateSnapshot(
			"/some/root", "", 3, true, "description", map[string]string{"key": "value"},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal(4))
		Expect(runner.CmdsMatch(snapperCmd)).To(Succeed())

		runner.ReturnValue = []byte("wrong")
		id, err = snap.CreateSnapshot(
			"/some/root", "", 3, true, "description", map[string]string{"key": "value"},
		)
		Expect(id).To(Equal(0))
		Expect(err).To(HaveOccurred())

		runner.ReturnError = fmt.Errorf("snapper failed")
		id, err = snap.CreateSnapshot(
			"/some/root", "", 3, true, "description", map[string]string{"key": "value"},
		)
		Expect(id).To(Equal(0))
		Expect(err).To(HaveOccurred())
	})
	It("sets default snapshot", func() {
		Expect(snap.SetDefault("/some/root", 3, false, map[string]string{"key": "value"})).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"snapper", "--no-dbus", "--root", "/some/root", "modify",
			"--default", "--read-only", "--userdata", "key=value", "3",
		}})).To(Succeed())

		Expect(snap.SetDefault("/some/root", 3, true, nil)).To(Succeed())
		Expect(runner.IncludesCmds([][]string{{
			"snapper", "--no-dbus", "--root", "/some/root", "modify",
			"--default", "--read-write", "3",
		}})).To(Succeed())

		runner.ReturnError = fmt.Errorf("snapper modify failed")
		Expect(snap.SetDefault("/some/root", 3, true, nil)).NotTo(Succeed())
	})
	It("cleans up old snapshots", func() {
		Expect(snap.SetDefault("/some/root", 3, false, map[string]string{"key": "value"})).To(Succeed())
		Expect(runner.CmdsMatch([][]string{{
			"snapper", "--no-dbus", "--root", "/some/root", "modify",
			"--default", "--read-only", "--userdata", "key=value", "3",
		}})).To(Succeed())

		Expect(snap.SetDefault("/some/root", 3, true, nil)).To(Succeed())
		Expect(runner.IncludesCmds([][]string{{
			"snapper", "--no-dbus", "--root", "/some/root", "modify",
			"--default", "--read-write", "3",
		}})).To(Succeed())

		runner.ReturnError = fmt.Errorf("snapper modify failed")
		Expect(snap.SetDefault("/some/root", 3, true, nil)).NotTo(Succeed())
	})
	Describe("ListSnapshots", func() {
		It("gets the list of snapshots", func() {
			runner.SideEffect = func(_ string, _ ...string) ([]byte, error) {
				return []byte(snapperList), nil
			}
			snaps, err := snap.ListSnapshots("/some/root", "root")
			Expect(err).NotTo(HaveOccurred())
			Expect(snaps.GetActive()).To(Equal(192))
			Expect(snaps.GetDefault()).To(Equal(192))
			Expect(snaps.GetWithUserdata("important", "no")).To(Equal([]int{336}))
		})
		It("fails to list snapshots for a wrong configuration", func() {
			runner.SideEffect = func(_ string, _ ...string) ([]byte, error) {
				return []byte(snapperList), nil
			}
			_, err := snap.ListSnapshots("/some/root", "wrong")
			Expect(err).To(HaveOccurred())
		})
		It("'snapper list' command fails", func() {
			runner.ReturnError = fmt.Errorf("snapper call failed")
			_, err := snap.ListSnapshots("/some/root", "root")
			Expect(err).To(HaveOccurred())
		})
		It("fails to unmarshal 'snapper list' command output", func() {
			runner.ReturnValue = []byte("this is not a json")
			_, err := snap.ListSnapshots("/some/root", "root")
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("Cleanup", func() {
		It("does nothing if snapshots count is not over the maximum", func() {
			runner.SideEffect = func(_ string, _ ...string) ([]byte, error) {
				return []byte(snapperList), nil
			}
			Expect(snap.Cleanup("/some/root", 4)).To(Succeed())
			Expect(runner.CmdsMatch([][]string{{
				"snapper", "--no-dbus", "--root", "/some/root", "-c", "root",
				"--jsonout", "list", "--columns", "number,default,active,userdata",
			}})).To(Succeed())
		})
		It("clears old snapshots until snapshots count is not higher than maximum", func() {
			runner.SideEffect = func(_ string, _ ...string) ([]byte, error) {
				return []byte(snapperList), nil
			}
			Expect(snap.Cleanup("/some/root", 2)).To(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{
					"snapper", "--no-dbus", "--root", "/some/root", "-c", "root",
					"--jsonout", "list", "--columns", "number,default,active,userdata",
				}, {"btrfs", "property"}, {"btrfs", "subvolume"}, {"btrfs", "property"}, {"btrfs", "subvolume"},
			})).To(Succeed())
		})
		It("fails to list current snapshots", func() {
			runner.ReturnError = fmt.Errorf("snapper call failed")
			Expect(snap.Cleanup("/some/root", 4)).NotTo(Succeed())
			Expect(runner.CmdsMatch([][]string{{
				"snapper", "--no-dbus", "--root", "/some/root", "-c", "root",
				"--jsonout", "list", "--columns", "number,default,active,userdata",
			}})).To(Succeed())
		})
		It("fails to delete specific snapshot", func() {
			runner.SideEffect = func(cmd string, _ ...string) ([]byte, error) {
				if cmd == "btrfs" {
					return []byte{}, fmt.Errorf("btrfs command failed")
				}
				return []byte(snapperList), nil
			}
			Expect(snap.Cleanup("/some/root", 2)).NotTo(Succeed())
			Expect(runner.CmdsMatch([][]string{
				{
					"snapper", "--no-dbus", "--root", "/some/root", "-c", "root",
					"--jsonout", "list", "--columns", "number,default,active,userdata",
				}, {"btrfs", "property"},
			})).To(Succeed())
		})
	})
	Describe("ConfigureRoot", func() {
		It("creates a root configuration", func() {
			rootDir := "/some/root"
			sysconfig := filepath.Join(rootDir, "/etc/sysconfig/snapper")
			template := filepath.Join(rootDir, "/usr/share/snapper/config-templates/default")
			configs := filepath.Join(rootDir, "/etc/snapper/configs")
			config := filepath.Join(rootDir, "/etc/snapper/configs/root")
			Expect(vfs.MkdirAll(fs, configs, vfs.DirPerm)).To(Succeed())
			Expect(vfs.MkdirAll(fs, filepath.Dir(sysconfig), vfs.DirPerm)).To(Succeed())
			Expect(fs.WriteFile(sysconfig, []byte{}, vfs.FilePerm)).To(Succeed())
			Expect(vfs.MkdirAll(fs, filepath.Dir(template), vfs.DirPerm)).To(Succeed())
			Expect(fs.WriteFile(template, []byte{}, vfs.FilePerm)).To(Succeed())
			Expect(snap.ConfigureRoot(rootDir, 4)).To(Succeed())
			f, err := fs.Open(config)
			Expect(err).NotTo(HaveOccurred())
			envMap, err := godotenv.Parse(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(envMap["TIMELINE_CREATE"]).To(Equal("no"))
			Expect(envMap["NUMBER_LIMIT"]).To(Equal("1-4"))
		})
	})
})
