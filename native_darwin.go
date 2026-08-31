// SPDX-License-Identifier: Apache-2.0
//go:build darwin && cgo

package secretstore

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static void ss_clear_free(void *value, size_t length) {
	if (value == NULL) return;
	volatile unsigned char *bytes = (volatile unsigned char *)value;
	while (length > 0) {
		*bytes++ = 0;
		length--;
	}
	free(value);
}

static CFStringRef ss_string(const void *bytes, size_t length) {
	return CFStringCreateWithBytes(kCFAllocatorDefault, bytes, (CFIndex)length,
		kCFStringEncodingUTF8, false);
}

static CFMutableDictionaryRef ss_query(const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui) {
	CFStringRef service_string = ss_string(service, service_len);
	CFStringRef account_string = ss_string(account, account_len);
	if (service_string == NULL || account_string == NULL) {
		if (service_string != NULL) CFRelease(service_string);
		if (account_string != NULL) CFRelease(account_string);
		return NULL;
	}
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(kCFAllocatorDefault,
		0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (query != NULL) {
		CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
		CFDictionarySetValue(query, kSecAttrService, service_string);
		CFDictionarySetValue(query, kSecAttrAccount, account_string);
		#pragma clang diagnostic push
		#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		CFDictionarySetValue(query, kSecUseAuthenticationUI,
			allow_ui ? kSecUseAuthenticationUIAllow : kSecUseAuthenticationUIFail);
		#pragma clang diagnostic pop
	}
	CFRelease(service_string);
	CFRelease(account_string);
	return query;
}

static OSStatus ss_keychain_get(const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui,
		size_t max_output_len, void **output, size_t *output_len) {
	*output = NULL;
	*output_len = 0;
	CFMutableDictionaryRef query = ss_query(service, service_len, account, account_len, allow_ui);
	if (query == NULL) return errSecParam;
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);
	if (status != errSecSuccess) return status;
	if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
		if (result != NULL) CFRelease(result);
		return errSecDecode;
	}
	CFDataRef data = (CFDataRef)result;
	CFIndex length = CFDataGetLength(data);
	if (length <= 0) {
		CFRelease(result);
		return errSecDecode;
	}
	if ((size_t)length > max_output_len) {
		CFRelease(result);
		return errSecDataTooLarge;
	}
	void *copy = malloc((size_t)length);
	if (copy == NULL) {
		CFRelease(result);
		return errSecAllocate;
	}
	memcpy(copy, CFDataGetBytePtr(data), (size_t)length);
	CFRelease(result);
	*output = copy;
	*output_len = (size_t)length;
	return errSecSuccess;
}

static OSStatus ss_keychain_set(const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui,
		const void *value, size_t value_len) {
	CFMutableDictionaryRef query = ss_query(service, service_len, account, account_len, allow_ui);
	if (query == NULL) return errSecParam;
	CFDataRef data = CFDataCreate(kCFAllocatorDefault, value, (CFIndex)value_len);
	if (data == NULL) {
		CFRelease(query);
		return errSecAllocate;
	}
	const void *keys[] = { kSecValueData };
	const void *values[] = { data };
	CFDictionaryRef update = CFDictionaryCreate(kCFAllocatorDefault, keys, values,
		1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = update == NULL ? errSecAllocate : SecItemUpdate(query, update);
	if (update != NULL) CFRelease(update);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(query, kSecValueData, data);
		status = SecItemAdd(query, NULL);
	}
	CFRelease(data);
	CFRelease(query);
	return status;
}

static OSStatus ss_keychain_delete(const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui) {
	CFMutableDictionaryRef query = ss_query(service, service_len, account, account_len, allow_ui);
	if (query == NULL) return errSecParam;
	OSStatus status = SecItemDelete(query);
	CFRelease(query);
	return status;
}
*/
import "C"

import (
	"context"
	"unsafe"
)

type keychainBackend struct{ interaction InteractionPolicy }

func openNative(ctx context.Context, configuration options) (backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError("open", "macos-keychain", err)
	}
	return &keychainBackend{interaction: configuration.interaction}, nil
}

func (*keychainBackend) name() string { return "macos-keychain" }

func (b *keychainBackend) get(ctx context.Context, key Key) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError("get", b.name(), err)
	}
	service := []byte(key.Service)
	account := []byte(key.Account)
	var output unsafe.Pointer
	var outputLength C.size_t
	status := C.ss_keychain_get(unsafe.Pointer(&service[0]), C.size_t(len(service)),
		unsafe.Pointer(&account[0]), C.size_t(len(account)), b.allowUI(), C.size_t(maxSecretBytes),
		&output, &outputLength)
	if output != nil {
		defer C.ss_clear_free(output, outputLength)
	}
	if status != C.errSecSuccess {
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

func (b *keychainBackend) set(ctx context.Context, key Key, value []byte) error {
	return runNativeMutation(ctx, "set", b.name(), func() error {
		service := []byte(key.Service)
		account := []byte(key.Account)
		status := C.ss_keychain_set(unsafe.Pointer(&service[0]), C.size_t(len(service)),
			unsafe.Pointer(&account[0]), C.size_t(len(account)), b.allowUI(),
			unsafe.Pointer(&value[0]), C.size_t(len(value)))
		if status != C.errSecSuccess {
			return b.mapStatus("set", status)
		}
		return nil
	})
}

func (b *keychainBackend) delete(ctx context.Context, key Key) error {
	return runNativeMutation(ctx, "delete", b.name(), func() error {
		service := []byte(key.Service)
		account := []byte(key.Account)
		status := C.ss_keychain_delete(unsafe.Pointer(&service[0]), C.size_t(len(service)),
			unsafe.Pointer(&account[0]), C.size_t(len(account)), b.allowUI())
		if status != C.errSecSuccess {
			return b.mapStatus("delete", status)
		}
		return nil
	})
}

func (*keychainBackend) close() error { return nil }

func (b *keychainBackend) allowUI() C.int {
	if b.interaction == InteractionAllowed {
		return 1
	}
	return 0
}

func (b *keychainBackend) mapStatus(op string, status C.OSStatus) error {
	switch status {
	case C.errSecItemNotFound:
		return storeError(NotFound, op, b.name(), nil)
	case C.errSecInteractionNotAllowed:
		return storeError(InteractionRequired, op, b.name(), nil)
	case C.errSecAuthFailed:
		return storeError(PermissionDenied, op, b.name(), nil)
	case C.errSecUserCanceled:
		return storeError(OperationCancelled, op, b.name(), nil)
	case C.errSecDataTooLarge, C.errSecAllocate:
		return storeError(ResourceLimit, op, b.name(), nil)
	default:
		return storeError(BackendFailure, op, b.name(), nil)
	}
}
