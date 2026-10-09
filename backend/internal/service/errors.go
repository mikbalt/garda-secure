// Package service holds business rules shared by HTTP handlers and the
// background jobs.
package service

import (
	"fmt"
	"net/http"
	"unicode/utf8"
)

// Error is a business-rule failure that maps to an HTTP status.
type Error struct {
	Status int
	Code   string
	Msg    string
	Field  string // offending input field, when known
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

// Owned is a record created through idempotent tablet sync.
type Owned interface {
	// Owner returns the scope (site, or session for scans) and the guard
	// that created the record.
	Owner() (scope, guardID string)
}

// CheckReplay guards idempotent creates (ON CONFLICT (id) DO NOTHING, then
// load the existing row): a repeat is only answered with the stored record
// when it was created in the same scope by the same guard. Otherwise the
// client reused an ID it does not own (e.g. another site's or realm's
// record) and gets 409 id_conflict instead of that record.
func CheckReplay(rec Owned, created bool, scope, guardID string) error {
	if created {
		return nil
	}
	s, g := rec.Owner()
	if s != scope || g != guardID {
		return Conflict("id_conflict", "this id is already used by another record")
	}
	return nil
}

// WithField records which input field the error is about.
func (e *Error) WithField(field string) *Error {
	e.Field = field
	return e
}

// TooLong reports a free-text field over its length limit.
func TooLong(field string, max int) *Error {
	return BadRequest("field_too_long", "%s is too long (max %d characters)", field, max).WithField(field)
}

// Field is one free-text input to length-check.
type Field struct {
	Name  string
	Value string
	Max   int // characters (runes)
}

// F is shorthand for a Field from an optional value.
func F(name string, v *string, max int) Field {
	if v == nil {
		return Field{Name: name, Max: max}
	}
	return Field{Name: name, Value: *v, Max: max}
}

// S is shorthand for a Field from a plain string.
func S(name, v string, max int) Field { return Field{Name: name, Value: v, Max: max} }

// CheckLengths fails with field_too_long on the first field over its limit.
func CheckLengths(fields ...Field) error {
	for _, f := range fields {
		if len(f.Value) > f.Max && utf8.RuneCountInString(f.Value) > f.Max {
			return TooLong(f.Name, f.Max)
		}
	}
	return nil
}

// Length limits shared by the handlers (characters).
const (
	MaxName        = 200  // people, sites, routes, checkpoints, devices
	MaxShortText   = 100  // categories, types, ID types, codes
	MaxLocation    = 300  // free-text locations
	MaxDescription = 4000 // incident/defect descriptions
	MaxReason      = 1000 // override/amendment reasons, notes, purposes
	MaxAddress     = 500
	MaxIdentifier  = 64 // employee IDs, ID card numbers, plates
)
