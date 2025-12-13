package errors

import "errors"

// Sentinel errors for common failure modes.
var (
	ErrConnectionFailed      = errors.New("connection failed")
	ErrReflectionUnavailable = errors.New("reflection not available")
	ErrInvalidDescriptor     = errors.New("invalid descriptor")
	ErrUserCancelled         = errors.New("user cancelled operation")
	ErrTimeout               = errors.New("operation timed out")
)

// ValidationError represents a field validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}
