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

package butane_test

import (
	"bytes"
	"encoding/json"
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

		conf.Variant = "flatcar"
		conf.Version = "1.1.0"
		conf.Passwd.Users = []v0_6.PasswdUser{{
			Name:              "pipo",
			SSHAuthorizedKeys: []v0_6.SSHAuthorizedKey{"longkey"},
		}}

		conf.AddSystemdUnit("test.service", "[Unit]\nDescription=Test unit\n[Install]\nWantedBy=test.target", true)
		conf.MergeInlineIngition("{\"ignition\": {\"version\": \"3.4.0\"}}")

		Expect(butane.WriteIngitionFile(system, conf, "/ignition.ign")).To(Succeed())
		ignBytes, err := system.FS().ReadFile("/ignition.ign")
		Expect(err).ToNot(HaveOccurred())

		Expect(json.Unmarshal(ignBytes, &jsonMap)).To(Succeed())
		// Converted to ignition 3.4.0 as expected by flatcar 1.1.0 variant
		Expect(jsonMap["ignition"].(map[string]any)["version"]).To(Equal("3.4.0"))

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
})
