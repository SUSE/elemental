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

package cleanstack

import "errors"

const (
	errorOnly = iota
	successOnly
	always
)

type Task func() error

// Job represents a task. It can be of three different types: errorOnly type
// is a job only executed on error, successOnly type is executed only on sucess and always
// is always executed regardless the error value.
type Job struct {
	task    Task
	jobType int
}

// Run executes the defined job
func (cj Job) Run() error {
	return cj.task()
}

// Type returns the CleanJob type
func (cj Job) Type() int {
	return cj.jobType
}

// NewCleanStack returns a new stack.
func NewCleanStack() *CleanStack {
	return &CleanStack{}
}

// Stack is a basic LIFO stack that resizes as needed.
type CleanStack struct {
	jobs  []*Job
	count int
}

// Push adds a node to the stack that will be always executed
func (clean *CleanStack) Push(task Task) {
	clean.jobs = append(clean.jobs[:clean.count], &Job{task: task, jobType: always})
	clean.count++
}

// PushErrorOnly adds an error only node to the stack
func (clean *CleanStack) PushErrorOnly(task Task) {
	clean.jobs = append(clean.jobs[:clean.count], &Job{task: task, jobType: errorOnly})
	clean.count++
}

// PushSuccessOnly adds a success only node to the stack
func (clean *CleanStack) PushSuccessOnly(task Task) {
	clean.jobs = append(clean.jobs[:clean.count], &Job{task: task, jobType: successOnly})
	clean.count++
}

// Pop removes and returns a node from the stack in last to first order.
func (clean *CleanStack) Pop() *Job {
	if clean.count == 0 {
		return nil
	}
	clean.count--
	return clean.jobs[clean.count]
}

// Cleanup runs the whole cleanup stack. In case of error it runs all jobs
// and returns the first error occurrence.
func (clean *CleanStack) Cleanup(err error) error {
	for clean.count > 0 {
		job := clean.Pop()
		switch job.Type() {
		case successOnly:
			if err == nil {
				err = runCleanJob(job, err)
			}
		case errorOnly:
			if err != nil {
				err = runCleanJob(job, err)
			}
		default:
			err = runCleanJob(job, err)
		}
	}
	return err
}

func runCleanJob(job *Job, errs error) error {
	err := job.Run()
	if err != nil {
		errs = errors.Join(errs, err)
	}
	return errs
}
