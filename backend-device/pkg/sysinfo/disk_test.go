// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package sysinfo

import "testing"

func TestDiskOf(t *testing.T) {
	d, ok := DiskOf(t.TempDir())
	if !ok || d.Total == 0 || d.Free > d.Total {
		t.Fatalf("disk = %+v, ok = %v", d, ok)
	}
	if _, ok := DiskOf("/no/such/folder/for/go-link"); ok {
		t.Fatal("a missing folder has no disk")
	}
}
