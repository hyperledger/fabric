/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bccsp

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
)

const (
	// ECDSA Elliptic Curve Digital Signature Algorithm (key gen, import, sign, verify),
	// at default security level (see primitives package).
	ECDSA = "ECDSA"
	// ECDSAReRand ECDSA key re-randomization
	ECDSAReRand = "ECDSA_RERAND"

	// RSA at default security level (see primitives package)
	RSA = "RSA"

	// AES Advanced Encryption Standard at default security level (see primitives package)
	AES = "AES"

	// HMAC keyed-hash message authentication code
	HMAC = "HMAC"
	// HMACTruncated256 HMAC truncated at 256 bits.
	HMACTruncated256 = "HMAC_TRUNCATED_256"

	// SHA Secure Hash Algorithm using default family (see primitives package)
	SHA = "SHA"

	// X509Certificate Label for X509 certificate realted operation
	X509Certificate = "X509Certificate"

	// DefaultHash is the identifier for the default hash function (see primitives package)
	DefaultHash = "DEFAULT_HASH"
)

// ECDSAKeyGenOpts contains options for ECDSA key generation.
type ECDSAKeyGenOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *ECDSAKeyGenOpts) Algorithm() string {
	return ECDSA
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *ECDSAKeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}

// ECDSAPKIXPublicKeyImportOpts contains options for ECDSA public key importation in PKIX format
type ECDSAPKIXPublicKeyImportOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *ECDSAPKIXPublicKeyImportOpts) Algorithm() string {
	return ECDSA
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *ECDSAPKIXPublicKeyImportOpts) Ephemeral() bool {
	return opts.Temporary
}

// ECDSAPrivateKeyImportOpts contains options for ECDSA secret key importation in DER format
// or PKCS#8 format.
type ECDSAPrivateKeyImportOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *ECDSAPrivateKeyImportOpts) Algorithm() string {
	return ECDSA
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *ECDSAPrivateKeyImportOpts) Ephemeral() bool {
	return opts.Temporary
}

// ECDSAGoPublicKeyImportOpts contains options for ECDSA key importation from ecdsa.PublicKey
type ECDSAGoPublicKeyImportOpts struct {
	Temporary bool
	PK        *ecdsa.PublicKey
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *ECDSAGoPublicKeyImportOpts) Algorithm() string {
	return ECDSA
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *ECDSAGoPublicKeyImportOpts) Ephemeral() bool {
	return opts.Temporary
}

// PublicKey returns the ecdsa.PublicKey to be imported.
func (opts *ECDSAGoPublicKeyImportOpts) PublicKey() *ecdsa.PublicKey {
	return opts.PK
}

// ECDSAReRandKeyOpts contains options for ECDSA key re-randomization.
type ECDSAReRandKeyOpts struct {
	Temporary bool
	Expansion []byte
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *ECDSAReRandKeyOpts) Algorithm() string {
	return ECDSAReRand
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *ECDSAReRandKeyOpts) Ephemeral() bool {
	return opts.Temporary
}

// ExpansionValue returns the re-randomization factor
func (opts *ECDSAReRandKeyOpts) ExpansionValue() []byte {
	return opts.Expansion
}

// AES256KeyGenOpts contains options for AES256 key generation.
type AESKeyGenOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *AESKeyGenOpts) Algorithm() string {
	return AES
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *AESKeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}

// AESCBCPKCS7ModeOpts contains options for AES encryption in CBC mode
// with PKCS7 padding.
type AESCBCPKCS7ModeOpts struct{}

// HMACTruncated256AESDeriveKeyOpts contains options for HMAC truncated
// at 256 bits key derivation.
type HMACTruncated256AESDeriveKeyOpts struct {
	Temporary bool
	Arg       []byte
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *HMACTruncated256AESDeriveKeyOpts) Algorithm() string {
	return HMACTruncated256
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *HMACTruncated256AESDeriveKeyOpts) Ephemeral() bool {
	return opts.Temporary
}

// Argument returns the argument to be passed to the HMAC
func (opts *HMACTruncated256AESDeriveKeyOpts) Argument() []byte {
	return opts.Arg
}

// HMACDeriveKeyOpts contains options for HMAC key derivation.
type HMACDeriveKeyOpts struct {
	Temporary bool
	Arg       []byte
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *HMACDeriveKeyOpts) Algorithm() string {
	return HMAC
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *HMACDeriveKeyOpts) Ephemeral() bool {
	return opts.Temporary
}

// Argument returns the argument to be passed to the HMAC
func (opts *HMACDeriveKeyOpts) Argument() []byte {
	return opts.Arg
}

// AES256ImportKeyOpts contains options for importing AES 256 keys.
type AES256ImportKeyOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to import the raw material of a key.
func (opts *AES256ImportKeyOpts) Algorithm() string {
	return AES
}

// Ephemeral returns true if the key generated has to be ephemeral,
// false otherwise.
func (opts *AES256ImportKeyOpts) Ephemeral() bool {
	return opts.Temporary
}

// HMACImportKeyOpts contains options for importing HMAC keys.
type HMACImportKeyOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to import the raw material of a key.
func (opts *HMACImportKeyOpts) Algorithm() string {
	return HMAC
}

// Ephemeral returns true if the key generated has to be ephemeral,
// false otherwise.
func (opts *HMACImportKeyOpts) Ephemeral() bool {
	return opts.Temporary
}

// SHAOpts contains options for computing SHA.
type SHAOpts struct {
}

// Algorithm returns an identifier for the algorithm to be used
// to hash.
func (opts *SHAOpts) Algorithm() string {
	return SHA
}

// RSAKeyGenOpts contains options for RSA key generation.
type RSAKeyGenOpts struct {
	Temporary bool
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *RSAKeyGenOpts) Algorithm() string {
	return RSA
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *RSAKeyGenOpts) Ephemeral() bool {
	return opts.Temporary
}

// ECDSAGoPublicKeyImportOpts contains options for RSA key importation from rsa.PublicKey
type RSAGoPublicKeyImportOpts struct {
	Temporary bool
	PK        *rsa.PublicKey
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *RSAGoPublicKeyImportOpts) Algorithm() string {
	return RSA
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *RSAGoPublicKeyImportOpts) Ephemeral() bool {
	return opts.Temporary
}

// PublicKey returns the ecdsa.PublicKey to be imported.
func (opts *RSAGoPublicKeyImportOpts) PublicKey() *rsa.PublicKey {
	return opts.PK
}

// X509PublicKeyImportOpts contains options for importing public keys from an x509 certificate
type X509PublicKeyImportOpts struct {
	Temporary bool
	Cert      *x509.Certificate
}

// Algorithm returns an identifier for the algorithm to be used
// to generate a key.
func (opts *X509PublicKeyImportOpts) Algorithm() string {
	return X509Certificate
}

// Ephemeral returns true if the key to generate has to be ephemeral,
// false otherwise.
func (opts *X509PublicKeyImportOpts) Ephemeral() bool {
	return opts.Temporary
}

// PublicKey returns the ecdsa.PublicKey to be imported.
func (opts *X509PublicKeyImportOpts) Certificate() *x509.Certificate {
	return opts.Cert
}
