// SPDX-License-Identifier: Apache-2.0

package secretstore_test

import (
	"context"

	"github.com/sukujgrg/go-secretstore"
)

func ExampleOpen() {
	ctx := context.Background()
	store, err := secretstore.Open(ctx,
		secretstore.WithInteraction(secretstore.InteractionDenied))
	if err != nil {
		return
	}
	defer store.Close()
	secret, err := store.Get(ctx, secretstore.Key{
		Service: "example.app",
		Account: "deployment/client",
	})
	if err != nil {
		return
	}
	defer secret.Close()
	value, err := secret.Bytes()
	if err != nil {
		return
	}
	defer clear(value)
	// Supply value directly to the bounded operation that needs it.
}

func ExampleOpenMemory() {
	store := secretstore.OpenMemory()
	defer store.Close()
	ctx := context.Background()
	_ = store.Set(ctx, secretstore.Key{Service: "example.app", Account: "alice"}, []byte("secret"))
	secret, err := store.Get(ctx, secretstore.Key{Service: "example.app", Account: "alice"})
	if err != nil {
		return
	}
	defer secret.Close()
}

func ExampleSet() {
	ctx := context.Background()
	if err := secretstore.Set(ctx, "example.app", "alice", []byte("secret")); err != nil {
		return
	}
	secret, err := secretstore.Get(ctx, "example.app", "alice")
	if err != nil {
		return
	}
	defer secret.Close()
	_ = secretstore.Delete(ctx, "example.app", "alice")
}
