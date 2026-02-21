/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sw

import (
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"hash"
	"reflect"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// NewDefaultSecurityLevel returns a new instance of the software-based BCCSP
// at security level 256, hash family SHA2 and using FolderBasedKeyStore as KeyStore.
func NewDefaultSecurityLevel(keyStorePath string) (bccsp.BCCSP, error) {
	ks := &fileBasedKeyStore{}
	if err := ks.Init(nil, keyStorePath, false); err != nil {
		return nil, errors.Wrapf(err, "Failed initializing key store at [%v]", keyStorePath)
	}

	return NewWithParams(256, "SHA2", ks)
}

// NewDefaultSecurityLevel returns a new instance of the software-based BCCSP
// at security level 256, hash family SHA2 and using the passed KeyStore.
func NewDefaultSecurityLevelWithKeystore(keyStore bccsp.KeyStore) (bccsp.BCCSP, error) {
	return NewWithParams(256, "SHA2", keyStore)
}

// NewWithParams returns a new instance of the software-based BCCSP
// set at the passed security level, hash family and KeyStore.
func NewWithParams(securityLevel int, hashFamily string, keyStore bccsp.KeyStore) (bccsp.BCCSP, error) {
	// Init config
	conf := &config{}
	err := conf.setSecurityLevel(securityLevel, hashFamily)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed initializing configuration at [%v,%v]", securityLevel, hashFamily)
	}

	swbccsp, err := New(keyStore)
	if err != nil {
		return nil, err
	}

	// Notice that errors are ignored here because some test will fail if one
	// of the following call fails.

	// Set the Encryptors
	swbccsp.AddWrapper(reflect.TypeFor[*aesPrivateKey](), &aescbcpkcs7Encryptor{})

	// Set the Decryptors
	swbccsp.AddWrapper(reflect.TypeFor[*aesPrivateKey](), &aescbcpkcs7Decryptor{})

	// Set the Signers
	swbccsp.AddWrapper(reflect.TypeFor[*ecdsaPrivateKey](), &ecdsaSigner{})

	// Set the Verifiers
	swbccsp.AddWrapper(reflect.TypeFor[*ecdsaPrivateKey](), &ecdsaPrivateKeyVerifier{})
	swbccsp.AddWrapper(reflect.TypeFor[*ecdsaPublicKey](), &ecdsaPublicKeyKeyVerifier{})

	// Set the Hashers
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.SHAOpts](), &hasher{hash: conf.hashFunction})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.SHA256Opts](), &hasher{hash: sha256.New})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.SHA384Opts](), &hasher{hash: sha512.New384})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.SHA3_256Opts](), &hasher{hash: func() hash.Hash { return sha3.New256() }})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.SHA3_384Opts](), &hasher{hash: func() hash.Hash { return sha3.New384() }})

	// Set the key generators
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.ECDSAKeyGenOpts](), &ecdsaKeyGenerator{curve: conf.ellipticCurve})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.ECDSAP256KeyGenOpts](), &ecdsaKeyGenerator{curve: elliptic.P256()})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.ECDSAP384KeyGenOpts](), &ecdsaKeyGenerator{curve: elliptic.P384()})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.AESKeyGenOpts](), &aesKeyGenerator{length: conf.aesBitLength})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.AES256KeyGenOpts](), &aesKeyGenerator{length: 32})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.AES192KeyGenOpts](), &aesKeyGenerator{length: 24})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.AES128KeyGenOpts](), &aesKeyGenerator{length: 16})

	// Set the key deriver
	swbccsp.AddWrapper(reflect.TypeFor[*ecdsaPrivateKey](), &ecdsaPrivateKeyKeyDeriver{})
	swbccsp.AddWrapper(reflect.TypeFor[*ecdsaPublicKey](), &ecdsaPublicKeyKeyDeriver{})
	swbccsp.AddWrapper(reflect.TypeFor[*aesPrivateKey](), &aesPrivateKeyKeyDeriver{conf: conf})

	// Set the key importers
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.AES256ImportKeyOpts](), &aes256ImportKeyOptsKeyImporter{})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.HMACImportKeyOpts](), &hmacImportKeyOptsKeyImporter{})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.ECDSAPKIXPublicKeyImportOpts](), &ecdsaPKIXPublicKeyImportOptsKeyImporter{})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.ECDSAPrivateKeyImportOpts](), &ecdsaPrivateKeyImportOptsKeyImporter{})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.ECDSAGoPublicKeyImportOpts](), &ecdsaGoPublicKeyImportOptsKeyImporter{})
	swbccsp.AddWrapper(reflect.TypeFor[*bccsp.X509PublicKeyImportOpts](), &x509PublicKeyImportOptsKeyImporter{bccsp: swbccsp})

	return swbccsp, nil
}
