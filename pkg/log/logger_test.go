/*
Copyright © 2021 SUSE LLC

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

package log_test

import (
	"bytes"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/suse/elemental/v3/pkg/log"
)

func TestLogSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Log test suite")
}

var _ = Describe("logger", Label("log"), func() {
	It("New returns a logger interface based on logrus", func() {
		l1 := log.New()
		l2 := logrus.New()
		Expect(reflect.TypeOf(l1).Kind()).To(Equal(reflect.TypeOf(l2).Kind()))
	})
	It("New with options returns a logger interface based on logrus", func() {
		l1 := log.New(log.WithDiscardAll())
		l2 := logrus.New()
		Expect(reflect.TypeOf(l1).Kind()).To(Equal(reflect.TypeOf(l2).Kind()))
	})
	It("DebugLevel returns the proper log level for debug output", func() {
		Expect(log.DebugLevel()).To(Equal(uint32(logrus.DebugLevel)))
	})
	It("Returns true on IsDebugLevel when log level is set to debug", func() {
		l := log.New()
		l.SetLevel(log.DebugLevel())
		Expect(log.IsDebugLevel(l)).To(BeTrue())
	})
	It("Returns false on IsDebugLevel when log level is not set to debug", func() {
		Expect(log.IsDebugLevel(log.New())).To(BeFalse())
	})
	It("NewBufferLogger stores content in a buffer", func() {
		b := &bytes.Buffer{}
		l1 := log.New(log.WithBuffer(b))
		l1.Info("TEST")
		Expect(b).To(ContainSubstring("TEST"))
	})
})
