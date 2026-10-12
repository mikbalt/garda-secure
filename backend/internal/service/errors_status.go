package service

import "net/http"

// Unavailable reports a dependency that cannot serve the request (503),
// e.g. Keycloak for user management.
func Unavailable(code, format string, args ...any) *Error {
	return newErr(http.StatusServiceUnavailable, code, format, args...)
}
