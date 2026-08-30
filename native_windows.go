// SPDX-License-Identifier: Apache-2.0
//go:build windows

package secretstore

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	credentialTypeGeneric         = 1
	credentialPersistLocalMachine = 2
	maxCredentialBlobBytes        = 5 * 512
	errnoNotFound                 = syscall.Errno(1168)
	errnoNoSuchLogonSession       = syscall.Errno(1312)
)

var (
	modadvapi32     = syscall.NewLazyDLL("advapi32.dll")
	procCredReadW   = modadvapi32.NewProc("CredReadW")
	procCredWriteW  = modadvapi32.NewProc("CredWriteW")
	procCredDeleteW = modadvapi32.NewProc("CredDeleteW")
	procCredFree    = modadvapi32.NewProc("CredFree")
)

type filetime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

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
	value, err := credRead(windowsTarget(key))
	if err != nil {
		return nil, b.mapError("get", err)
	}
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
	if err := credWrite(windowsTarget(key), value); err != nil {
		return b.mapError("set", err)
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
	if err := credDelete(windowsTarget(key)); err != nil {
		return b.mapError("delete", err)
	}
	if err := ctx.Err(); err != nil {
		return contextError("delete", b.name(), err)
	}
	return nil
}

func (*credentialManagerBackend) close() error { return nil }

func (b *credentialManagerBackend) mapError(op string, err error) error {
	switch {
	case errors.Is(err, errnoNotFound):
		return storeError(NotFound, op, b.name(), nil)
	case errors.Is(err, syscall.ERROR_ACCESS_DENIED):
		return storeError(PermissionDenied, op, b.name(), nil)
	case errors.Is(err, errnoNoSuchLogonSession):
		return storeError(Unsupported, op, b.name(), nil)
	case errors.Is(err, syscall.EINVAL):
		return storeError(ResourceLimit, op, b.name(), nil)
	default:
		return storeError(BackendFailure, op, b.name(), nil)
	}
}

func credRead(target string) ([]byte, error) {
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return nil, err
	}
	var result *credential
	r1, _, e1 := syscall.SyscallN(procCredReadW.Addr(),
		uintptr(unsafe.Pointer(targetName)),
		uintptr(credentialTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&result)))
	if r1 == 0 {
		return nil, errnoErr(e1)
	}
	defer syscall.SyscallN(procCredFree.Addr(), uintptr(unsafe.Pointer(result)))
	if result == nil || result.CredentialBlobSize == 0 || result.CredentialBlobSize > maxCredentialBlobBytes || result.CredentialBlob == nil {
		return nil, syscall.EINVAL
	}
	return append([]byte(nil), unsafe.Slice(result.CredentialBlob, int(result.CredentialBlobSize))...), nil
}

func credWrite(target string, value []byte) error {
	if len(value) == 0 || len(value) > maxCredentialBlobBytes {
		return syscall.EINVAL
	}
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	input := credential{
		Type:               credentialTypeGeneric,
		TargetName:         targetName,
		CredentialBlobSize: uint32(len(value)),
		CredentialBlob:     &value[0],
		Persist:            credentialPersistLocalMachine,
	}
	r1, _, e1 := syscall.SyscallN(procCredWriteW.Addr(), uintptr(unsafe.Pointer(&input)), 0)
	runtime.KeepAlive(value)
	runtime.KeepAlive(targetName)
	if r1 == 0 {
		return errnoErr(e1)
	}
	return nil
}

func credDelete(target string) error {
	targetName, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	r1, _, e1 := syscall.SyscallN(procCredDeleteW.Addr(),
		uintptr(unsafe.Pointer(targetName)),
		uintptr(credentialTypeGeneric),
		0)
	if r1 == 0 {
		return errnoErr(e1)
	}
	return nil
}

func errnoErr(e syscall.Errno) error {
	if e == 0 {
		return syscall.EINVAL
	}
	return e
}
