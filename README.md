# go-secretstore

[![CI](https://github.com/sukujgrg/go-secretstore/workflows/CI/badge.svg)](https://github.com/sukujgrg/go-secretstore/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/sukujgrg/go-secretstore.svg)](https://pkg.go.dev/github.com/sukujgrg/go-secretstore)

A small Go library for the current user's native protected secret store.

- Docs: [pkg.go.dev/github.com/sukujgrg/go-secretstore](https://pkg.go.dev/github.com/sukujgrg/go-secretstore)
- CI: [github.com/sukujgrg/go-secretstore/actions](https://github.com/sukujgrg/go-secretstore/actions/workflows/ci.yml) (latest `main` run [succeeded](https://github.com/sukujgrg/go-secretstore/actions/runs/33289240207))

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
