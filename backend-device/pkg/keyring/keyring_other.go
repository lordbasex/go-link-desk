// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !(darwin && cgo) && !windows && !linux

package keyring

func set(string, string, string) error   { return ErrUnavailable }
func get(string, string) (string, error) { return "", ErrUnavailable }
func del(string, string) error           { return ErrUnavailable }
