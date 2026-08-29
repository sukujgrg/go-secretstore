// SPDX-License-Identifier: Apache-2.0
//go:build secretstore_native

package secretstore_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/sukujgrg/go-secretstore"
)

func TestPlatformNativeStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := secretstore.Open(ctx, secretstore.WithInteraction(secretstore.InteractionDenied))
	if err != nil {
		t.Fatalf("native store prerequisite or open failed: %v", err)
	}
	defer store.Close()
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	key := secretstore.Key{Service: "dev.go-secretstore.platform-test", Account: hex.EncodeToString(random)}
	value := []byte("line1\nline2\x00binary")
	defer func() { _ = store.Delete(context.Background(), key) }()
	if err := store.Set(ctx, key, []byte("first")); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := store.Set(ctx, key, value); err != nil {
		t.Fatalf("overwrite Set failed: %v", err)
	}
	secret, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	got, err := secret.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := secret.Close(); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(value) {
		t.Fatal("native store returned a different value")
	}
	clear(got)
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := store.Get(ctx, key); secretstore.CodeOf(err) != secretstore.NotFound {
		t.Fatalf("Get after Delete code = %q, error = %v", secretstore.CodeOf(err), err)
	}
}
