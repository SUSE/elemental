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

package helm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

var _ = Describe("Values resolver tests", func() {
	Describe("Merging maps", func() {
		It("Returns an empty map if attempting to merge empty maps", func() {
			result := mergeMaps(map[string]any{}, map[string]any{})
			Expect(result).NotTo(BeNil())
			Expect(result).To(BeEmpty())
		})

		It("Merges non-empty first map and empty second map", func() {
			m1 := map[string]any{"a": 1}
			m2 := map[string]any{}

			result := mergeMaps(m1, m2)
			Expect(result).NotTo(BeNil())
			Expect(result).To(Equal(map[string]any{"a": 1}))
		})

		It("Merges empty first map and non-empty second map", func() {
			m1 := map[string]any{}
			m2 := map[string]any{"b": 5}

			result := mergeMaps(m1, m2)
			Expect(result).NotTo(BeNil())
			Expect(result).To(Equal(map[string]any{"b": 5}))
		})

		It("Merges two non-empty maps", func() {
			m1 := map[string]any{
				"a": map[string]any{
					"a1": 1,
					"a2": 5,
				},
				"b": "five",
			}
			m2 := map[string]any{
				"a": map[string]any{
					"a1": 100,
				},
				"c": 777,
			}

			result := mergeMaps(m1, m2)
			Expect(result).NotTo(BeNil())
			Expect(result).To(Equal(map[string]any{
				"a": map[string]any{
					"a1": 100,
					"a2": 5,
				},
				"b": "five",
				"c": 777,
			}))
		})
	})

	Describe("Resolving values", func() {
		It("Returns nil from empty source", func() {
			resolver := &ValuesResolver{}
			source := &ValueSource{}

			b, err := resolver.Resolve(source)
			Expect(err).NotTo(HaveOccurred())
			Expect(b).To(BeNil())
		})

		It("Returns inline values if file is not present", func() {
			resolver := &ValuesResolver{}
			source := &ValueSource{
				Inline: map[string]any{
					"debug":        true,
					"replicaCount": 1,
				},
			}

			b, err := resolver.Resolve(source)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("debug: true\nreplicaCount: 1\n"))
		})

		It("Fails reading from FS", func() {
			fs, cleanup, err := sysmock.TestFS(nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			resolver := &ValuesResolver{
				ValuesDir: "/etc/helm/values",
				FS:        fs,
			}

			source := &ValueSource{File: "neuvector.yaml"}
			b, err := resolver.Resolve(source)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reading values file: open"))
			Expect(err.Error()).To(ContainSubstring("no such file or directory"))
			Expect(b).To(BeNil())
		})

		It("Fails unmarshalling values from file", func() {
			fs, cleanup, err := sysmock.TestFS(map[string]string{
				"/etc/helm/values/neuvector.yaml": "11",
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			resolver := &ValuesResolver{
				ValuesDir: "/etc/helm/values",
				FS:        fs,
			}

			source := &ValueSource{File: "neuvector.yaml"}
			b, err := resolver.Resolve(source)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshaling values from file: yaml: unmarshal errors:"))
			Expect(b).To(BeNil())
		})

		It("Fails if values file is empty", func() {
			fs, cleanup, err := sysmock.TestFS(map[string]string{
				"/etc/helm/values/neuvector.yaml": "",
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			resolver := &ValuesResolver{
				ValuesDir: "/etc/helm/values",
				FS:        fs,
			}

			source := &ValueSource{Inline: nil, File: "neuvector.yaml"}
			b, err := resolver.Resolve(source)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("empty values file: /etc/helm/values/neuvector.yaml"))
			Expect(b).To(BeNil())
		})

		It("Returns values from file when inline ones are empty", func() {
			fs, cleanup, err := sysmock.TestFS(map[string]string{
				"/etc/helm/values/neuvector.yaml": `replicaCount: 3
foo: bar`,
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			resolver := &ValuesResolver{
				ValuesDir: "/etc/helm/values",
				FS:        fs,
			}

			source := &ValueSource{
				Inline: nil,
				File:   "neuvector.yaml",
			}

			b, err := resolver.Resolve(source)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("foo: bar\nreplicaCount: 3\n"))
		})

		It("Merges values from different sources", func() {
			fs, cleanup, err := sysmock.TestFS(map[string]string{
				"/etc/helm/values/neuvector.yaml": `replicaCount: 3
foo: bar`,
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(cleanup)

			resolver := &ValuesResolver{
				ValuesDir: "/etc/helm/values",
				FS:        fs,
			}

			source := &ValueSource{
				Inline: map[string]any{
					"debug":        true,
					"replicaCount": 1,
				},
				File: "neuvector.yaml",
			}

			b, err := resolver.Resolve(source)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal("debug: true\nfoo: bar\nreplicaCount: 3\n"))
		})
	})
})
