// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin && cgo

package keyring

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <Security/Security.h>

static CFMutableDictionaryRef kr_query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

static int kr_set(const char *service, const char *account, const void *data, int n) {
	CFMutableDictionaryRef q = kr_query(service, account);
	SecItemDelete(q);
	CFDataRef d = CFDataCreate(NULL, data, n);
	CFDictionarySetValue(q, kSecValueData, d);
	OSStatus st = SecItemAdd(q, NULL);
	CFRelease(d);
	CFRelease(q);
	return (int)st;
}

static int kr_get(const char *service, const char *account, void **out, int *n) {
	CFMutableDictionaryRef q = kr_query(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef r = NULL;
	OSStatus st = SecItemCopyMatching(q, &r);
	CFRelease(q);
	if (st != errSecSuccess) return (int)st;
	CFDataRef d = (CFDataRef)r;
	*n = (int)CFDataGetLength(d);
	*out = malloc(*n > 0 ? *n : 1);
	memcpy(*out, CFDataGetBytePtr(d), *n);
	CFRelease(r);
	return 0;
}

static int kr_del(const char *service, const char *account) {
	CFMutableDictionaryRef q = kr_query(service, account);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return (int)st;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const errSecItemNotFound = -25300

func status(st C.int) error {
	switch st {
	case 0:
		return nil
	case errSecItemNotFound:
		return ErrNotFound
	}
	return fmt.Errorf("keyring: keychain error %d", int(st))
}

func set(service, account, secret string) error {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	data := C.CBytes([]byte(secret))
	defer C.free(data)
	return status(C.kr_set(s, a, data, C.int(len(secret))))
}

func get(service, account string) (string, error) {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var out unsafe.Pointer
	var n C.int
	if err := status(C.kr_get(s, a, &out, &n)); err != nil {
		return "", err
	}
	defer C.free(out)
	return C.GoStringN((*C.char)(out), n), nil
}

func del(service, account string) error {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	return status(C.kr_del(s, a))
}
