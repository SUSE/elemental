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

package runner

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/suse/elemental/v3/pkg/log"
)

type run struct {
	logger log.Logger
}

type RunOption func(r *run)

func WithLogger(l log.Logger) RunOption {
	return func(r *run) {
		r.logger = l
	}
}

func NewRunner(opts ...RunOption) *run { //nolint:revive
	r := &run{}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r run) Run(command string, args ...string) ([]byte, error) {
	r.debug("Running cmd: '%s %s'", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.debug("'%s' command reported an error: %s", command, err.Error())
		r.debug("'%s' command output: %s", command, out)
	}
	return out, err
}

func (r run) RunContext(ctx context.Context, command string, args ...string) ([]byte, error) {
	r.debug("Running cmd: '%s %s'", command, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.debug("'%s' command reported an error: %s", command, err.Error())
		r.debug("'%s' command output: %s", command, out)
	}
	return out, err
}

func (r run) RunContextParseOutput(ctx context.Context, stdoutH, stderrH func(string), command string, args ...string) error {
	var err error
	var stdoutP, stderrP io.ReadCloser
	var wg sync.WaitGroup

	r.debug("Running cmd: '%s %s'", command, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, command, args...)
	if stdoutH != nil {
		stdoutP, err = cmd.StdoutPipe()
		if err != nil {
			r.debug("cound not pipe stdout for command '%s': %s", command, err.Error())
			return err
		}
		defer stdoutP.Close()
	}
	if stderrH != nil {
		stderrP, err = cmd.StderrPipe()
		if err != nil {
			r.debug("cound not pipe stderr for command '%s': %s", command, err.Error())
			return err
		}
		defer stderrP.Close()
	}
	err = cmd.Start()
	if err != nil {
		r.debug("'%s' command reported an error: %s", command, err.Error())
		return err
	}

	if stdoutP != nil {
		wg.Add(1)
		go parseReader(&wg, stdoutP, stdoutH)
	}

	if stderrP != nil {
		wg.Add(1)
		go parseReader(&wg, stderrP, stderrH)
	}

	err = cmd.Wait()
	if err != nil {
		r.debug("'%s' command exited with error: %s", command, err.Error())
		return err
	}

	if stderrP != nil {
		_ = stderrP.Close()
	}

	if stdoutP != nil {
		_ = stdoutP.Close()
	}

	wg.Wait()
	return nil
}

func (r run) debug(msg string, args ...any) {
	if r.logger != nil {
		r.logger.Debug(msg, args...)
	}
}

func parseReader(wg *sync.WaitGroup, reader io.Reader, parser func(string)) {
	scanner := bufio.NewScanner(reader)
	scanner.Split(scanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parser(line)
	}
	wg.Done()
}

// scanLine is a port form the bufio.ScanLines including '/r' rune as a line break
func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, data[0:i], nil
	}
	if i := bytes.IndexByte(data, '\r'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, data[0:i], nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}
