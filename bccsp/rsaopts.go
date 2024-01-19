/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package bccsp

// RSA1024KeyGenOpts contains options for RSA key generation at 1024 security.
type RSA1024KeyGenOpts struct {
	Temporary bool
}

// Algorithm returns the key generation algorithm identifier (to be used).
func (opts *RSA1024KeyGenOpts) Algorithm() string {
	return RSA1024
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *RSA1024KeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}

// RSA2048KeyGenOpts contains options for RSA key generation at 2048 security.
type RSA2048KeyGenOpts struct {
	Temporary bool
}

// Algorithm returns the key generation algorithm identifier (to be used).
func (opts *RSA2048KeyGenOpts) Algorithm() string {
	return RSA2048
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *RSA2048KeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}

// RSA3072KeyGenOpts contains options for RSA key generation at 3072 security.
type RSA3072KeyGenOpts struct {
	Temporary bool
}

// Algorithm returns the key generation algorithm identifier (to be used).
func (opts *RSA3072KeyGenOpts) Algorithm() string {
	return RSA3072
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *RSA3072KeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}

// RSA4096KeyGenOpts contains options for RSA key generation at 4096 security.
type RSA4096KeyGenOpts struct {
	Temporary bool
}

// Algorithm returns the key generation algorithm identifier (to be used).
func (opts *RSA4096KeyGenOpts) Algorithm() string {
	return RSA4096
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *RSA4096KeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}
