package serve

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/timescale/ghost/internal/serve/api"
)

// RequiredFieldError is the error type returned when a required request body
// field is missing.
type RequiredFieldError struct {
	Field string
}

// Error implements the error interface.
func (e *RequiredFieldError) Error() string {
	return fmt.Sprintf("missing required field: '%s'", e.Field)
}

type InvalidContentTypeError struct {
	Required string
}

func (e *InvalidContentTypeError) Error() string {
	return fmt.Sprintf("missing required request header: 'Content-Type: %s'", e.Required)
}

type InvalidJSONBodyError struct {
	Err error
}

func (e *InvalidJSONBodyError) Error() string {
	if e.Err == nil {
		return "invalid JSON body"
	}
	return fmt.Sprintf("invalid JSON body: %s", e.Err)
}

type SessionIDConflictError struct {
	ID uuid.UUID
}

func (e *SessionIDConflictError) Error() string {
	return fmt.Sprintf("session with ID already exists: %s", e.ID)
}

type InvalidSessionIDError struct {
	ID uuid.UUID
}

func (e *InvalidSessionIDError) Error() string {
	return fmt.Sprintf("invalid session ID: %s", e.ID)
}

type RunIDConflictError struct {
	ID uuid.UUID
}

func (e *RunIDConflictError) Error() string {
	return fmt.Sprintf("run with ID already exists: %s", e.ID)
}

type InvalidRunIDError struct {
	ID uuid.UUID
}

func (e *InvalidRunIDError) Error() string {
	return fmt.Sprintf("invalid run ID: %s", e.ID)
}

type RunFormatError struct {
	RunID  uuid.UUID
	Format api.OutputFormat
}

func (e *RunFormatError) Error() string {
	return fmt.Sprintf("run is not configured to return results in %s format: %s", e.Format, e.RunID)
}

type ResultsUnavailableError struct {
	RunID uuid.UUID
}

func (e *ResultsUnavailableError) Error() string {
	return fmt.Sprintf("results are unavailable for run: %s", e.RunID)
}
