// SPDX-License-Identifier: Apache-2.0

// Package secretstore is a small, provider-neutral interface to the current
// user's native protected secret store.
//
// There is no file, environment, command-line, or alternate-store fallback.
// Native backends are:
//
//   - macOS: Security.framework generic passwords (CGO)
//   - Linux: libsecret / Freedesktop Secret Service (CGO)
//   - Windows: Credential Manager via advapi32 syscalls (no CGO)
//
// OpenMemory provides an in-memory Store for tests, following the same
// contract as the native backends.
package secretstore
