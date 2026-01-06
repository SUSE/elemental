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

package http

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/sys/mock"
)

func TestDownloadSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Download test suite")
}

var _ = Describe("Invalid download attempts", func() {
	It("Fails to create a request for nil context", func() {
		err := DownloadFile(nil, nil, "", "")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("creating request: net/http: nil Context"))
	})

	It("Fails to execute a request for invalid URL", func() {
		err := DownloadFile(context.Background(), nil, "invalid-url", "")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("executing request: Get \"invalid-url\": unsupported protocol scheme \"\""))
	})

	It("Fails to download a request due to unexpected status code", func() {
		url := "https://github.com/suse/elemental3"
		err := DownloadFile(context.Background(), nil, url, "")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("unexpected status code: 404"))
	})

	It("Fails to create output file", func() {
		fs, cleanup, err := mock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		url := "https://github.com/suse/elemental"
		err = DownloadFile(context.Background(), fs, url, "downloads/abc")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("creating file: Create downloads/abc: operation not permitted"))
	})
})
