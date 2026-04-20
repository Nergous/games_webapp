// internal/errors/grpc.go
package g_errors

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewFromGRPC converts gRPC-error to ServiceError
// Used in controllers for unified processing of gRPC client responses
//
// Mapping of gRPC-codes -> g_errors.Code:
//
// codes.InvalidArgument -> CodeInvalidInput
// codes.NotFound -> CodeNotFound
// codes.AlreadyExists -> CodeConflict
// codes.PermissionDenied -> CodeForbidden
// codes.Unauthenticated -> CodeUnauthorized
// codes.DeadlineExceeded -> CodeTimeout
// else -> CodeInternal
func NewFromGRPC(op string, err error) *ServiceError {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		// not gRPC-error — wrapped as internal
		return Wrap(op, CodeInternal, "", err)
	}

	var code Code
	switch st.Code() {
	case codes.InvalidArgument:
		code = CodeInvalidInput
	case codes.NotFound:
		code = CodeNotFound
	case codes.AlreadyExists:
		code = CodeConflict
	case codes.PermissionDenied:
		code = CodeForbidden
	case codes.Unauthenticated:
		code = CodeUnauthorized
	case codes.DeadlineExceeded:
		code = CodeTimeout
	default:
		code = CodeInternal
	}

	return NewWithInfo(op, code, "", map[string]any{
		"grpc_code":    st.Code().String(),
		"grpc_message": st.Message(),
	})
}
