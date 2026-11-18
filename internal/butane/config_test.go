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

package butane_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/coreos/butane/base/v0_6"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestButaneSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Butane test suite")
}

var _ = Describe("Ignition configuration", func() {
	var system *sys.System
	var fs vfs.FS
	var cleanup func()
	var err error

	var buffer *bytes.Buffer

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		fs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).ToNot(HaveOccurred())

		system, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())

	})

	AfterEach(func() {
		cleanup()
	})

	It("translates a butane configuration object to an ignition JSON file", func() {
		var conf butane.Config
		var jsonMap map[string]any

		conf.Variant = "fcos"
		conf.Version = "1.6.0"
		conf.Passwd.Users = []v0_6.PasswdUser{{
			Name:              "pipo",
			SSHAuthorizedKeys: []v0_6.SSHAuthorizedKey{"longkey"},
		}}

		conf.AddSystemdUnit("test.service", "[Unit]\nDescription=Test unit\n[Install]\nWantedBy=test.target", true)
		conf.MergeInlineIgnition("{\"ignition\": {\"version\": \"3.5.0\"}}")

		Expect(butane.WriteIgnitionFile(system, conf, "/ignition.ign")).To(Succeed())
		ignBytes, err := system.FS().ReadFile("/ignition.ign")
		Expect(err).ToNot(HaveOccurred())

		Expect(json.Unmarshal(ignBytes, &jsonMap)).To(Succeed())
		// Converted to ignition 3.5.0 as expected by fcos 1.6.0 variant
		Expect(jsonMap["ignition"].(map[string]any)["version"]).To(Equal("3.5.0"))

		// Defined user is added
		user := jsonMap["passwd"].(map[string]any)["users"].([]any)[0].(map[string]any)
		Expect(user["name"]).To(Equal("pipo"))

		// Merged ignition is present
		merges := jsonMap["ignition"].(map[string]any)["config"].(map[string]any)["merge"].([]any)
		Expect(len(merges)).To(Equal(1))

		// Systemd unit is present
		units := jsonMap["systemd"].(map[string]any)["units"].([]any)
		Expect(len(units)).To(Equal(1))
		Expect(units[0].(map[string]any)["enabled"]).To(BeTrue())
	})

	It("adds a file with inline contents from a reader", func() {
		var conf butane.Config

		Expect(conf.AddFileInlineFromReader("/etc/foo", io.NopCloser(strings.NewReader("bar")), 0o644)).To(Succeed())
		Expect(conf.Storage.Files).To(HaveLen(1))
		Expect(conf.Storage.Files[0].Path).To(Equal("/etc/foo"))
		Expect(*conf.Storage.Files[0].Contents.Inline).To(Equal("bar"))
	})

	It("fails to add a file from a reader exceeding the maximum size", func() {
		var conf butane.Config

		tooBig := io.NopCloser(io.LimitReader(zeroReader{}, 16*1024*1024+1))
		err := conf.AddFileInlineFromReader("/etc/foo", tooBig, 0o644)
		Expect(err).To(MatchError(ContainSubstring("reading contents for file '/etc/foo'")))
		Expect(conf.Storage.Files).To(BeEmpty())
	})
})

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
