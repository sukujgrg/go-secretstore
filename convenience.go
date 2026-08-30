// SPDX-License-Identifier: Apache-2.0

package secretstore

import "context"

// Get opens the native store, reads one secret, and closes the store.
func Get(ctx context.Context, service, account string, opts ...Option) (*Secret, error) {
	store, err := Open(ctx, opts...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()
	return store.Get(ctx, Key{Service: service, Account: account})
}

// Set opens the native store, writes one secret, and closes the store.
func Set(ctx context.Context, service, account string, secret []byte, opts ...Option) error {
	store, err := Open(ctx, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.Set(ctx, Key{Service: service, Account: account}, secret)
}

// Delete opens the native store, removes one secret, and closes the store.
func Delete(ctx context.Context, service, account string, opts ...Option) error {
	store, err := Open(ctx, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.Delete(ctx, Key{Service: service, Account: account})
}
