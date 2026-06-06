package api

import (
	"errors"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrMethodNotAllowed = errors.New("method not allowed")
	ErrInternalServer   = errors.New("something went wrong")
)

// NormalizedError represents an error returned from the underlying database.
// It includes a variety of optional fields which are populated depending on
// the underlying database driver implementation. Only the source and message
// fields are guaranteed to be present.
type NormalizedError struct {
	Code     string `json:"code,omitempty"`
	Column   int32  `json:"column,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Line     int32  `json:"line,omitempty"`
	Message  string `json:"message"`
	Position int32  `json:"position,omitempty"`

	// The name of the client.
	Source string `json:"source"`

	// Connect indicates that the error occurred while connecting to the
	// database.
	Connect bool `json:"connect,omitempty"`

	// Fatal indicates whether the underlying session/connection is broken.
	Fatal bool `json:"fatal,omitempty"`

	// Timeout indicates whether the run timed out.
	Timeout bool `json:"timeout,omitempty"`

	// Cancel indicates whether the run was canceled.
	Cancel bool `json:"cancel,omitempty"`
}

// Error implements the error interface.
func (e *NormalizedError) Error() string {
	return e.Message
}

// Error represents the error field of an [ErrorResponse].
type Error struct {
	Message string `json:"message"`
}

// ErrorResponse represents the structure of an error response body as returned
// from one of the API endpoints.
type ErrorResponse struct {
	Error   Error `json:"error"`
	Success bool  `json:"success"`
}

// NewErrorResponse constructs a new [ErrorResponse] from the provided [Error].
func NewErrorResponse(err error) ErrorResponse {
	return ErrorResponse{
		Error: Error{
			Message: err.Error(),
		},
		Success: false,
	}
}

// NormalizedErrorResponse represents the structure of an error response body
// as returned from one of the API endpoints when the error is a
// [NormalizedError]. Because the fields of [NormalizedError] are a superset of
// the [Error] fields, the response structure is compatible with
// [ErrorResponse].
type NormalizedErrorResponse struct {
	Error   *NormalizedError `json:"error"`
	Success bool             `json:"success"`
}

// NewNormalizedErrorResponse constructs a new [NormalizedErrorResponse] from
// the provided [NormalizedError].
func NewNormalizedErrorResponse(err *NormalizedError) NormalizedErrorResponse {
	return NormalizedErrorResponse{
		Error:   err,
		Success: false,
	}
}
