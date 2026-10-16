package exit

import (
	"errors"
	"fmt"
)

// Exit codes from PRD 9.7. An empty successful query is still CodeOK.
const (
	CodeOK                = 0
	CodeInternal          = 1
	CodeUsage             = 2
	CodeSourceUnavailable = 3
	CodeBadData           = 4
)

type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

func newError(code int, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

func Usage(format string, args ...any) error {
	return newError(CodeUsage, format, args...)
}

func SourceUnavailable(format string, args ...any) error {
	return newError(CodeSourceUnavailable, format, args...)
}

func BadData(format string, args ...any) error {
	return newError(CodeBadData, format, args...)
}

func FromError(err error) int {
	if err == nil {
		return CodeOK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}
