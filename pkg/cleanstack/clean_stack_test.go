/*
Copyright © 2022-2026 SUSE LLC
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

package cleanstack_test

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/cleanstack"
)

func TestCleanStackSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CleanStack test suite")
}

var _ = Describe("CleanStack", Label("cleanstack"), func() {
	var cleaner *cleanstack.CleanStack
	BeforeEach(func() {
		cleaner = cleanstack.NewCleanStack()
	})
	It("Adds a callback to the stack and pops it", func() {
		var flag bool
		callback := func() error {
			flag = true
			return nil
		}
		Expect(cleaner.Pop()).To(BeNil())
		cleaner.Push(callback)
		poppedJob := cleaner.Pop()
		Expect(poppedJob).NotTo(BeNil())
		poppedJob.Run()
		Expect(flag).To(BeTrue())
	})
	It("On Cleanup runs callback stack in reverse order", func() {
		result := ""
		callback1 := func() error {
			result = result + "one "
			return nil
		}
		callback2 := func() error {
			result = result + "two "
			return nil
		}
		callback3 := func() error {
			result = result + "three "
			return nil
		}
		callback4 := func() error {
			result = result + "four "
			return nil
		}
		callback5 := func() error {
			result = result + "successOnly "
			return nil
		}
		cleaner.Push(callback1)
		cleaner.Push(callback2)
		cleaner.Push(callback3)
		cleaner.PushErrorOnly(callback4)
		cleaner.PushSuccessOnly(callback5)
		cleaner.Cleanup(nil)

		// Fourth callback is not executed if no error is reported
		Expect(result).To(Equal("successOnly three two one "))
	})
	It("cleans up keeping former error", func() {
		err := errors.New("Former error")
		count := 0
		onErrorCallback := false
		onSuccessCallback := false
		callback := func() error {
			count++
			if count == 2 {
				return errors.New("Cleanup Error")
			}
			return nil
		}
		successOnlyCallback := func() error {
			onSuccessCallback = true
			return nil
		}
		errorOnlyCallback := func() error {
			onErrorCallback = true
			return nil
		}
		cleaner.Push(callback)
		cleaner.Push(callback)
		cleaner.Push(callback)
		cleaner.PushSuccessOnly(successOnlyCallback)
		cleaner.PushErrorOnly(errorOnlyCallback)
		err = cleaner.Cleanup(err)
		Expect(count).To(Equal(3))
		Expect(err.Error()).To(ContainSubstring("Former error"))
		Expect(onSuccessCallback).To(BeFalse())
		Expect(onErrorCallback).To(BeTrue())
	})
	It("On Cleanup error reports first error and all callbacks are executed", func() {
		var err error
		count := 0
		onErrorCallback := false
		onSuccessCallback := false
		callback := func() error {
			count++
			if count >= 2 {
				return errors.New(fmt.Sprintf("Cleanup error %d", count))
			}
			return nil
		}
		successOnlyCallback := func() error {
			onSuccessCallback = true
			return nil
		}
		errorOnlyCallback := func() error {
			onErrorCallback = true
			return nil
		}
		cleaner.PushSuccessOnly(successOnlyCallback)
		cleaner.PushErrorOnly(errorOnlyCallback)
		cleaner.Push(callback)
		cleaner.Push(callback)
		cleaner.Push(callback)
		err = cleaner.Cleanup(err)
		Expect(count).To(Equal(3))
		Expect(err.Error()).To(ContainSubstring("Cleanup error 2"))
		Expect(err.Error()).To(ContainSubstring("Cleanup error 3"))
		Expect(onSuccessCallback).To(BeFalse())
		Expect(onErrorCallback).To(BeTrue())
	})
})
