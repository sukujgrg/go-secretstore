// SPDX-License-Identifier: Apache-2.0

package secretstore

import (
	"context"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	maxKeyPartBytes = 512
	maxSecretBytes  = 64 * 1024
)

// InteractionPolicy controls whether the native store may display its own
// authentication or unlock UI. Secretstore itself never renders prompts.
type InteractionPolicy uint8

const (
	InteractionDenied InteractionPolicy = iota
	InteractionAllowed
)

// Key identifies one generic secret. Service is the owning application or
// protocol namespace; Account is the name within that namespace.
type Key struct {
	Service string
	Account string
}

// Store is safe for concurrent use. Close is idempotent and waits for active
// operations to finish.
type Store interface {
	Get(context.Context, Key) (*Secret, error)
	Set(context.Context, Key, []byte) error
	Delete(context.Context, Key) error
	Close() error
}

type options struct {
	interaction InteractionPolicy
}

// Option configures Open. Options are validated before a native store is
// contacted.
type Option func(*options) error

// WithInteraction selects whether the native store may request user
// interaction. The secure default is InteractionDenied.
func WithInteraction(policy InteractionPolicy) Option {
	return func(value *options) error {
		if policy != InteractionDenied && policy != InteractionAllowed {
			return storeError(InvalidInput, "open", "", nil)
		}
		value.interaction = policy
		return nil
	}
}

// Open selects the native backend for the current platform. There is no file,
// environment, command-line, or alternate-store fallback.
func Open(ctx context.Context, supplied ...Option) (Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError("open", "", err)
	}
	configuration := options{interaction: InteractionDenied}
	for _, option := range supplied {
		if option == nil {
			return nil, storeError(InvalidInput, "open", "", nil)
		}
		if err := option(&configuration); err != nil {
			return nil, err
		}
	}
	native, err := openNative(ctx, configuration)
	if err != nil {
		return nil, err
	}
	return newStore(native), nil
}

type backend interface {
	name() string
	get(context.Context, Key) ([]byte, error)
	set(context.Context, Key, []byte) error
	delete(context.Context, Key) error
	close() error
}

// runNativeMutation checks cancellation before entering a synchronous native
// mutation. Once mutate starts, its result is authoritative: reporting a
// context error after native success would leave the caller unable to tell
// whether the mutation took effect.
func runNativeMutation(ctx context.Context, op, backend string, mutate func() error) error {
	if err := ctx.Err(); err != nil {
		return contextError(op, backend, err)
	}
	return mutate()
}

type store struct {
	mu      sync.RWMutex
	closed  bool
	backend backend
}

func newStore(value backend) *store { return &store{backend: value} }

func (s *store) Get(ctx context.Context, key Key) (*Secret, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, storeError(StoreClosed, "get", s.backend.name(), nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, contextError("get", s.backend.name(), err)
	}
	value, err := s.backend.get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(value) == 0 {
		clear(value)
		return nil, storeError(BackendFailure, "get", s.backend.name(), nil)
	}
	if len(value) > maxSecretBytes {
		clear(value)
		return nil, storeError(ResourceLimit, "get", s.backend.name(), nil)
	}
	return &Secret{value: value}, nil
}

func (s *store) Set(ctx context.Context, key Key, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if len(value) == 0 {
		return storeError(InvalidInput, "set", "", nil)
	}
	if len(value) > maxSecretBytes {
		return storeError(ResourceLimit, "set", "", nil)
	}
	owned := append([]byte(nil), value...)
	defer clear(owned)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return storeError(StoreClosed, "set", s.backend.name(), nil)
	}
	if err := ctx.Err(); err != nil {
		return contextError("set", s.backend.name(), err)
	}
	return s.backend.set(ctx, key, owned)
}

func (s *store) Delete(ctx context.Context, key Key) error {
	if err := validateKey(key); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return storeError(StoreClosed, "delete", s.backend.name(), nil)
	}
	if err := ctx.Err(); err != nil {
		return contextError("delete", s.backend.name(), err)
	}
	return s.backend.delete(ctx, key)
}

func (s *store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if err := s.backend.close(); err != nil {
		return storeError(BackendFailure, "close", s.backend.name(), nil)
	}
	return nil
}

func validateKey(key Key) error {
	valid := func(value string) bool {
		return value != "" && len(value) <= maxKeyPartBytes && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
	}
	if !valid(key.Service) || !valid(key.Account) {
		return storeError(InvalidInput, "key", "", nil)
	}
	return nil
}

// Secret owns the returned native-store bytes. Close clears that owned buffer
// and is idempotent. Bytes returns a caller-owned copy; Go and native runtimes
// may make additional copies that this package cannot erase.
type Secret struct {
	mu     sync.RWMutex
	value  []byte
	closed bool
}

func (s *Secret) Bytes() ([]byte, error) {
	if s == nil {
		return nil, storeError(StoreClosed, "secret", "", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, storeError(StoreClosed, "secret", "", nil)
	}
	return append([]byte(nil), s.value...), nil
}

func (s *Secret) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	clear(s.value)
	s.value = nil
	s.closed = true
	return nil
}
