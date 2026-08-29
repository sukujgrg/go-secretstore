// SPDX-License-Identifier: Apache-2.0
//go:build windows && cgo

package secretstore

/*
#cgo LDFLAGS: -ladvapi32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <wincred.h>
#include <stdlib.h>
#include <string.h>

#define SS_WIN_MAX_BLOB (5 * 512)

static LPWSTR ss_utf16(const void *bytes, size_t len) {
	if (len == 0 || len > 0x7fffffff) return NULL;
	int n = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, (LPCCH)bytes, (int)len, NULL, 0);
	if (n <= 0) return NULL;
	LPWSTR out = (LPWSTR)malloc(((size_t)n + 1) * sizeof(WCHAR));
	if (out == NULL) return NULL;
	int written = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, (LPCCH)bytes, (int)len, out, n);
	if (written <= 0) {
		free(out);
		return NULL;
	}
	out[written] = 0;
	return out;
}

DWORD ss_cred_read(const void *target, size_t target_len, void **output, size_t *output_len) {
	*output = NULL;
	*output_len = 0;
	LPWSTR name = ss_utf16(target, target_len);
	if (name == NULL) return ERROR_INVALID_PARAMETER;
	PCREDENTIALW cred = NULL;
	if (!CredReadW(name, CRED_TYPE_GENERIC, 0, &cred)) {
		DWORD err = GetLastError();
		free(name);
		return err;
	}
	free(name);
	if (cred == NULL || cred->CredentialBlobSize == 0 || cred->CredentialBlob == NULL ||
		cred->CredentialBlobSize > SS_WIN_MAX_BLOB) {
		if (cred != NULL) CredFree(cred);
		return ERROR_INVALID_PARAMETER;
	}
	void *copy = malloc(cred->CredentialBlobSize);
	if (copy == NULL) {
		CredFree(cred);
		return ERROR_NOT_ENOUGH_MEMORY;
	}
	memcpy(copy, cred->CredentialBlob, cred->CredentialBlobSize);
	*output = copy;
	*output_len = (size_t)cred->CredentialBlobSize;
	CredFree(cred);
	return ERROR_SUCCESS;
}

DWORD ss_cred_write(const void *target, size_t target_len, const void *value, size_t value_len) {
	if (value_len == 0 || value_len > SS_WIN_MAX_BLOB) return ERROR_INVALID_PARAMETER;
	LPWSTR name = ss_utf16(target, target_len);
	if (name == NULL) return ERROR_INVALID_PARAMETER;
	CREDENTIALW cred;
	ZeroMemory(&cred, sizeof(cred));
	cred.Type = CRED_TYPE_GENERIC;
	cred.TargetName = name;
	cred.CredentialBlobSize = (DWORD)value_len;
	cred.CredentialBlob = (LPBYTE)value;
	cred.Persist = CRED_PERSIST_LOCAL_MACHINE;
	BOOL ok = CredWriteW(&cred, 0);
	DWORD err = ok ? ERROR_SUCCESS : GetLastError();
	free(name);
	return err;
}

DWORD ss_cred_delete(const void *target, size_t target_len) {
	LPWSTR name = ss_utf16(target, target_len);
	if (name == NULL) return ERROR_INVALID_PARAMETER;
	BOOL ok = CredDeleteW(name, CRED_TYPE_GENERIC, 0);
	DWORD err = ok ? ERROR_SUCCESS : GetLastError();
	free(name);
	return err;
}
*/
import "C"

import (
	"context"
	"unsafe"
)

type credentialManagerBackend struct{}

func openNative(ctx context.Context, configuration options) (backend, error) {
	_ = configuration
	if err := ctx.Err(); err != nil {
		return nil, contextError("open", "windows-credential-manager", err)
	}
	return &credentialManagerBackend{}, nil
}

func (*credentialManagerBackend) name() string { return "windows-credential-manager" }

func (b *credentialManagerBackend) get(ctx context.Context, key Key) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError("get", b.name(), err)
	}
	target := []byte(windowsTarget(key))
	var output unsafe.Pointer
	var outputLength C.size_t
	status := C.ss_cred_read(unsafe.Pointer(&target[0]), C.size_t(len(target)), &output, &outputLength)
	if output != nil {
		defer C.free(output)
	}
	if status != C.ERROR_SUCCESS {
		return nil, b.mapStatus("get", status)
	}
	if uint64(outputLength) > uint64(maxSecretBytes) {
		return nil, storeError(ResourceLimit, "get", b.name(), nil)
	}
	value := C.GoBytes(output, C.int(outputLength))
	if err := ctx.Err(); err != nil {
		clear(value)
		return nil, contextError("get", b.name(), err)
	}
	return value, nil
}

func (b *credentialManagerBackend) set(ctx context.Context, key Key, value []byte) error {
	if err := ctx.Err(); err != nil {
		return contextError("set", b.name(), err)
	}
	target := []byte(windowsTarget(key))
	status := C.ss_cred_write(unsafe.Pointer(&target[0]), C.size_t(len(target)),
		unsafe.Pointer(&value[0]), C.size_t(len(value)))
	if status != C.ERROR_SUCCESS {
		return b.mapStatus("set", status)
	}
	if err := ctx.Err(); err != nil {
		return contextError("set", b.name(), err)
	}
	return nil
}

func (b *credentialManagerBackend) delete(ctx context.Context, key Key) error {
	if err := ctx.Err(); err != nil {
		return contextError("delete", b.name(), err)
	}
	target := []byte(windowsTarget(key))
	status := C.ss_cred_delete(unsafe.Pointer(&target[0]), C.size_t(len(target)))
	if status != C.ERROR_SUCCESS {
		return b.mapStatus("delete", status)
	}
	if err := ctx.Err(); err != nil {
		return contextError("delete", b.name(), err)
	}
	return nil
}

func (*credentialManagerBackend) close() error { return nil }

func (b *credentialManagerBackend) mapStatus(op string, status C.DWORD) error {
	switch status {
	case C.ERROR_NOT_FOUND:
		return storeError(NotFound, op, b.name(), nil)
	case C.ERROR_ACCESS_DENIED:
		return storeError(PermissionDenied, op, b.name(), nil)
	case C.ERROR_NO_SUCH_LOGON_SESSION:
		return storeError(Unsupported, op, b.name(), nil)
	case C.ERROR_INVALID_PARAMETER, C.ERROR_NOT_ENOUGH_MEMORY:
		return storeError(ResourceLimit, op, b.name(), nil)
	default:
		return storeError(BackendFailure, op, b.name(), nil)
	}
}
