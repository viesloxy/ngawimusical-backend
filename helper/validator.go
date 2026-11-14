package helper

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return field.Name
		}
		return name
	})

	return v
}

// Validate memeriksa struct dan mengembalikan AppError berisi field yang gagal.
func Validate(input any) error {
	if input == nil {
		return NewValidationError(map[string]string{"body": "Wajib diisi."})
	}

	err := validate.Struct(input)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return WrapAppError(err, 422, "VALIDATION_FAILED", "Validasi gagal.")
	}

	fields := make(map[string]string, len(validationErrors))
	for _, fieldError := range validationErrors {
		fields[fieldError.Field()] = validationMessage(fieldError)
	}

	return NewValidationError(fields)
}

func validationMessage(fieldError validator.FieldError) string {
	switch fieldError.Tag() {
	case "required":
		return "Wajib diisi."
	case "email":
		return "Format email tidak valid."
	case "url":
		return "Format URL tidak valid."
	case "min":
		return fmt.Sprintf("Minimal %s.", fieldError.Param())
	case "max":
		return fmt.Sprintf("Maksimal %s.", fieldError.Param())
	case "len":
		return fmt.Sprintf("Panjang harus %s.", fieldError.Param())
	case "gte":
		return fmt.Sprintf("Nilai minimal %s.", fieldError.Param())
	case "lte":
		return fmt.Sprintf("Nilai maksimal %s.", fieldError.Param())
	case "oneof":
		return fmt.Sprintf("Harus salah satu dari %s.", fieldError.Param())
	default:
		return "Nilai tidak valid."
	}
}
