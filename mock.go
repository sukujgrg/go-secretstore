// SPDX-License-Identifier: Apache-2.0

package secretstore

import (
	"context"
	"sync"
)

// OpenMemory returns an in-memory Store for tests. It is not a production
// fallback and never contacts a native backend.
func OpenMemory() Store {
	return newStore(newMemoryBackend())
}

type memoryBackend struct {
	mu     sync.Mutex
	values map[Key][]byte
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{values: make(map[Key][]byte)}
}

func (*memoryBackend) name() string { return "memory" }

func (m *memoryBackend) get(_ context.Context, key Key) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	if !ok {
		return nil, storeError(NotFound, "get", m.name(), nil)
	}
	return append([]byte(nil), value...), nil
}

func (m *memoryBackend) set(_ context.Context, key Key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = append([]byte(nil), value...)
	return nil
}

func (m *memoryBackend) delete(_ context.Context, key Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	if !ok {
		return storeError(NotFound, "delete", m.name(), nil)
	}
	clear(value)
	delete(m.values, key)
	return nil
}

func (m *memoryBackend) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range m.values {
		clear(value)
		delete(m.values, key)
	}
	return nil
}
