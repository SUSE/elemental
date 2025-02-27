/*
Copyright © 2022 - 2025 SUSE LLC

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
	"fmt"
	"os/exec"
	"strings"

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

func NewRunner(opts ...RunOption) *run {
	r := &run{}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r run) InitCmd(command string, args ...string) *exec.Cmd {
	return exec.Command(command, args...)
}

func (r run) RunCmd(cmd *exec.Cmd) ([]byte, error) {
	return cmd.CombinedOutput()
}

func (r run) Run(command string, args ...string) ([]byte, error) {
	r.debug(fmt.Sprintf("Running cmd: '%s %s'", command, strings.Join(args, " ")))
	cmd := r.InitCmd(command, args...)
	out, err := r.RunCmd(cmd)
	if err != nil {
		r.debug(fmt.Sprintf("'%s' command reported an error: %s", command, err.Error()))
		r.debug(fmt.Sprintf("'%s' command output: %s", command, out))
	}
	return out, err
}

func (r run) debug(msg string) {
	if r.logger != nil {
		r.logger.Debug(msg)
	}
}
