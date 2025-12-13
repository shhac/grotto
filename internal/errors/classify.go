package errors

import (
	"context"
	"errors"
)

// ErrorSeverity indicates the severity of an error for UI presentation.
type ErrorSeverity int

const (
	SeverityInfo    ErrorSeverity = iota // User should know, not blocking
	SeverityWarning                      // Degraded functionality
	SeverityError                        // Operation failed, can retry
	SeverityFatal                        // Application must exit
)

// ErrorAction represents a user action that can be taken in response to an error.
type ErrorAction struct {
	Label   string
	Handler func()
}

// UIError wraps an error with UI-friendly presentation metadata.
type UIError struct {
	Err      error
	Severity ErrorSeverity
	Title    string        // Short user-facing title
	Message  string        // Detailed user-facing message
	Recovery []string      // Suggested actions (bullet points)
	Actions  []ErrorAction // Buttons for user actions
	Details  string        // Technical details (collapsed by default)
}

func (e UIError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Title
}

// Unwrap returns the underlying error.
func (e UIError) Unwrap() error {
	return e.Err
}

// ClassifyError converts a standard error into a UIError with appropriate
// severity, title, message, and recovery suggestions.
func ClassifyError(err error) *UIError {
	if err == nil {
		return nil
	}

	// Check if already a UIError
	var uiErr *UIError
	if errors.As(err, &uiErr) {
		return uiErr
	}

	// Context errors
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Request Timeout",
			Message:  "The server took too long to respond.",
			Recovery: []string{"Try again", "Increase the timeout setting"},
			Actions:  []ErrorAction{{Label: "Retry"}, {Label: "Settings"}},
		}

	case errors.Is(err, context.Canceled):
		return &UIError{
			Err:      err,
			Severity: SeverityInfo,
			Title:    "Request Cancelled",
			Message:  "The operation was cancelled.",
			Recovery: []string{},
		}

	case errors.Is(err, ErrUserCancelled):
		return &UIError{
			Err:      err,
			Severity: SeverityInfo,
			Title:    "Cancelled",
			Message:  "Operation cancelled by user.",
			Recovery: []string{},
		}

	case errors.Is(err, ErrConnectionFailed):
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Connection Failed",
			Message:  "Unable to connect to the server.",
			Recovery: []string{
				"Check that the server is running",
				"Verify the address and port",
				"Check your network connection",
			},
			Actions: []ErrorAction{{Label: "Retry"}, {Label: "Edit Connection"}},
		}

	case errors.Is(err, ErrReflectionUnavailable):
		return &UIError{
			Err:      err,
			Severity: SeverityWarning,
			Title:    "Reflection Not Available",
			Message:  "This server doesn't support gRPC reflection.",
			Recovery: []string{"Import proto files manually"},
			Actions:  []ErrorAction{{Label: "Import Proto Files"}},
		}

	case errors.Is(err, ErrInvalidDescriptor):
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Invalid Descriptor",
			Message:  "The server returned an invalid proto descriptor.",
			Recovery: []string{
				"Check server configuration",
				"Import proto files manually",
			},
			Details: err.Error(),
		}

	case errors.Is(err, ErrTimeout):
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Operation Timeout",
			Message:  "The operation timed out.",
			Recovery: []string{"Try again", "Increase timeout setting"},
			Actions:  []ErrorAction{{Label: "Retry"}},
		}
	}

	// Validation errors
	var validationErr ValidationError
	if errors.As(err, &validationErr) {
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Validation Error",
			Message:  validationErr.Message,
			Recovery: []string{"Correct the field value and try again"},
			Details:  validationErr.Error(),
		}
	}

	// Default fallback for unknown errors
	return &UIError{
		Err:      err,
		Severity: SeverityError,
		Title:    "Unexpected Error",
		Message:  "An unexpected error occurred.",
		Recovery: []string{"Try again"},
		Details:  err.Error(),
	}
}
