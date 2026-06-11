package rest

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Error is returned for any response with a 4xx or 5xx status code. The
// Prefix is the owning package name (set via WithErrorPrefix); the zero
// value formats without one.
type Error struct {
	Prefix     string
	Method     string
	Path       string
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}

	if e.Prefix == "" {
		return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.StatusCode, msg)
	}

	return fmt.Sprintf("%s: %s %s: %d %s", e.Prefix, e.Method, e.Path, e.StatusCode, msg)
}

// errorBodyLimit caps how much of an error response body is kept.
const errorBodyLimit = 4 << 10

func (c *Client) checkResponse(resp *http.Response, method, path string) error {
	if resp.StatusCode < 400 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
	return &Error{
		Prefix:     c.errPrefix,
		Method:     method,
		Path:       path,
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(body)),
	}
}

// HasStatus reports whether err is an *Error with the given status code.
func HasStatus(err error, code int) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == code
}

// IsNotFound reports whether err is an *Error with status 404.
func IsNotFound(err error) bool { return HasStatus(err, http.StatusNotFound) }

// IsUnauthorized reports whether err is an *Error with status 401.
func IsUnauthorized(err error) bool { return HasStatus(err, http.StatusUnauthorized) }

// IsForbidden reports whether err is an *Error with status 403.
func IsForbidden(err error) bool { return HasStatus(err, http.StatusForbidden) }

// IsBadRequest reports whether err is an *Error with status 400.
func IsBadRequest(err error) bool { return HasStatus(err, http.StatusBadRequest) }
