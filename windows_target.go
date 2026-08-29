// SPDX-License-Identifier: Apache-2.0

package secretstore

import (
	"encoding/base64"
	"fmt"
)

func windowsTarget(key Key) string {
	return fmt.Sprintf("go-secretstore/v1/%s/%s",
		base64.RawURLEncoding.EncodeToString([]byte(key.Service)),
		base64.RawURLEncoding.EncodeToString([]byte(key.Account)))
}
