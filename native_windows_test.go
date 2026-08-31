// SPDX-License-Identifier: Apache-2.0
//go:build windows

package secretstore

import (
	"bytes"
	"testing"
)

func TestClearCredentialBlob(t *testing.T) {
	value := []byte("secret")
	credential := &credential{
		CredentialBlobSize: uint32(len(value)),
		CredentialBlob:     &value[0],
	}
	clearCredentialBlob(credential)
	if !bytes.Equal(value, make([]byte, len(value))) {
		t.Fatal("credential blob was not cleared")
	}
}
