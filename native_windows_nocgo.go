// SPDX-License-Identifier: Apache-2.0
//go:build windows && !cgo

package secretstore

import "context"

func openNative(context.Context, options) (backend, error) {
	return nil, storeError(Unsupported, "open", "windows-credential-manager", nil)
}
