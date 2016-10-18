// padding_test.go: tests for padding.go
//
// Copyright (c) 2013 CloudFlare, Inc.

package padding

import (
	"bytes"
	"testing"
)

func assert(t *testing.T, b bool) {
	if !b {
		t.Fail()
	}
}

func TestAddPadding(t *testing.T) {
	b := make([]byte, 16)
	c := AddPadding(b)
	assert(t, len(c) == 32)
	assert(t, c[31] == 16)

	b = make([]byte, 32)
	c = AddPadding(b)
	assert(t, len(c) == 48)
	assert(t, c[47] == 16)

	b = make([]byte, 1)
	c = AddPadding(b)
	assert(t, len(c) == 16)
	assert(t, c[15] == 15)

	b = make([]byte, 15)
	c = AddPadding(b)
	assert(t, len(c) == 16)
	assert(t, c[15] == 1)
}

func TestRemovePadding(t *testing.T) {
	b := []byte("0123456789ABCDEF")
	c := AddPadding(b)
	assert(t, len(c) == 32)
	assert(t, c[31] == 16)
	assert(t, bytes.Compare(c[:16], b[:16]) == 0)
	d, err := RemovePadding(c)
	assert(t, err == nil)
	assert(t, len(d) == 16)
	assert(t, bytes.Compare(b, d) == 0)

	b = []byte("0123456789")
	c = AddPadding(b)
	assert(t, len(c) == 16)
	assert(t, c[15] == 6)
	assert(t, bytes.Compare(c[:10], b[:10]) == 0)
	d, err = RemovePadding(c)
	assert(t, err == nil)
	assert(t, len(d) == 10)
	assert(t, bytes.Compare(b, d) == 0)
}

func TestDetectBadPadding(t *testing.T) {
	b := []byte("0123456789ABCDEF")
	c := AddPadding(b)
	assert(t, len(c) == 32)
	assert(t, c[31] == 16)
	assert(t, bytes.Compare(c[:16], b[:16]) == 0)
	c[31] = 42
	d, err := RemovePadding(c)
	assert(t, err != nil)
	assert(t, d == nil)
}
