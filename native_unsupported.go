// SPDX-License-Identifier: Apache-2.0
//go:build !darwin && !linux && !windows

package secretstore

import "context"

func openNative(context.Context, options) (backend, error) {
	return nil, storeError(Unsupported, "open", "native", nil)
}
