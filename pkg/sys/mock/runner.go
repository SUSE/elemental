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

package mock

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
)

type Runner struct {
	cmds        [][]string
	envs        [][]string
	ReturnValue []byte
	SideEffect  func(command string, args ...string) ([]byte, error)
	ReturnError error
	Logger      log.Logger
}

var _ sys.Runner = (*Runner)(nil)

func NewRunner() *Runner {
	return &Runner{cmds: [][]string{}, ReturnValue: []byte{}, SideEffect: nil, ReturnError: nil}
}

func (r *Runner) Run(command string, args ...string) ([]byte, error) {
	return r.RunEnv(command, []string{}, args...)
}

func (r *Runner) RunEnv(command string, envs []string, args ...string) ([]byte, error) {
	err := r.ReturnError
	out := r.ReturnValue

	r.debug(fmt.Sprintf("Running cmd: '%s %s'", command, strings.Join(args, " ")))
	r.cmds = append(r.cmds, append([]string{command}, args...))
	r.envs = append(r.envs, append([]string{command}, envs...))
	if r.SideEffect != nil {
		if len(r.cmds) > 0 {
			lastCmd := len(r.cmds) - 1
			out, err = r.SideEffect(r.cmds[lastCmd][0], r.cmds[lastCmd][1:]...)
		}
	}
	if err != nil {
		r.error(fmt.Sprintf("Error running command: %s", err.Error()))
	}
	return out, err
}

func (r *Runner) RunContext(_ context.Context, command string, args ...string) ([]byte, error) {
	return r.Run(command, args...)
}

func (r *Runner) RunContextParseOutput(_ context.Context, stdoutH, _ func(string), command string, args ...string) error {
	out, err := r.Run(command, args...)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		stdoutH(scanner.Text())
	}
	return err
}

func (r *Runner) ClearCmds() {
	r.cmds = [][]string{}
}

// CmdsMatch matches the commands list in order. Note HasPrefix is being used to evaluate the
// match, so expecting initial part of the command is enough to get a match.
// It facilitates testing commands with dynamic arguments (aka temporary files)
func (r Runner) CmdsMatch(cmdList [][]string) error {
	if len(cmdList) != len(r.cmds) {
		return fmt.Errorf("number of calls mismatch, expected %d calls but got %d", len(cmdList), len(r.cmds))
	}
	for i, cmd := range cmdList {
		expect := strings.Join(cmd, " ")
		got := strings.Join(r.cmds[i], " ")
		if !strings.HasPrefix(got, expect) {
			return fmt.Errorf("expected command: '%s.*' got: '%s'", expect, got)
		}
	}
	return nil
}

func (r Runner) EnvsMatch(envList [][]string) error {
	if len(envList) != len(r.envs) {
		return fmt.Errorf("number of envs mismatch, expected %d calls but got %d", len(envList), len(r.envs))
	}
	for i, env := range envList {
		expect := strings.Join(env, " ")
		got := strings.Join(r.envs[i], " ")
		if !strings.HasPrefix(got, expect) {
			return fmt.Errorf("expected env var: '%s.*' got: '%s'", expect, got)
		}
	}
	return nil
}

// IncludesCmds checks the given commands were executed in any order.
// Note it uses HasPrefix to match commands, see CmdsMatch.
func (r Runner) IncludesCmds(cmdList [][]string) error {
	for _, cmd := range cmdList {
		expect := strings.Join(cmd, " ")
		found := false
		for _, rcmd := range r.cmds {
			got := strings.Join(rcmd, " ")
			if strings.HasPrefix(got, expect) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("command '%s.*' not found", expect)
		}
	}
	return nil
}

// MatchMilestones matches all the given commands were executed in the provided
// order. Note it uses HasPrefix to match commands, see CmdsMatch.
func (r Runner) MatchMilestones(cmdList [][]string) error {
	var match string
	for _, cmd := range r.cmds {
		if len(cmdList) == 0 {
			break
		}
		got := strings.Join(cmd, " ")
		match = strings.Join(cmdList[0], " ")
		if !strings.HasPrefix(got, match) {
			continue
		}

		cmdList = cmdList[1:]
	}

	if len(cmdList) > 0 {
		return fmt.Errorf("command '%s' not executed", match)
	}

	return nil
}

// GetCmds returns the list of commands recorded by this FakeRunner instance
// this is helpful to debug tests
func (r Runner) GetCmds() [][]string {
	return r.cmds
}

func (r Runner) GetLogger() log.Logger {
	return r.Logger
}

func (r *Runner) SetLogger(logger log.Logger) {
	r.Logger = logger
}

func (r Runner) error(msg string) {
	if r.Logger != nil {
		r.Logger.Error(msg)
	}
}

func (r Runner) debug(msg string) {
	if r.Logger != nil {
		r.Logger.Debug(msg)
	}
}
