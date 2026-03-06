package api

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidatorOpts func(*validator.Validate)

func WithCustomValidations(customValidations map[string]validator.Func) ValidatorOpts {
	return func(val *validator.Validate) {
		for tag, fn := range customValidations {
			_ = val.RegisterValidation(tag, fn)
		}
	}
}

func WithYAMLFieldNames() ValidatorOpts {
	return func(val *validator.Validate) {
		val.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
			switch name {
			case "-":
				// return empty name if yaml filed is ignored
				return ""
			case "":
				// defer to the original field name if yaml name is empty (e.g. ",inline")
				return fld.Name
			default:
				return name
			}
		})
	}
}

func NewValidator(opts ...ValidatorOpts) *validator.Validate {
	validate := validator.New(validator.WithRequiredStructEnabled())
	for _, opt := range opts {
		opt(validate)
	}
	return validate
}

func FormatErrors(errs validator.ValidationErrors) error {
	var messages []string
	for _, err := range errs {
		switch err.Tag() {
		case "required":
			messages = append(messages, fmt.Sprintf("field %q is required", err.Namespace()))
		case "oneof":
			messages = append(messages, fmt.Sprintf("field %q must be one of [%s], but got %q", err.Namespace(), err.Param(), err.Value()))
		case "url":
			messages = append(messages, fmt.Sprintf("field %q must be a valid URL, but got %q", err.Namespace(), err.Value()))
		default:
			messages = append(messages, fmt.Sprintf("field %q failed validation on tag %q", err.Namespace(), err.Tag()))
		}
	}
	return errors.New(strings.Join(messages, "; "))
}
