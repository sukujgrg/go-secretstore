// SPDX-License-Identifier: Apache-2.0

// Package secretstore is a small, provider-neutral interface to the current
// user's native protected secret store.
//
// There is no file, environment, command-line, or alternate-store fallback.
// CGO is required on each supported platform:
//
//   - macOS: Security.framework generic passwords
//   - Linux: libsecret / Freedesktop Secret Service
//   - Windows: Credential Manager via advapi32
//
// OpenMemory provides an in-memory Store for tests, following the same
// contract as the native backends.
package secretstore
