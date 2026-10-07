// Package service holds business rules shared by HTTP handlers and the
// background jobs.
package service

import (
	"fmt"
	"net/http"
)

// Error is a business-rule failure that maps to an HTTP status.
type Error struct {
	Status int
	Code   string
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func newErr(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Msg: fmt.Sprintf(format, args...)}
}

// BadRequest reports invalid input.
func BadRequest(code, format string, args ...any) *Error {
	return newErr(http.StatusBadRequest, code, format, args...)
}

// Conflict reports a request that clashes with the current state.
func Conflict(code, format string, args ...any) *Error {
	return newErr(http.StatusConflict, code, format, args...)
}

// NotFound reports a missing resource.
func NotFound(code, format string, args ...any) *Error {
	return newErr(http.StatusNotFound, code, format, args...)
}

// Forbidden reports an action the caller may not perform.
func Forbidden(code, format string, args ...any) *Error {
	return newErr(http.StatusForbidden, code, format, args...)
}
