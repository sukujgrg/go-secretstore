// SPDX-License-Identifier: Apache-2.0

package secretstore

import (
	"context"
	"errors"
	"testing"
)

func TestCodeOfAndErrorText(t *testing.T) {
	if CodeOf(errors.New("plain")) != BackendFailure {
		t.Fatal("CodeOf(plain) should be backend_failure")
	}
	if CodeOf(nil) != BackendFailure {
		t.Fatal("CodeOf(nil) should be backend_failure")
	}

	plain := storeError(NotFound, "", "", nil)
	if plain.Error() != string(NotFound) {
		t.Fatalf("plain error = %q", plain.Error())
	}
	withOp := storeError(NotFound, "get", "", nil)
	if withOp.Error() != "secretstore get: not_found" {
		t.Fatalf("op error = %q", withOp.Error())
	}
	full := storeError(NotFound, "get", "memory", nil)
	if full.Error() != "secretstore get (memory): not_found" {
		t.Fatalf("full error = %q", full.Error())
	}

	cancelled := contextError("get", "memory", context.Canceled)
	if !errors.Is(cancelled, context.Canceled) || CodeOf(cancelled) != OperationCancelled {
		t.Fatalf("cancel wrap = %v", cancelled)
	}
	deadline := contextError("get", "memory", context.DeadlineExceeded)
	if !errors.Is(deadline, context.DeadlineExceeded) || CodeOf(deadline) != DeadlineExceeded {
		t.Fatalf("deadline wrap = %v", deadline)
	}
	if contextError("get", "memory", errors.New("other")) != nil {
		t.Fatal("contextError should ignore unrelated errors")
	}
}
