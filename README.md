# go-secretstore

[![CI](https://github.com/sukujgrg/go-secretstore/workflows/CI/badge.svg)](https://github.com/sukujgrg/go-secretstore/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/sukujgrg/go-secretstore.svg)](https://pkg.go.dev/github.com/sukujgrg/go-secretstore)

A small Go library for the current user's native protected secret store.
Module path: `github.com/sukujgrg/go-secretstore`.

There is **no file, environment, command-line, or alternate-store fallback**.
There are **no Go module dependencies**. macOS and Linux use CGO; Windows uses
stdlib syscalls.

| Platform | Backend | Build requirement |
| --- | --- | --- |
| macOS | Security.framework generic passwords | CGO, `Security` + `CoreFoundation` |
| Linux | libsecret / Freedesktop Secret Service | CGO, `pkg-config` `libsecret-1` |
| Windows | Credential Manager (`CredReadW` / `CredWriteW` / `CredDeleteW`) | stdlib `syscall` + `advapi32` (no CGO) |

A `CGO_ENABLED=0` build compiles everywhere. On macOS and Linux, `Open` then
returns `unsupported`. On Windows the syscall backend still works.

## Why this library?

Most cross-platform Go keyring packages optimize for either easy static builds
or a broad choice of storage backends. `go-secretstore` makes a narrower choice:
one protected store per OS, accessed through the platform's native client API,
with no silent downgrade to a different kind of storage.

| Design choice | `go-secretstore` behavior |
| --- | --- |
| Native API boundary | Calls Security.framework on macOS, libsecret on Linux, and Credential Manager on Windows; it does not start a helper command. |
| Fail closed | If the native store is unavailable, the native operation fails. It never falls back to a file, environment variable, `pass`, kernel keyring, or another provider. |
| Interaction control | Native authentication and unlock UI is denied by default and must be enabled explicitly. |
| Secret representation | Secrets are bounded binary `[]byte` values rather than strings. Returned values have explicit ownership and clearing semantics. |
| Errors | Operations return stable, backend-neutral `ErrorCode` values; rendered errors contain neither the key nor the secret. |
| Dependency surface | The module has no Go dependencies. macOS and Linux deliberately trade simple static cross-compilation for native framework/library dependencies. |

This is intentionally different from
[`zalando/go-keyring`](https://github.com/zalando/go-keyring), which favors
CGO-free/static builds, invokes `/usr/bin/security` on macOS, and implements the
Linux Secret Service client in Go over D-Bus. It is also narrower than
[`99designs/keyring`](https://github.com/99designs/keyring), which provides a
richer, configurable abstraction over native stores plus alternatives such as
`pass`, kernel keyrings, and encrypted files.

### Why CGO on macOS and Linux?

CGO is not inherently more secure, and it does not prevent a secret from
appearing in Go or native memory. It is a better fit for this package's goal of
being a thin native-store adapter: the implementation can pass binary data
directly, use native status codes and interaction flags, and rely on the
platform client library's object and session lifecycle. It also avoids a child
process, command input/output encoding, and parsing command text as an API.

On Linux, libsecret is itself a client for the Freedesktop Secret Service over
D-Bus. The choice is therefore not “native calls instead of D-Bus”; it is to let
libsecret own that protocol and session behavior rather than reimplementing it
in Go.

The cost is real: macOS and Linux builds need CGO, a C toolchain, and the native
development files. If a single static binary or easy cross-compilation matters
more than this API fidelity, a CGO-free library is likely the better choice.

## Library

```go
ctx := context.Background()
store, err := secretstore.Open(ctx, secretstore.WithInteraction(secretstore.InteractionDenied))
if err != nil {
    return err
}
defer store.Close()

key := secretstore.Key{Service: "example.app", Account: "alice"}
if err := store.Set(ctx, key, []byte("secret")); err != nil {
    return err
}
secret, err := store.Get(ctx, key)
if err != nil {
    return err
}
defer secret.Close()
value, err := secret.Bytes()
if err != nil {
    return err
}
defer clear(value)
```

One-shot helpers (`Get` / `Set` / `Delete`) open the native store, run one
operation, and close it.

`OpenMemory` returns an in-memory `Store` for tests. It is not a production
fallback.

Interaction is denied by default. Pass `WithInteraction(InteractionAllowed)`
when the native store may show its own unlock or authentication UI.

The native APIs are synchronous and cannot be forcibly interrupted by a Go
context once entered. A cancelled context prevents a native operation from
starting; a cancelled `Get` discards a value returned afterward. Once `Set` or
`Delete` starts, its native result is authoritative, so a successful mutation
is not reported as cancelled.

Errors use stable `ErrorCode` values and never include the secret or key.
`Secret.Close` clears the buffer owned by the returned `Secret`. `Set` copies
the caller slice and does not retain it.

Linux items use Secret Service attributes `service` and `username` (the
account), so `secret-tool lookup service … username …` can find them. Windows
target names are `go-secretstore/v1/<base64url(service)>/<base64url(account)>`
so service/account values cannot collide.

## Test binary

```sh
make build
./bin/secretstore set example.app alice 'the-secret'
./bin/secretstore get example.app alice
./bin/secretstore delete example.app alice
```

`set` reads the secret from the last argument, or from stdin if omitted.
`--allow-interaction` lets the native store prompt. `--timeout` defaults to 30s.

## Makefile

| Target | Purpose |
| --- | --- |
| `make build` | CGO-enabled `bin/secretstore` |
| `make test` | unit tests (in-memory store) |
| `make test-native` | live Get/Set/Delete against the OS store |
| `make lint` | `golangci-lint` for the current OS and Windows |
| `make smoke` | build, then set/get/delete `go-secretstore.smoke` / `local-test` |
| `make vet` | `go vet` |
| `make clean` | remove `bin/` |

Linux compile packages: `libsecret-1-dev` (Debian/Ubuntu) or `libsecret-devel`
(Fedora). Runtime needs a Secret Service daemon such as GNOME Keyring or KDE
Wallet.
