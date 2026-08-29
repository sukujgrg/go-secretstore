// SPDX-License-Identifier: Apache-2.0
//go:build linux && !cgo

package secretstore

import "context"

func openNative(context.Context, options) (backend, error) {
	return nil, storeError(Unsupported, "open", "linux-secret-service", nil)
}
