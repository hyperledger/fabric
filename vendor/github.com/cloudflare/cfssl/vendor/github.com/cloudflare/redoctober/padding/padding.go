// Package padding adds and removes padding for AES-CBC mode.
//
// Copyright (c) 2013 CloudFlare, Inc.

package padding

import "errors"

// The final byte of a padded []byte indicates the number of padding
// bytes that were added. The padding bytes are always NUL bytes and
// up to 16 bytes may be added.
//
// Examples:
//
// 1. Data to be padded has a length divisible by 16. 16 bytes will be
// added where the first 15 are 0x00 and the final byte is 0x10.
//
// 2. Data to be padded has a length with remainder 15 when divided by
// 16. One byte will be added and that byte will be 0x01 (indicating
// one byte of padding).
//
// 3. Data to be padded has a length with remainder 2 when divided by
// 16. 14 bytes will be added. The first 13 will be 0x00 and then final
// byte will be 0x0e.
//
// Removing padding is trivial: the number of bytes specified by the
// final byte are removed.

// RemovePadding removes padding from data that was added with
// AddPadding
func RemovePadding(b []byte) ([]byte, error) {
	l := int(b[len(b)-1])
	if l > 16 {
		return nil, errors.New("Padding incorrect")
	}

	return b[:len(b)-l], nil
}

// AddPadding adds padding to a block of data
func AddPadding(b []byte) []byte {
	l := 16 - len(b)%16
	padding := make([]byte, l)
	padding[l-1] = byte(l)
	return append(b, padding...)
}
