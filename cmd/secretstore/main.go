// SPDX-License-Identifier: Apache-2.0

// Command secretstore is a local test binary for the native secret store.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sukujgrg/go-secretstore"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("secretstore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	allowInteraction := fs.Bool("allow-interaction", false, "allow the native store to show unlock or auth UI")
	timeout := fs.Duration("timeout", 30*time.Second, "operation timeout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage:
  secretstore [flags] set <service> <account> [<secret>]
  secretstore [flags] get <service> <account>
  secretstore [flags] delete <service> <account>

set reads <secret> from the argument when present, otherwise from stdin.

flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fs.Usage()
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var opts []secretstore.Option
	if *allowInteraction {
		opts = append(opts, secretstore.WithInteraction(secretstore.InteractionAllowed))
	}

	switch rest[0] {
	case "set":
		if len(rest) < 3 || len(rest) > 4 {
			fs.Usage()
			return 2
		}
		secret, err := setSecret(rest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		defer clear(secret)
		if err := secretstore.Set(ctx, rest[1], rest[2], secret, opts...); err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		return 0
	case "get":
		if len(rest) != 3 {
			fs.Usage()
			return 2
		}
		secret, err := secretstore.Get(ctx, rest[1], rest[2], opts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		defer secret.Close()
		value, err := secret.Bytes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		defer clear(value)
		if _, err := os.Stdout.Write(value); err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		if _, err := os.Stdout.Write([]byte("\n")); err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		return 0
	case "delete":
		if len(rest) != 3 {
			fs.Usage()
			return 2
		}
		if err := secretstore.Delete(ctx, rest[1], rest[2], opts...); err != nil {
			fmt.Fprintf(os.Stderr, "secretstore: %v\n", err)
			return 1
		}
		return 0
	default:
		fs.Usage()
		return 2
	}
}

func setSecret(rest []string) ([]byte, error) {
	if len(rest) == 4 {
		if rest[3] == "" {
			return nil, fmt.Errorf("%s", secretstore.InvalidInput)
		}
		return []byte(rest[3]), nil
	}
	value, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	if len(value) > 0 && value[len(value)-1] == '\n' {
		value = value[:len(value)-1]
		if len(value) > 0 && value[len(value)-1] == '\r' {
			value = value[:len(value)-1]
		}
	}
	if len(value) == 0 {
		return nil, fmt.Errorf("%s", secretstore.InvalidInput)
	}
	return value, nil
}
