// SPDX-License-Identifier: Apache-2.0

package secretstore

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type hookBackend struct {
	mu      sync.Mutex
	values  map[Key][]byte
	setSeen chan []byte
	setWait chan struct{}
}

func newHookBackend() *hookBackend { return &hookBackend{values: make(map[Key][]byte)} }
func (*hookBackend) name() string  { return "memory-test" }

func (m *hookBackend) get(_ context.Context, key Key) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	if !ok {
		return nil, storeError(NotFound, "get", m.name(), nil)
	}
	return append([]byte(nil), value...), nil
}

func (m *hookBackend) set(ctx context.Context, key Key, value []byte) error {
	if m.setSeen != nil {
		m.setSeen <- value
		select {
		case <-m.setWait:
		case <-ctx.Done():
			return contextError("set", m.name(), ctx.Err())
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = append([]byte(nil), value...)
	return nil
}

func (m *hookBackend) delete(_ context.Context, key Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.values[key]; !ok {
		return storeError(NotFound, "delete", m.name(), nil)
	}
	delete(m.values, key)
	return nil
}

func (*hookBackend) close() error { return nil }

func TestStoreContract(t *testing.T) {
	ctx := context.Background()
	store := OpenMemory()
	key := Key{Service: "example.test", Account: "alice"}
	original := []byte("secret-value")
	if err := store.Set(ctx, key, original); err != nil {
		t.Fatal(err)
	}
	clear(original)
	secret, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	first, err := secret.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "secret-value" {
		t.Fatalf("got %q", first)
	}
	first[0] = 'X'
	second, _ := secret.Bytes()
	if string(second) != "secret-value" {
		t.Fatal("Bytes did not return an independent copy")
	}
	owned := secret.value
	if err := secret.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("Secret.Close did not clear the owned buffer")
	}
	if _, err := secret.Bytes(); CodeOf(err) != StoreClosed {
		t.Fatalf("Bytes after Close code = %q", CodeOf(err))
	}
	if err := secret.Close(); err != nil {
		t.Fatal("Secret.Close is not idempotent")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, key); CodeOf(err) != NotFound {
		t.Fatalf("Get missing code = %q", CodeOf(err))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal("Store.Close is not idempotent")
	}
	if err := store.Set(ctx, key, []byte("x")); CodeOf(err) != StoreClosed {
		t.Fatalf("Set after Close code = %q", CodeOf(err))
	}
}

func TestSetDoesNotRetainCallerSlice(t *testing.T) {
	native := newHookBackend()
	native.setSeen = make(chan []byte, 1)
	native.setWait = make(chan struct{})
	store := newStore(native)
	input := []byte("caller-owned")
	done := make(chan error, 1)
	go func() {
		done <- store.Set(context.Background(), Key{Service: "service", Account: "account"}, input)
	}()
	received := <-native.setSeen
	input[0] = 'X'
	if string(received) != "caller-owned" {
		t.Fatal("backend received caller's mutable slice")
	}
	close(native.setWait)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestValidationAndCancellation(t *testing.T) {
	store := OpenMemory()
	for _, key := range []Key{{}, {Service: "s", Account: ""}, {Service: "s\n", Account: "a"}, {Service: "s", Account: string([]byte{0xff})}} {
		if err := store.Set(context.Background(), key, []byte("x")); CodeOf(err) != InvalidInput {
			t.Fatalf("key %#v code = %q", key, CodeOf(err))
		}
	}
	key := Key{Service: "s", Account: "a"}
	if err := store.Set(context.Background(), key, nil); CodeOf(err) != InvalidInput {
		t.Fatalf("empty value code = %q", CodeOf(err))
	}
	if err := store.Set(context.Background(), key, make([]byte, maxSecretBytes+1)); CodeOf(err) != ResourceLimit {
		t.Fatalf("large value code = %q", CodeOf(err))
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Get(cancelled, key); CodeOf(err) != OperationCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Get = %v", err)
	}
	expired, expire := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer expire()
	if _, err := store.Get(expired, key); CodeOf(err) != DeadlineExceeded || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline Get = %v", err)
	}
}

func TestErrorTextDoesNotContainKeyOrSecret(t *testing.T) {
	err := storeError(BackendFailure, "get", "native", errors.New("secret-canary account-canary"))
	if strings.Contains(err.Error(), "canary") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestWindowsTargetIsUnambiguous(t *testing.T) {
	left := windowsTarget(Key{Service: "a/b", Account: "c"})
	right := windowsTarget(Key{Service: "a", Account: "b/c"})
	if left == right || strings.Contains(left, "a/b") || strings.Contains(right, "b/c") {
		t.Fatalf("ambiguous or unencoded targets: %q %q", left, right)
	}
}

func TestWithInteractionRejectsUnknownPolicy(t *testing.T) {
	_, err := Open(context.Background(), WithInteraction(InteractionPolicy(9)))
	if CodeOf(err) != InvalidInput {
		t.Fatalf("code = %q, err = %v", CodeOf(err), err)
	}
}

func TestOpenRejectsNilOptionAndCancelledContext(t *testing.T) {
	if _, err := Open(context.Background(), nil); CodeOf(err) != InvalidInput {
		t.Fatalf("nil option code = %q", CodeOf(err))
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(cancelled); CodeOf(err) != OperationCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Open = %v", err)
	}
}

func TestSecretNilReceiver(t *testing.T) {
	var secret *Secret
	if _, err := secret.Bytes(); CodeOf(err) != StoreClosed {
		t.Fatalf("nil Bytes code = %q", CodeOf(err))
	}
	if err := secret.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGetDeleteValidationAndClosedStore(t *testing.T) {
	store := OpenMemory()
	bad := Key{Service: "s\x00", Account: "a"}
	if _, err := store.Get(context.Background(), bad); CodeOf(err) != InvalidInput {
		t.Fatalf("Get bad key code = %q", CodeOf(err))
	}
	if err := store.Delete(context.Background(), bad); CodeOf(err) != InvalidInput {
		t.Fatalf("Delete bad key code = %q", CodeOf(err))
	}
	key := Key{Service: "s", Account: "a"}
	if err := store.Delete(context.Background(), key); CodeOf(err) != NotFound {
		t.Fatalf("Delete missing code = %q", CodeOf(err))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), key); CodeOf(err) != StoreClosed {
		t.Fatalf("Get after Close code = %q", CodeOf(err))
	}
	if err := store.Delete(context.Background(), key); CodeOf(err) != StoreClosed {
		t.Fatalf("Delete after Close code = %q", CodeOf(err))
	}
}

func TestSetOverwriteAndKeyIsolation(t *testing.T) {
	store := OpenMemory()
	defer func() { _ = store.Close() }()
	alice := Key{Service: "svc", Account: "alice"}
	bob := Key{Service: "svc", Account: "bob"}
	other := Key{Service: "other", Account: "alice"}
	if err := store.Set(context.Background(), alice, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), alice, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), bob, []byte("bob-secret")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), other, []byte("other-secret")); err != nil {
		t.Fatal(err)
	}
	mustSecret(t, store, alice, "two")
	mustSecret(t, store, bob, "bob-secret")
	mustSecret(t, store, other, "other-secret")
}

func TestBinaryAndMultilineSecret(t *testing.T) {
	store := OpenMemory()
	defer func() { _ = store.Close() }()
	key := Key{Service: "svc", Account: "bin"}
	value := []byte("line1\nline2\x00\xff\xfe")
	if err := store.Set(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	secret, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = secret.Close() }()
	got, err := secret.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, value) {
		t.Fatalf("got %q", got)
	}
}

func TestKeyMaxLength(t *testing.T) {
	store := OpenMemory()
	defer func() { _ = store.Close() }()
	ok := Key{Service: strings.Repeat("s", maxKeyPartBytes), Account: strings.Repeat("a", maxKeyPartBytes)}
	if err := store.Set(context.Background(), ok, []byte("x")); err != nil {
		t.Fatal(err)
	}
	tooLong := Key{Service: strings.Repeat("s", maxKeyPartBytes+1), Account: "a"}
	if err := store.Set(context.Background(), tooLong, []byte("x")); CodeOf(err) != InvalidInput {
		t.Fatalf("oversized key code = %q", CodeOf(err))
	}
}

func TestGetRejectsEmptyAndOversizedBackendValue(t *testing.T) {
	backend := &scriptedBackend{getValue: []byte{}}
	store := newStore(backend)
	key := Key{Service: "s", Account: "a"}
	if _, err := store.Get(context.Background(), key); CodeOf(err) != BackendFailure {
		t.Fatalf("empty get code = %q", CodeOf(err))
	}
	backend.getValue = make([]byte, maxSecretBytes+1)
	if _, err := store.Get(context.Background(), key); CodeOf(err) != ResourceLimit {
		t.Fatalf("oversized get code = %q", CodeOf(err))
	}
}

func TestMemoryCloseClearsValues(t *testing.T) {
	backend := newMemoryBackend()
	store := newStore(backend)
	key := Key{Service: "s", Account: "a"}
	if err := store.Set(context.Background(), key, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	owned := backend.values[key]
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if len(backend.values) != 0 {
		t.Fatal("Close left map entries")
	}
	if !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("Close did not clear stored secret bytes")
	}
}

func TestStoreConcurrent(t *testing.T) {
	store := OpenMemory()
	defer func() { _ = store.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := Key{Service: "svc", Account: strings.Repeat("a", i+1)}
			if err := store.Set(context.Background(), key, []byte("v")); err != nil {
				t.Errorf("set: %v", err)
				return
			}
			secret, err := store.Get(context.Background(), key)
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			_, _ = secret.Bytes()
			_ = secret.Close()
			if err := store.Delete(context.Background(), key); err != nil {
				t.Errorf("delete: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestWindowsTargetPrefix(t *testing.T) {
	target := windowsTarget(Key{Service: "svc", Account: "acct"})
	if !strings.HasPrefix(target, "go-secretstore/v1/") {
		t.Fatalf("target = %q", target)
	}
}

type scriptedBackend struct{ getValue []byte }

func (*scriptedBackend) name() string { return "scripted" }

func (s *scriptedBackend) get(context.Context, Key) ([]byte, error) {
	return append([]byte(nil), s.getValue...), nil
}

func (*scriptedBackend) set(context.Context, Key, []byte) error { return nil }
func (*scriptedBackend) delete(context.Context, Key) error      { return nil }
func (*scriptedBackend) close() error                           { return nil }

func mustSecret(t *testing.T, store Store, key Key, want string) {
	t.Helper()
	secret, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = secret.Close() }()
	got, err := secret.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("key %#v got %q, want %q", key, got, want)
	}
}
