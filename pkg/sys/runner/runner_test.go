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

package runner_test

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys/runner"
)

func TestRunnerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runner test suite")
}

var _ = Describe("Runner", Label("runner"), func() {
	It("Runs commands on the real Runner", func() {
		r := runner.NewRunner()
		_, err := r.Run("pwd")
		Expect(err).To(BeNil())
	})
	It("logs the command when on debug", func() {
		memLog := &bytes.Buffer{}
		logger := log.New(log.WithBuffer(memLog))
		logger.SetLevel(log.DebugLevel())
		r := runner.NewRunner(runner.WithLogger(logger))
		_, err := r.Run("echo", "-n", "Some message")
		Expect(err).To(BeNil())
		Expect(memLog.String()).To(ContainSubstring("echo -n Some message"))
	})
	It("logs when command is not found in debug mode", func() {
		memLog := &bytes.Buffer{}
		logger := log.New(log.WithBuffer(memLog))
		logger.SetLevel(log.DebugLevel())
		r := runner.NewRunner(runner.WithLogger(logger))
		_, err := r.Run("IAmMissing")
		Expect(err).NotTo(BeNil())
		Expect(memLog.String()).To(ContainSubstring("not found"))
	})
	It("runs a command with context and it can be cancelled", func() {
		r := runner.NewRunner()
		ctx, cancel := context.WithCancel(context.Background())

		var wg sync.WaitGroup
		var err error

		runWrapper := func() {
			_, err = r.RunContext(ctx, "sleep", "300")
			wg.Done()
		}
		before := time.Now()
		wg.Add(1)
		go runWrapper()
		time.Sleep(300 * time.Millisecond)
		cancel()
		wg.Wait()

		Expect(time.Now().After(before.Add(300 * time.Millisecond))).To(BeTrue())
		Expect(time.Now().Before(before.Add(1 * time.Second))).To(BeTrue())
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("killed"))
	})
	It("runs a command with context, it can be cancelled and parses output in real time", func() {
		r := runner.NewRunner()
		ctx, cancel := context.WithCancel(context.Background())

		var wg sync.WaitGroup
		var err error
		var stderr, stdout []string

		stdoutH := func(line string) {
			stdout = append(stdout, line)
		}
		stderrH := func(line string) {
			stderr = append(stderr, line)
		}

		runWrapper := func() {
			err = r.RunContextParseOutput(ctx, stdoutH, stderrH, "sh", "-c", "while true; do echo stdout; echo stderr 1>&2; sleep 1; done")
			wg.Done()
		}
		before := time.Now()
		wg.Add(1)
		go runWrapper()
		time.Sleep(2500 * time.Millisecond)
		cancel()
		wg.Wait()

		Expect(time.Now().After(before.Add(2500 * time.Millisecond))).To(BeTrue())
		Expect(time.Now().Before(before.Add(3 * time.Second))).To(BeTrue())
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("killed"))
		Expect(len(stdout)).To(Equal(3))
		Expect(slices.Contains(stdout, "stdout")).To(BeTrue())
		Expect(len(stderr)).To(Equal(3))
		Expect(slices.Contains(stderr, "stderr")).To(BeTrue())
	})
})
