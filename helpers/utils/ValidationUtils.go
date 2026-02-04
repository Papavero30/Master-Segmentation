package utils

import (
	"fmt"
	"regexp"
	"strings"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Validation failed: ")
	for i, err := range ve {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(fmt.Sprintf("%s: %s", err.Field, err.Message))
	}
	return sb.String()
}

type Validator struct {
	errors ValidationErrors
}

func NewValidator() *Validator {
	return &Validator{errors: ValidationErrors{}}
}

func (v *Validator) ValidateRequired(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "This field is required",
		})
	}
	return v
}

func (v *Validator) ValidateMinLength(field, value string, minLength int) *Validator {
	if len(strings.TrimSpace(value)) < minLength {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("Must be at least %d characters long", minLength),
		})
	}
	return v
}

func (v *Validator) ValidateMaxLength(field, value string, maxLength int) *Validator {
	if len(value) > maxLength {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fmt.Sprintf("Must be no more than %d characters long", maxLength),
		})
	}
	return v
}

func (v *Validator) ValidateEmail(field, value string) *Validator {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if value != "" && !emailRegex.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "Invalid email format",
		})
	}
	return v
}

func (v *Validator) ValidatePhoneNumber(field, value string) *Validator {
	phoneRegex := regexp.MustCompile(`^[0-9+\-\s]{8,15}$`)
	if value != "" && !phoneRegex.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "Invalid phone number format",
		})
	}
	return v
}

func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

func (v *Validator) Errors() ValidationErrors {
	return v.errors
}
