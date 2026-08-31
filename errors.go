// SPDX-License-Identifier: Apache-2.0

package secretstore

import (
	"context"
	"errors"
	"fmt"
)

// ErrorCode is a stable category suitable for policy decisions. Error text
// never contains a secret value or store key.
type ErrorCode string

const (
	InvalidInput        ErrorCode = "invalid_input"
	NotFound            ErrorCode = "not_found"
	PermissionDenied    ErrorCode = "permission_denied"
	InteractionRequired ErrorCode = "interaction_required"
	Unsupported         ErrorCode = "unsupported"
	ResourceLimit       ErrorCode = "resource_limit"
	OperationCancelled  ErrorCode = "operation_cancelled"
	DeadlineExceeded    ErrorCode = "deadline_exceeded"
	StoreClosed         ErrorCode = "store_closed"
	BackendFailure      ErrorCode = "backend_failure"
)

// Error is safe to return across application boundaries. Cause is retained
// only for errors.Is cancellation/deadline checks and is not rendered.
type Error struct {
	Code    ErrorCode
	Op      string
	Backend string
	cause   error
}

func (e *Error) Error() string {
	if e.Op == "" && e.Backend == "" {
		return string(e.Code)
	}
	if e.Backend == "" {
		return fmt.Sprintf("secretstore %s: %s", e.Op, e.Code)
	}
	return fmt.Sprintf("secretstore %s (%s): %s", e.Op, e.Backend, e.Code)
}

func (e *Error) Unwrap() error { return e.cause }

// CodeOf returns BackendFailure for errors outside this package.
func CodeOf(err error) ErrorCode {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return BackendFailure
}

func storeError(code ErrorCode, op, backend string, cause error) error {
	return &Error{Code: code, Op: op, Backend: backend, cause: cause}
}

func contextError(op, backend string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return storeError(DeadlineExceeded, op, backend, context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return storeError(OperationCancelled, op, backend, context.Canceled)
	}
	return nil
}

// cancellationError preserves a caller cancellation cause only when the
// supplied context was actually cancelled. Native user-interface cancellation
// is otherwise reported without pretending that the caller cancelled ctx.
func cancellationError(ctx context.Context, op, backend string) error {
	if err := contextError(op, backend, ctx.Err()); err != nil {
		return err
	}
	return storeError(OperationCancelled, op, backend, nil)
}
