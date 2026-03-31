package apperr

import (
	"errors"
	"fmt"
)

// Code defines stable, machine-readable error categories.
type Code string

const (
	CodeValidation Code = "VALIDATION_ERROR"
	CodePlatform   Code = "PLATFORM_ERROR"
	CodeCommand    Code = "COMMAND_ERROR"
	CodeConfig     Code = "CONFIG_ERROR"
	CodeInternal   Code = "INTERNAL_ERROR"
)

// Error is a structured application error.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func IsCode(err error, code Code) bool {
	var appErr *Error
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Code == code
}
