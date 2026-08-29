// SPDX-License-Identifier: Apache-2.0
//go:build linux && cgo

package secretstore

/*
#cgo pkg-config: libsecret-1
#include <libsecret/secret.h>
#include <gio/gio.h>
#include <stdlib.h>
#include <string.h>

enum {
	SS_OK = 0,
	SS_NOT_FOUND,
	SS_DENIED,
	SS_INTERACTION,
	SS_UNSUPPORTED,
	SS_CANCELLED,
	SS_LIMIT,
	SS_BACKEND
};

typedef struct ss_linux_store {
	SecretService *service;
} ss_linux_store;

static const SecretSchema ss_schema = {
	"com.github.sukujgrg.go-secretstore",
	SECRET_SCHEMA_NONE,
	{
		{ "service", SECRET_SCHEMA_ATTRIBUTE_STRING },
		{ "username", SECRET_SCHEMA_ATTRIBUTE_STRING },
		{ NULL, 0 },
	}
};

static int ss_map_error(GError *err) {
	if (err == NULL) {
		return SS_BACKEND;
	}
	if (g_error_matches(err, G_IO_ERROR, G_IO_ERROR_CANCELLED)) {
		return SS_CANCELLED;
	}
	if (g_error_matches(err, SECRET_ERROR, SECRET_ERROR_IS_LOCKED)) {
		return SS_INTERACTION;
	}
	if (g_error_matches(err, SECRET_ERROR, SECRET_ERROR_NO_SUCH_OBJECT)) {
		return SS_NOT_FOUND;
	}
	if (g_error_matches(err, G_IO_ERROR, G_IO_ERROR_NOT_FOUND) ||
		g_error_matches(err, G_IO_ERROR, G_IO_ERROR_NOT_SUPPORTED) ||
		g_error_matches(err, G_DBUS_ERROR, G_DBUS_ERROR_SERVICE_UNKNOWN) ||
		g_error_matches(err, G_DBUS_ERROR, G_DBUS_ERROR_NAME_HAS_NO_OWNER) ||
		g_error_matches(err, G_DBUS_ERROR, G_DBUS_ERROR_SPAWN_SERVICE_NOT_FOUND)) {
		return SS_UNSUPPORTED;
	}
	if (g_error_matches(err, G_DBUS_ERROR, G_DBUS_ERROR_ACCESS_DENIED)) {
		return SS_DENIED;
	}
	return SS_BACKEND;
}

static GHashTable *ss_attrs(const void *service, size_t service_len,
		const void *account, size_t account_len) {
	GHashTable *attrs = g_hash_table_new_full(g_str_hash, g_str_equal, g_free, g_free);
	g_hash_table_insert(attrs, g_strdup("service"), g_strndup(service, service_len));
	g_hash_table_insert(attrs, g_strdup("username"), g_strndup(account, account_len));
	return attrs;
}

int ss_linux_open(ss_linux_store **out) {
	GError *err = NULL;
	SecretService *service = secret_service_get_sync(SECRET_SERVICE_OPEN_SESSION, NULL, &err);
	if (service == NULL) {
		int code = ss_map_error(err);
		if (err != NULL) g_error_free(err);
		if (code == SS_BACKEND) code = SS_UNSUPPORTED;
		return code;
	}
	ss_linux_store *store = g_new0(ss_linux_store, 1);
	store->service = service;
	*out = store;
	return SS_OK;
}

void ss_linux_close(ss_linux_store *store) {
	if (store == NULL) return;
	if (store->service != NULL) g_object_unref(store->service);
	g_free(store);
}

static int ss_unlock_collection(ss_linux_store *store, SecretCollection *collection, int allow_ui) {
	if (!secret_collection_get_locked(collection)) {
		return SS_OK;
	}
	if (!allow_ui) {
		return SS_INTERACTION;
	}
	GError *err = NULL;
	GList *objects = g_list_append(NULL, collection);
	GList *unlocked = NULL;
	gboolean ok = secret_service_unlock_sync(store->service, objects, NULL, &unlocked, &err);
	g_list_free(objects);
	if (unlocked != NULL) g_list_free(unlocked);
	if (!ok) {
		int code = ss_map_error(err);
		if (err != NULL) g_error_free(err);
		return code;
	}
	return SS_OK;
}

int ss_linux_get(ss_linux_store *store, const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui,
		void **output, size_t *output_len) {
	*output = NULL;
	*output_len = 0;
	GHashTable *attrs = ss_attrs(service, service_len, account, account_len);
	SecretSearchFlags flags = SECRET_SEARCH_ALL | SECRET_SEARCH_LOAD_SECRETS;
	if (allow_ui) flags |= SECRET_SEARCH_UNLOCK;
	GError *err = NULL;
	GList *items = secret_service_search_sync(store->service, &ss_schema, attrs, flags, NULL, &err);
	g_hash_table_unref(attrs);
	if (err != NULL) {
		int code = ss_map_error(err);
		g_error_free(err);
		if (items != NULL) g_list_free_full(items, g_object_unref);
		return code;
	}
	if (items == NULL) {
		return SS_NOT_FOUND;
	}
	if (items->next != NULL) {
		g_list_free_full(items, g_object_unref);
		return SS_BACKEND;
	}
	SecretItem *item = SECRET_ITEM(items->data);
	if (secret_item_get_locked(item)) {
		g_list_free_full(items, g_object_unref);
		return SS_INTERACTION;
	}
	SecretValue *secret = secret_item_get_secret(item);
	if (secret == NULL) {
		if (!secret_item_load_secret_sync(item, NULL, &err)) {
			int code = ss_map_error(err);
			if (err != NULL) g_error_free(err);
			g_list_free_full(items, g_object_unref);
			return code;
		}
		secret = secret_item_get_secret(item);
	}
	if (secret == NULL) {
		g_list_free_full(items, g_object_unref);
		return SS_BACKEND;
	}
	gsize length = 0;
	const gchar *bytes = secret_value_get(secret, &length);
	if (bytes == NULL || length == 0) {
		secret_value_unref(secret);
		g_list_free_full(items, g_object_unref);
		return SS_BACKEND;
	}
	void *copy = malloc(length);
	if (copy == NULL) {
		secret_value_unref(secret);
		g_list_free_full(items, g_object_unref);
		return SS_LIMIT;
	}
	memcpy(copy, bytes, length);
	secret_value_unref(secret);
	g_list_free_full(items, g_object_unref);
	*output = copy;
	*output_len = (size_t)length;
	return SS_OK;
}

int ss_linux_set(ss_linux_store *store, const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui,
		const void *value, size_t value_len) {
	GError *err = NULL;
	SecretCollection *collection = secret_collection_for_alias_sync(store->service,
		SECRET_COLLECTION_DEFAULT, SECRET_COLLECTION_NONE, NULL, &err);
	if (collection == NULL) {
		int code = ss_map_error(err);
		if (err != NULL) g_error_free(err);
		return code;
	}
	int unlocked = ss_unlock_collection(store, collection, allow_ui);
	if (unlocked != SS_OK) {
		g_object_unref(collection);
		return unlocked;
	}
	GHashTable *attrs = ss_attrs(service, service_len, account, account_len);
	gchar *service_s = g_strndup(service, service_len);
	gchar *account_s = g_strndup(account, account_len);
	gchar *label = g_strdup_printf("Password for '%s' on '%s'", account_s, service_s);
	g_free(service_s);
	g_free(account_s);
	SecretValue *secret = secret_value_new((const gchar *)value, (gssize)value_len, "application/octet-stream");
	SecretItem *item = secret_item_create_sync(collection, &ss_schema, attrs, label, secret,
		SECRET_ITEM_CREATE_REPLACE, NULL, &err);
	secret_value_unref(secret);
	g_free(label);
	g_hash_table_unref(attrs);
	g_object_unref(collection);
	if (item == NULL) {
		int code = ss_map_error(err);
		if (err != NULL) g_error_free(err);
		return code;
	}
	g_object_unref(item);

	GHashTable *check = ss_attrs(service, service_len, account, account_len);
	GList *items = secret_service_search_sync(store->service, &ss_schema, check,
		SECRET_SEARCH_ALL, NULL, &err);
	g_hash_table_unref(check);
	if (err != NULL) {
		int code = ss_map_error(err);
		g_error_free(err);
		if (items != NULL) g_list_free_full(items, g_object_unref);
		return code;
	}
	int n = 0;
	for (GList *p = items; p != NULL; p = p->next) n++;
	if (items != NULL) g_list_free_full(items, g_object_unref);
	if (n != 1) return SS_BACKEND;
	return SS_OK;
}

int ss_linux_delete(ss_linux_store *store, const void *service, size_t service_len,
		const void *account, size_t account_len, int allow_ui) {
	GHashTable *attrs = ss_attrs(service, service_len, account, account_len);
	SecretSearchFlags flags = SECRET_SEARCH_ALL;
	if (allow_ui) flags |= SECRET_SEARCH_UNLOCK;
	GError *err = NULL;
	GList *items = secret_service_search_sync(store->service, &ss_schema, attrs, flags, NULL, &err);
	g_hash_table_unref(attrs);
	if (err != NULL) {
		int code = ss_map_error(err);
		g_error_free(err);
		if (items != NULL) g_list_free_full(items, g_object_unref);
		return code;
	}
	if (items == NULL) {
		return SS_NOT_FOUND;
	}
	if (items->next != NULL) {
		g_list_free_full(items, g_object_unref);
		return SS_BACKEND;
	}
	SecretItem *item = SECRET_ITEM(items->data);
	if (secret_item_get_locked(item) && !allow_ui) {
		g_list_free_full(items, g_object_unref);
		return SS_INTERACTION;
	}
	if (!secret_item_delete_sync(item, NULL, &err)) {
		int code = ss_map_error(err);
		if (err != NULL) g_error_free(err);
		g_list_free_full(items, g_object_unref);
		return code;
	}
	g_list_free_full(items, g_object_unref);
	return SS_OK;
}
*/
import "C"

import (
	"context"
	"sync"
	"unsafe"
)

type secretServiceBackend struct {
	mu          sync.Mutex
	store       *C.ss_linux_store
	interaction InteractionPolicy
}

func openNative(ctx context.Context, configuration options) (backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError("open", "linux-secret-service", err)
	}
	var native *C.ss_linux_store
	status := C.ss_linux_open(&native)
	if status != C.SS_OK {
		return nil, mapLinuxStatus("open", "linux-secret-service", status)
	}
	return &secretServiceBackend{store: native, interaction: configuration.interaction}, nil
}

func (*secretServiceBackend) name() string { return "linux-secret-service" }

func (b *secretServiceBackend) get(ctx context.Context, key Key) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, contextError("get", b.name(), err)
	}
	service := []byte(key.Service)
	account := []byte(key.Account)
	var output unsafe.Pointer
	var outputLength C.size_t
	status := C.ss_linux_get(b.store, unsafe.Pointer(&service[0]), C.size_t(len(service)),
		unsafe.Pointer(&account[0]), C.size_t(len(account)), b.allowUI(), &output, &outputLength)
	if output != nil {
		defer C.free(output)
	}
	if status != C.SS_OK {
		return nil, mapLinuxStatus("get", b.name(), status)
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

func (b *secretServiceBackend) set(ctx context.Context, key Key, value []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contextError("set", b.name(), err)
	}
	service := []byte(key.Service)
	account := []byte(key.Account)
	status := C.ss_linux_set(b.store, unsafe.Pointer(&service[0]), C.size_t(len(service)),
		unsafe.Pointer(&account[0]), C.size_t(len(account)), b.allowUI(),
		unsafe.Pointer(&value[0]), C.size_t(len(value)))
	if status != C.SS_OK {
		return mapLinuxStatus("set", b.name(), status)
	}
	if err := ctx.Err(); err != nil {
		return contextError("set", b.name(), err)
	}
	return nil
}

func (b *secretServiceBackend) delete(ctx context.Context, key Key) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contextError("delete", b.name(), err)
	}
	service := []byte(key.Service)
	account := []byte(key.Account)
	status := C.ss_linux_delete(b.store, unsafe.Pointer(&service[0]), C.size_t(len(service)),
		unsafe.Pointer(&account[0]), C.size_t(len(account)), b.allowUI())
	if status != C.SS_OK {
		return mapLinuxStatus("delete", b.name(), status)
	}
	if err := ctx.Err(); err != nil {
		return contextError("delete", b.name(), err)
	}
	return nil
}

func (b *secretServiceBackend) close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.store != nil {
		C.ss_linux_close(b.store)
		b.store = nil
	}
	return nil
}

func (b *secretServiceBackend) allowUI() C.int {
	if b.interaction == InteractionAllowed {
		return 1
	}
	return 0
}

func mapLinuxStatus(op, backend string, status C.int) error {
	switch status {
	case C.SS_NOT_FOUND:
		return storeError(NotFound, op, backend, nil)
	case C.SS_DENIED:
		return storeError(PermissionDenied, op, backend, nil)
	case C.SS_INTERACTION:
		return storeError(InteractionRequired, op, backend, nil)
	case C.SS_UNSUPPORTED:
		return storeError(Unsupported, op, backend, nil)
	case C.SS_CANCELLED:
		return storeError(OperationCancelled, op, backend, context.Canceled)
	case C.SS_LIMIT:
		return storeError(ResourceLimit, op, backend, nil)
	default:
		return storeError(BackendFailure, op, backend, nil)
	}
}
