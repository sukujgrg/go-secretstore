// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestRunUsage(t *testing.T) {
	stderr := os.Stderr
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devNull
	t.Cleanup(func() {
		os.Stderr = stderr
		_ = devNull.Close()
	})

	for _, args := range [][]string{
		{},
		{"wat"},
		{"set", "svc"},
		{"set", "svc", "acct", "secret", "extra"},
		{"get", "svc"},
		{"get", "svc", "acct", "extra"},
		{"delete", "svc"},
		{"-h"},
	} {
		if code := run(args); code != 2 {
			t.Fatalf("run(%q) = %d, want 2", args, code)
		}
	}
}

func TestSetSecretFromArg(t *testing.T) {
	got, err := setSecret([]string{"set", "svc", "acct", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	if _, err := setSecret([]string{"set", "svc", "acct", ""}); err == nil {
		t.Fatal("empty argument should fail")
	}
}

func TestSetSecretFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(w, bytes.NewReader([]byte("from-stdin\r\n")))
		_ = w.Close()
		done <- err
	}()
	got, err := setSecret([]string{"set", "svc", "acct"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-stdin" {
		t.Fatalf("got %q", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	emptyR, emptyW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = emptyR
	_ = emptyW.Close()
	if _, err := setSecret([]string{"set", "svc", "acct"}); err == nil {
		t.Fatal("empty stdin should fail")
	}
}
