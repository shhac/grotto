package errors

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ClassifyGRPCError converts a gRPC error into a UIError with user-friendly
// messages, recovery suggestions, and appropriate actions.
func ClassifyGRPCError(err error) *UIError {
	if err == nil {
		return nil
	}

	// Try to extract gRPC status
	st, ok := status.FromError(err)
	if !ok {
		// Not a gRPC error, fall back to standard classification
		return ClassifyError(err)
	}

	// Build details string with gRPC code and message
	details := fmt.Sprintf("gRPC: %s - %s", st.Code(), st.Message())

	switch st.Code() {
	case codes.Unavailable:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Cannot Connect to Server",
			Message:  "The server is not responding.",
			Recovery: []string{
				"Check that the server is running",
				"Verify the address and port",
				"Check your network connection",
			},
			Actions: []ErrorAction{{Label: "Retry"}, {Label: "Edit Connection"}},
			Details: details,
		}

	case codes.DeadlineExceeded:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Request Timeout",
			Message:  "The server took too long to respond.",
			Recovery: []string{"Try again", "Increase timeout setting"},
			Actions:  []ErrorAction{{Label: "Retry"}, {Label: "Settings"}},
			Details:  details,
		}

	case codes.Unauthenticated:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Authentication Required",
			Message:  "You need to authenticate to access this service.",
			Recovery: []string{"Add credentials in metadata"},
			Actions:  []ErrorAction{{Label: "Add Credentials"}},
			Details:  details,
		}

	case codes.PermissionDenied:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Access Denied",
			Message:  "You don't have permission to call this method.",
			Recovery: []string{"Contact administrator for access"},
			Details:  details,
		}

	case codes.InvalidArgument:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Invalid Request",
			Message:  "The request contains invalid data.",
			Recovery: []string{"Check field values", "See details for specifics"},
			Actions:  []ErrorAction{{Label: "View Details"}, {Label: "Edit Request"}},
			Details:  st.Message(),
		}

	case codes.Internal:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Server Error",
			Message:  "The server encountered an unexpected error.",
			Recovery: []string{"Try again later", "Contact server administrator"},
			Actions:  []ErrorAction{{Label: "Retry"}},
			Details:  details,
		}

	case codes.Unimplemented:
		return &UIError{
			Err:      err,
			Severity: SeverityWarning,
			Title:    "Method Not Available",
			Message:  "This method is not implemented on the server.",
			Recovery: []string{"Check method name", "Verify server version"},
			Details:  details,
		}

	case codes.NotFound:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Not Found",
			Message:  "The requested resource was not found.",
			Recovery: []string{"Check the request parameters"},
			Details:  details,
		}

	case codes.AlreadyExists:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Already Exists",
			Message:  "The resource already exists.",
			Recovery: []string{"Use a different identifier"},
			Details:  details,
		}

	case codes.ResourceExhausted:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Resource Exhausted",
			Message:  "The server has insufficient resources.",
			Recovery: []string{"Try again later", "Reduce request size"},
			Actions:  []ErrorAction{{Label: "Retry"}},
			Details:  details,
		}

	case codes.FailedPrecondition:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Failed Precondition",
			Message:  "The operation was rejected due to system state.",
			Recovery: []string{"Check system state", "See details for more info"},
			Details:  st.Message(),
		}

	case codes.Aborted:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Operation Aborted",
			Message:  "The operation was aborted, typically due to concurrency issues.",
			Recovery: []string{"Try again"},
			Actions:  []ErrorAction{{Label: "Retry"}},
			Details:  details,
		}

	case codes.OutOfRange:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Out of Range",
			Message:  "A value is out of the valid range.",
			Recovery: []string{"Check input values", "See details for specifics"},
			Details:  st.Message(),
		}

	case codes.DataLoss:
		return &UIError{
			Err:      err,
			Severity: SeverityFatal,
			Title:    "Data Loss",
			Message:  "Unrecoverable data loss or corruption.",
			Recovery: []string{"Contact server administrator immediately"},
			Details:  details,
		}

	case codes.Canceled:
		return &UIError{
			Err:      err,
			Severity: SeverityInfo,
			Title:    "Request Cancelled",
			Message:  "The operation was cancelled.",
			Recovery: []string{},
			Details:  details,
		}

	case codes.Unknown:
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Unknown Error",
			Message:  st.Message(),
			Recovery: []string{"Try again", "Contact server administrator if problem persists"},
			Actions:  []ErrorAction{{Label: "Retry"}},
			Details:  details,
		}

	default:
		// Fallback for any other gRPC codes
		return &UIError{
			Err:      err,
			Severity: SeverityError,
			Title:    "Request Failed",
			Message:  st.Message(),
			Recovery: []string{"Try again"},
			Details:  details,
		}
	}
}
