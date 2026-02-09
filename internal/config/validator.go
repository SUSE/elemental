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

package config

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/install"
)

var (
	validate *validator.Validate
	once     sync.Once
)

func getValidator() *validator.Validate {
	once.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
		_ = validate.RegisterValidation("disksize", validateDiskSize)
	})
	return validate
}

func validateDiskSize(fl validator.FieldLevel) bool {
	diskSize, ok := fl.Field().Interface().(install.DiskSize)
	if !ok {
		return false
	}
	if diskSize == "" {
		return true
	}
	return diskSize.IsValid()
}

func Validate(conf *image.Configuration) error {
	err := getValidator().Struct(conf)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		var messages []string
		for _, vErr := range validationErrors {
			switch vErr.Tag() {
			case "required":
				messages = append(messages, fmt.Sprintf("field %q is required", vErr.Namespace()))
			case "oneof":
				messages = append(messages, fmt.Sprintf("field %q must be one of [%s], but got %q", vErr.Namespace(), vErr.Param(), vErr.Value()))
			case "disksize":
				messages = append(messages, fmt.Sprintf("field %q must be a valid disk size (e.g., 10G, 500M), but got %q", vErr.Namespace(), vErr.Value()))
			case "url":
				messages = append(messages, fmt.Sprintf("field %q must be a valid URL, but got %q", vErr.Namespace(), vErr.Value()))
			case "hostname":
				messages = append(messages, fmt.Sprintf("field %q must be a valid hostname, but got %q", vErr.Namespace(), vErr.Value()))
			default:
				messages = append(messages, fmt.Sprintf("field %q failed validation on tag %q", vErr.Namespace(), vErr.Tag()))
			}
		}
		return fmt.Errorf("validation failed: %s", strings.Join(messages, "; "))
	}

	return err
}
