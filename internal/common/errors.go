package common

import (
	"errors"
	"fmt"

	"github.com/timescale/ghost/internal/api"
)

// Exit codes as defined in the CLI specification
const (
	ExitSuccess             = 0 // Success
	ExitGeneralError        = 1 // General error
	ExitTimeout             = 2 // Operation timeout (wait-timeout exceeded) or connection timeout
	ExitInvalidParameters   = 3 // Invalid parameters
	ExitAuthenticationError = 4 // Authentication error
	ExitPermissionDenied    = 5 // Permission denied
	ExitDatabaseNotFound    = 6 // Database not found
	ExitUpdateAvailable     = 7 // Update available
)

// ExitCodeError creates an error that will cause the program to exit with the specified code
type ExitCodeError struct {
	code int
	err  error
}

func (e ExitCodeError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e ExitCodeError) ExitCode() int {
	return e.code
}

func (e ExitCodeError) Unwrap() error {
	return e.err
}

// ExitWithCode returns an error that will cause the program to exit with the specified code
func ExitWithCode(code int, err error) error {
	return ExitCodeError{code: code, err: err}
}

// IsNoPaymentMethod reports whether an API error indicates that a payment
// method is required for the operation but none is on file.
func IsNoPaymentMethod(apiErr *api.Error) bool {
	return apiErr != nil && apiErr.Code != nil && *apiErr.Code == api.ErrorCodeNoPaymentMethod
}

// NoPaymentMethodError returns a user-friendly error explaining that a payment
// method is required for the given action, with guidance on how to add one.
// The action should complete the sentence "a payment method is required to ..."
// (e.g. "create a dedicated database"). It carries the invalid-parameters exit
// code, matching the 400 the API returns for this condition.
func NoPaymentMethodError(action string) error {
	return ExitWithCode(ExitInvalidParameters,
		fmt.Errorf("a payment method is required to %s\n\nAdd one with 'ghost payment add', then try again", action))
}

// IsComputeLimitExceeded reports whether an API error indicates the operation
// was rejected because the space has used up its included compute allowance.
func IsComputeLimitExceeded(apiErr *api.Error) bool {
	return apiErr != nil && apiErr.Code != nil && *apiErr.Code == api.ErrorCodeComputeLimitExceeded
}

// ComputeLimitExceededError returns a user-friendly error explaining that the
// space has reached its compute limit, with guidance on enabling overages. The
// action should complete the sentence "you can't ..." (e.g. "create a
// database"). It carries the invalid-parameters exit code, matching the 400 the
// API returns for this condition.
func ComputeLimitExceededError(action string) error {
	return ExitWithCode(ExitInvalidParameters,
		fmt.Errorf("this space has reached its compute limit, so you can't %s\n\nRaise or remove the limit with 'ghost overages enable', or wait until your allowance\nresets next cycle", action))
}

// ExitWithErrorFromStatusCode maps HTTP status codes to CLI exit codes
func ExitWithErrorFromStatusCode(statusCode int, err error) error {
	if err == nil {
		err = errors.New("unknown error")
	}
	switch statusCode {
	case 400:
		// Bad request - invalid parameters
		return ExitWithCode(ExitInvalidParameters, err)
	case 401:
		// Unauthorized - authentication error
		return ExitWithCode(ExitAuthenticationError, err)
	case 403:
		// Forbidden - permission denied
		return ExitWithCode(ExitPermissionDenied, err)
	case 404:
		// Not found - database/resource not found
		return ExitWithCode(ExitDatabaseNotFound, err)
	case 408, 504:
		// Request timeout or gateway timeout
		return ExitWithCode(ExitTimeout, err)
	default:
		// For other 4xx errors, use general error
		if statusCode >= 400 && statusCode < 500 {
			return ExitWithCode(ExitGeneralError, err)
		}
		// For 5xx and other errors, use general error
		return ExitWithCode(ExitGeneralError, err)
	}
}
