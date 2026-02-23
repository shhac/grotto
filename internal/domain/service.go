package domain

// Service represents a gRPC service discovered via reflection
type Service struct {
	Name     string
	FullName string // Fully qualified name
	Methods  []Method
	Error    string // non-empty when descriptor resolution failed
}

// Method represents a gRPC method
type Method struct {
	Name           string
	FullName       string
	InputType      string // Message type name
	OutputType     string
	IsClientStream bool
	IsServerStream bool
}

// MethodType returns the RPC type (Unary, ServerStream, ClientStream, or BidiStream)
func (m Method) MethodType() string {
	if m.IsClientStream && m.IsServerStream {
		return "BidiStream"
	}
	if m.IsServerStream {
		return "ServerStream"
	}
	if m.IsClientStream {
		return "ClientStream"
	}
	return "Unary"
}
