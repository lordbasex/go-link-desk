// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows

package keyring

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32   = windows.NewLazySystemDLL("advapi32.dll")
	credWrite  = advapi32.NewProc("CredWriteW")
	credRead   = advapi32.NewProc("CredReadW")
	credDelete = advapi32.NewProc("CredDeleteW")
	credFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

// credential is Windows' CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func target(service, account string) (*uint16, error) {
	return windows.UTF16PtrFromString(service + "/" + account)
}

func callErr(err error) error {
	if errors.Is(err, windows.ERROR_NOT_FOUND) {
		return ErrNotFound
	}
	return err
}

func set(service, account, secret string) error {
	t, err := target(service, account)
	if err != nil {
		return err
	}
	user, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return err
	}
	blob := []byte(secret)
	c := credential{Type: credTypeGeneric, TargetName: t, UserName: user, Persist: credPersistLocalMachine, CredentialBlobSize: uint32(len(blob))}
	if len(blob) > 0 {
		c.CredentialBlob = &blob[0]
	}
	if r, _, err := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return callErr(err)
	}
	return nil
}

func get(service, account string) (string, error) {
	t, err := target(service, account)
	if err != nil {
		return "", err
	}
	var c *credential
	if r, _, err := credRead.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c))); r == 0 {
		return "", callErr(err)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c.CredentialBlobSize == 0 {
		return "", nil
	}
	return string(unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)), nil
}

func del(service, account string) error {
	t, err := target(service, account)
	if err != nil {
		return err
	}
	if r, _, err := credDelete.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0); r == 0 {
		return callErr(err)
	}
	return nil
}
