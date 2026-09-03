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
package idemix

import (
	"errors"
	"fmt"
	"reflect"

	bccsp "github.com/IBM/idemix/bccsp/types"
)

// KeyGenerator is a BCCSP-like interface that provides key generation algorithms
type KeyGenerator interface {

	// KeyGen generates a key using opts.
	KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error)
}

// KeyDeriver is a BCCSP-like interface that provides key derivation algorithms
type KeyDeriver interface {

	// KeyDeriv derives a key from k using opts.
	// The opts argument should be appropriate for the primitive used.
	KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error)
}

// KeyImporter is a BCCSP-like interface that provides key import algorithms
type KeyImporter interface {

	// KeyImport imports a key from its raw representation using opts.
	// The opts argument should be appropriate for the primitive used.
	KeyImport(raw any, opts bccsp.KeyImportOpts) (k bccsp.Key, err error)
}

// Signer is a BCCSP-like interface that provides signing algorithms
type Signer interface {

	// Sign signs digest using key k.
	// The opts argument should be appropriate for the algorithm used.
	//
	// Note that when a signature of a hash of a larger message is needed,
	// the caller is responsible for hashing the larger message and passing
	// the hash (as digest).
	Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error)
}

// Verifier is a BCCSP-like interface that provides verifying algorithms
type Verifier interface {

	// Verify verifies signature against key k and digest
	// The opts argument should be appropriate for the algorithm used.
	Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error)
}

// CSP provides a generic implementation of the BCCSP interface based
// on wrappers. It can be customized by providing implementations for the
// following algorithm-based wrappers: KeyGenerator, KeyDeriver, KeyImporter,
// Signer, Verifier. Each wrapper is bound to a Go type representing either
// an option or a key.
type CSP struct {
	ks bccsp.KeyStore

	KeyGenerators map[reflect.Type]KeyGenerator
	KeyDerivers   map[reflect.Type]KeyDeriver
	KeyImporters  map[reflect.Type]KeyImporter
	Signers       map[reflect.Type]Signer
	Verifiers     map[reflect.Type]Verifier
}

func NewImpl(keyStore bccsp.KeyStore) (*CSP, error) {
	if keyStore == nil {
		return nil, errors.New("invalid bccsp.KeyStore instance, it must be different from nil")
	}

	signers := make(map[reflect.Type]Signer)
	verifiers := make(map[reflect.Type]Verifier)
	keyGenerators := make(map[reflect.Type]KeyGenerator)
	keyDerivers := make(map[reflect.Type]KeyDeriver)
	keyImporters := make(map[reflect.Type]KeyImporter)

	csp := &CSP{keyStore,
		keyGenerators, keyDerivers, keyImporters,
		signers, verifiers}

	return csp, nil
}

// KeyGen generates a key using opts.
func (csp *CSP) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("invalid Opts parameter, it must not be nil")
	}

	keyGenerator, found := csp.KeyGenerators[reflect.TypeOf(opts)]
	if !found {
		return nil, fmt.Errorf("unsupported 'KeyGenOpts' provided [%v]", opts)
	}

	k, err = keyGenerator.KeyGen(opts)
	if err != nil {
		return nil, fmt.Errorf("failed generating key with opts [%v]: %w", opts, err)
	}

	// If the key is not Ephemeral, store it.
	if !opts.Ephemeral() {
		// Store the key
		err = csp.ks.StoreKey(k)
		if err != nil {
			return nil, fmt.Errorf("failed storing key [%v]: %w", reflect.TypeOf(opts), err)
		}
	}

	return k, nil
}

// KeyDeriv derives a key from k using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *CSP) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("invalid Key. It must not be nil")
	}
	if opts == nil {
		return nil, errors.New("invalid opts. It must not be nil")
	}

	keyDeriver, found := csp.KeyDerivers[reflect.TypeOf(k)]
	if !found {
		return nil, fmt.Errorf("unsupported 'Key' provided [%v]", k)
	}

	k, err = keyDeriver.KeyDeriv(k, opts)
	if err != nil {
		return nil, fmt.Errorf("failed deriving key with opts [%v]: %w", opts, err)
	}

	// If the key is not Ephemeral, store it.
	if !opts.Ephemeral() {
		// Store the key
		err = csp.ks.StoreKey(k)
		if err != nil {
			return nil, fmt.Errorf("failed storing key [%v]: %w", reflect.TypeOf(opts), err)
		}
	}

	return k, nil
}

// KeyImport imports a key from its raw representation using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *CSP) KeyImport(raw any, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if raw == nil {
		return nil, errors.New("invalid raw, it must not be nil")
	}
	if opts == nil {
		return nil, errors.New("invalid opts, it must not be nil")
	}

	keyImporter, found := csp.KeyImporters[reflect.TypeOf(opts)]
	if !found {
		return nil, fmt.Errorf("unsupported 'KeyImportOpts' provided [%v]", opts)
	}

	k, err = keyImporter.KeyImport(raw, opts)
	if err != nil {
		return nil, fmt.Errorf("failed importing key with opts [%v]: %w", opts, err)
	}

	// If the key is not Ephemeral, store it.
	if !opts.Ephemeral() {
		// Store the key
		err = csp.ks.StoreKey(k)
		if err != nil {
			return nil, fmt.Errorf("failed storing imported key with opts [%v]: %w", opts, err)
		}
	}

	return
}

// GetKey returns the key this CSP associates to
// the Subject Key Identifier ski.
func (csp *CSP) GetKey(ski []byte) (k bccsp.Key, err error) {
	k, err = csp.ks.GetKey(ski)
	if err != nil {
		return nil, fmt.Errorf("failed getting key for SKI [%v]: %w", ski, err)
	}

	return
}

// Sign signs digest using key k.
// The opts argument should be appropriate for the primitive used.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest).
func (csp *CSP) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("invalid Key. It must not be nil")
	}
	if len(digest) == 0 {
		return nil, errors.New("invalid digest. Cannot be empty")
	}

	keyType := reflect.TypeOf(k)
	signer, found := csp.Signers[keyType]
	if !found {
		return nil, fmt.Errorf("unsupported 'SignKey' provided [%s]", keyType)
	}

	signature, err = signer.Sign(k, digest, opts)
	if err != nil {
		return nil, fmt.Errorf("failed signing with opts [%v]: %w", opts, err)
	}

	return
}

// Verify verifies signature against key k and digest
func (csp *CSP) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	// Validate arguments
	if k == nil {
		return false, errors.New("invalid Key. It must not be nil")
	}
	if len(signature) == 0 {
		return false, errors.New("invalid signature. Cannot be empty")
	}
	if len(digest) == 0 {
		return false, errors.New("invalid digest. Cannot be empty")
	}

	verifier, found := csp.Verifiers[reflect.TypeOf(k)]
	if !found {
		return false, fmt.Errorf("unsupported 'VerifyKey' provided [%v]", k)
	}

	valid, err = verifier.Verify(k, signature, digest, opts)
	if err != nil {
		return false, fmt.Errorf("failed verifing with opts [%v]: %w", opts, err)
	}

	return
}

// AddWrapper binds the passed type to the passed wrapper.
// Notice that wrapper must be an instance of one of the following interfaces:
// KeyGenerator, KeyDeriver, KeyImporter, Signer, Verifier.
func (csp *CSP) AddWrapper(t reflect.Type, w any) error {
	if t == nil {
		return errors.New("type cannot be nil")
	}
	if w == nil {
		return errors.New("wrapper cannot be nil")
	}
	switch dt := w.(type) {
	case KeyGenerator:
		csp.KeyGenerators[t] = dt
	case KeyImporter:
		csp.KeyImporters[t] = dt
	case KeyDeriver:
		csp.KeyDerivers[t] = dt
	case Signer:
		csp.Signers[t] = dt
	case Verifier:
		csp.Verifiers[t] = dt
	default:
		return errors.New("wrapper type not valid, must be one of: KeyGenerator, KeyDeriver, KeyImporter, Signer, Verifier")
	}

	return nil
}
