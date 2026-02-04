package utils

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type AppError struct {
	StatusCode int                    `json:"-"`
	Message    string                 `json:"message"`
	Code       string                 `json:"code,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	DetailsStr map[string]string      `json:"details_str,omitempty"`
	Err        error                  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func AsAppError(err error, target **AppError) bool {
	return errors.As(err, target)
}

func IsNotFoundError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.StatusCode == http.StatusNotFound
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func NewBadRequestError(message string) *AppError {
	return &AppError{
		StatusCode: http.StatusBadRequest,
		Message:    message,
	}
}

func NewNotFoundError(resource string, id interface{}) *AppError {
	return &AppError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("%s with ID %v not found", resource, id),
	}
}

func NewInternalServerError(message string, err error) *AppError {
	return &AppError{
		StatusCode: http.StatusInternalServerError,
		Message:    message,
		Err:        err,
	}
}

func NewValidationError(message string, details map[string]string) *AppError {
	detailsInterface := make(map[string]interface{})
	for k, v := range details {
		detailsInterface[k] = v
	}
	return &AppError{
		StatusCode: http.StatusBadRequest,
		Message:    message,
		Details:    detailsInterface,
		DetailsStr: details,
	}
}

func NewTooManyRequestsError(message string) *AppError {
	return &AppError{StatusCode: http.StatusTooManyRequests, Message: message}
}

func NewUnauthorizedError(message string) *AppError {
	return &AppError{StatusCode: http.StatusUnauthorized, Message: message}
}

func NewForbiddenError(message string) *AppError {
	return &AppError{StatusCode: http.StatusForbidden, Message: message}
}

func NewConflictError(resource string, id interface{}) *AppError {
	return &AppError{
		StatusCode: http.StatusConflict,
		Message:    fmt.Sprintf("%s with ID %v already exists", resource, id),
	}
}

func NewRateLimitErrorWithDetails(message, code string, details map[string]interface{}) *AppError {
	return &AppError{
		StatusCode: http.StatusTooManyRequests,
		Message:    message,
		Code:       code,
		Details:    details,
	}
}
