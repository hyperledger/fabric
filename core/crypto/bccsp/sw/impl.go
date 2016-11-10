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
package sw

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"crypto/rsa"

	"hash"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	"github.com/op/go-logging"
)

var (
	logger = logging.MustGetLogger("SW_BCCSP")
)

// New returns a new instance of the software-based BCCSP.
func New() (bccsp.BCCSP, error) {
	conf := &config{}
	err := conf.init()
	if err != nil {
		return nil, fmt.Errorf("Failed initializing configuration [%s]", err)
	}

	ks := &keyStore{}
	if err := ks.init(nil, conf); err != nil {
		return nil, fmt.Errorf("Failed initializing key store [%s]", err)
	}
	return &impl{ks}, nil
}

// SoftwareBasedBCCSP is the software-based implementation of the BCCSP.
// It is based on code used in the primitives package.
// It can be configured via viper.
type impl struct {
	ks *keyStore
}

// KeyGen generates a key using opts.
func (csp *impl) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil.")
	}

	// Parse algorithm
	switch opts.Algorithm() {
	case bccsp.ECDSA:
		lowLevelKey, err := primitives.NewECDSAKey()
		if err != nil {
			return nil, fmt.Errorf("Failed generating ECDSA key [%s]", err)
		}

		k = &ecdsaPrivateKey{lowLevelKey}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storePrivateKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
			}
		}

		return k, nil
	case bccsp.AES:
		lowLevelKey, err := primitives.GenAESKey()

		if err != nil {
			return nil, fmt.Errorf("Failed generating AES key [%s]", err)
		}

		k = &aesPrivateKey{lowLevelKey, false}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storeKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing AES key [%s]", err)
			}
		}

		return k, nil
	case bccsp.RSA:
		lowLevelKey, err := primitives.NewRSAKey()

		if err != nil {
			return nil, fmt.Errorf("Failed generating RSA (2048) key [%s]", err)
		}

		k = &rsaPrivateKey{lowLevelKey}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storePrivateKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing AES key [%s]", err)
			}
		}

		return k, nil
	default:
		return nil, fmt.Errorf("Algorithm not recognized [%s]", opts.Algorithm())
	}
}

// KeyDeriv derives a key from k using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("Invalid Key. It must not be nil.")
	}

	// Derive key
	switch k.(type) {
	case *ecdsaPrivateKey:
		// Validate opts
		if opts == nil {
			return nil, errors.New("Invalid Opts parameter. It must not be nil.")
		}

		ecdsaK := k.(*ecdsaPrivateKey)

		switch opts.(type) {

		// Re-randomized an ECDSA private key
		case *bccsp.ECDSAReRandKeyOpts:
			reRandOpts := opts.(*bccsp.ECDSAReRandKeyOpts)
			tempSK := &ecdsa.PrivateKey{
				PublicKey: ecdsa.PublicKey{
					Curve: ecdsaK.k.Curve,
					X:     new(big.Int),
					Y:     new(big.Int),
				},
				D: new(big.Int),
			}

			var k = new(big.Int).SetBytes(reRandOpts.ExpansionValue())
			var one = new(big.Int).SetInt64(1)
			n := new(big.Int).Sub(ecdsaK.k.Params().N, one)
			k.Mod(k, n)
			k.Add(k, one)

			tempSK.D.Add(ecdsaK.k.D, k)
			tempSK.D.Mod(tempSK.D, ecdsaK.k.PublicKey.Params().N)

			// Compute temporary public key
			tempX, tempY := ecdsaK.k.PublicKey.ScalarBaseMult(k.Bytes())
			tempSK.PublicKey.X, tempSK.PublicKey.Y =
				tempSK.PublicKey.Add(
					ecdsaK.k.PublicKey.X, ecdsaK.k.PublicKey.Y,
					tempX, tempY,
				)

			// Verify temporary public key is a valid point on the reference curve
			isOn := tempSK.Curve.IsOnCurve(tempSK.PublicKey.X, tempSK.PublicKey.Y)
			if !isOn {
				return nil, errors.New("Failed temporary public key IsOnCurve check. This is an foreign key.")
			}

			reRandomizedKey := &ecdsaPrivateKey{tempSK}

			// If the key is not Ephemeral, store it.
			if !opts.Ephemeral() {
				// Store the key
				err = csp.ks.storePrivateKey(hex.EncodeToString(reRandomizedKey.SKI()), tempSK)
				if err != nil {
					return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
				}
			}

			return reRandomizedKey, nil

		default:
			return nil, errors.New("Opts not supported")

		}
	case *aesPrivateKey:
		// Validate opts
		if opts == nil {
			return nil, errors.New("Invalid Opts parameter. It must not be nil.")
		}

		aesK := k.(*aesPrivateKey)

		switch opts.(type) {
		case *bccsp.HMACTruncated256AESDeriveKeyOpts:
			hmacOpts := opts.(*bccsp.HMACTruncated256AESDeriveKeyOpts)

			hmacedKey := &aesPrivateKey{primitives.HMACAESTruncated(aesK.k, hmacOpts.Argument()), false}

			// If the key is not Ephemeral, store it.
			if !opts.Ephemeral() {
				// Store the key
				err = csp.ks.storeKey(hex.EncodeToString(hmacedKey.SKI()), hmacedKey.k)
				if err != nil {
					return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
				}
			}

			return hmacedKey, nil

		case *bccsp.HMACDeriveKeyOpts:

			hmacOpts := opts.(*bccsp.HMACDeriveKeyOpts)

			hmacedKey := &aesPrivateKey{primitives.HMAC(aesK.k, hmacOpts.Argument()), true}

			// If the key is not Ephemeral, store it.
			if !opts.Ephemeral() {
				// Store the key
				err = csp.ks.storeKey(hex.EncodeToString(hmacedKey.SKI()), hmacedKey.k)
				if err != nil {
					return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
				}
			}

			return hmacedKey, nil

		default:
			return nil, errors.New("Opts not supported")

		}

	default:
		return nil, fmt.Errorf("Key type not recognized [%s]", k)
	}
}

// KeyImport imports a key from its raw representation using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyImport(raw []byte, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil.")
	}

	switch opts.(type) {

	case *bccsp.AES256ImportKeyOpts:

		if len(raw) != 32 {
			return nil, fmt.Errorf("[AES256ImportKeyOpts] Invalid Key Length [%d]. Must be 32 bytes", len(raw))
		}

		aesK := &aesPrivateKey{utils.Clone(raw), false}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storeKey(hex.EncodeToString(aesK.SKI()), aesK.k)
			if err != nil {
				return nil, fmt.Errorf("Failed storing AES key [%s]", err)
			}
		}

		return aesK, nil

	case *bccsp.HMACImportKeyOpts:

		if len(raw) == 0 {
			return nil, errors.New("[HMACImportKeyOpts] Invalid raw. It must not be nil.")
		}

		aesK := &aesPrivateKey{utils.Clone(raw), false}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storeKey(hex.EncodeToString(aesK.SKI()), aesK.k)
			if err != nil {
				return nil, fmt.Errorf("Failed storing AES key [%s]", err)
			}
		}

		return aesK, nil

	case *bccsp.ECDSAPKIXPublicKeyImportOpts:

		if len(raw) == 0 {
			return nil, errors.New("[ECDSAPKIXPublicKeyImportOpts] Invalid raw. It must not be nil.")
		}

		lowLevelKey, err := primitives.DERToPublicKey(raw)
		if err != nil {
			return nil, fmt.Errorf("Failed converting PKIX to ECDSA public key [%s]", err)
		}

		ecdsaPK, ok := lowLevelKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("Failed casting to ECDSA public key. Invalid raw material.")
		}

		k = &ecdsaPublicKey{ecdsaPK}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storePublicKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
			}
		}

		return k, nil

	case *bccsp.ECDSAPrivateKeyImportOpts:

		if len(raw) == 0 {
			return nil, errors.New("[ECDSADERPrivateKeyImportOpts] Invalid raw. It must not be nil.")
		}

		lowLevelKey, err := primitives.DERToPrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("Failed converting PKIX to ECDSA public key [%s]", err)
		}

		ecdsaSK, ok := lowLevelKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("Failed casting to ECDSA public key. Invalid raw material.")
		}

		k = &ecdsaPrivateKey{ecdsaSK}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storePrivateKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
			}
		}

		return k, nil

	case *bccsp.ECDSAGoPublicKeyImportOpts:

		lowLevelKey := opts.(*bccsp.ECDSAGoPublicKeyImportOpts).PublicKey()
		if lowLevelKey == nil {
			return nil, errors.New("Invalid Opts. ECDSA Public key cannot be nil")
		}

		k = &ecdsaPublicKey{lowLevelKey}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storePublicKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
			}
		}

		return k, nil

	case *bccsp.RSAGoPublicKeyImportOpts:

		lowLevelKey := opts.(*bccsp.RSAGoPublicKeyImportOpts).PublicKey()
		if lowLevelKey == nil {
			return nil, errors.New("Invalid Opts. ECDSA Public key cannot be nil")
		}

		k = &rsaPublicKey{lowLevelKey}

		// If the key is not Ephemeral, store it.
		if !opts.Ephemeral() {
			// Store the key
			err = csp.ks.storePublicKey(hex.EncodeToString(k.SKI()), lowLevelKey)
			if err != nil {
				return nil, fmt.Errorf("Failed storing ECDSA key [%s]", err)
			}
		}

		return k, nil

	case *bccsp.X509PublicKeyImportOpts:

		x509Cert := opts.(*bccsp.X509PublicKeyImportOpts).Certificate()
		if x509Cert == nil {
			return nil, errors.New("Invalid Opts. X509 certificate cannot be nil")
		}

		pk := x509Cert.PublicKey

		switch pk.(type) {
		case *ecdsa.PublicKey:
			return csp.KeyImport(nil, &bccsp.ECDSAGoPublicKeyImportOpts{Temporary: opts.Ephemeral(), PK: pk.(*ecdsa.PublicKey)})
		case *rsa.PublicKey:
			return csp.KeyImport(nil, &bccsp.RSAGoPublicKeyImportOpts{Temporary: opts.Ephemeral(), PK: pk.(*rsa.PublicKey)})
		default:
			return nil, errors.New("Certificate public key type not recognized. Supported keys: [ECDSA, RSA]")
		}

	default:
		return nil, errors.New("Import Key Options not recognized")
	}
}

// GetKey returns the key this CSP associates to
// the Subject Key Identifier ski.
func (csp *impl) GetKey(ski []byte) (k bccsp.Key, err error) {
	// Validate arguments
	if len(ski) == 0 {
		return nil, errors.New("Invalid SKI. Cannot be of zero length.")
	}

	suffix := csp.ks.getSuffix(hex.EncodeToString(ski))

	switch suffix {
	case "key":
		// Load the key
		key, err := csp.ks.loadKey(hex.EncodeToString(ski))
		if err != nil {
			return nil, fmt.Errorf("Failed loading key [%x] [%s]", ski, err)
		}

		return &aesPrivateKey{key, false}, nil
	case "sk":
		// Load the private key
		key, err := csp.ks.loadPrivateKey(hex.EncodeToString(ski))
		if err != nil {
			return nil, fmt.Errorf("Failed loading key [%x] [%s]", ski, err)
		}

		switch key.(type) {
		case *ecdsa.PrivateKey:
			return &ecdsaPrivateKey{key.(*ecdsa.PrivateKey)}, nil
		case *rsa.PrivateKey:
			return &rsaPrivateKey{key.(*rsa.PrivateKey)}, nil
		default:
			return nil, errors.New("Key type not recognized")
		}
	default:
		return nil, errors.New("Key not recognized")
	}
}

// Hash hashes messages msg using options opts.
func (csp *impl) Hash(msg []byte, opts bccsp.HashOpts) (hash []byte, err error) {
	if opts == nil {
		return primitives.Hash(msg), nil
	}

	switch opts.Algorithm() {
	case bccsp.DefaultHash, bccsp.SHA:
		return primitives.Hash(msg), nil
	default:
		return nil, fmt.Errorf("Algorithm not recognized [%s]", opts.Algorithm())
	}
}

// GetHash returns and instance of hash.Hash using options opts.
// If opts is nil then the default hash function is returned.
func (csp *impl) GetHash(opts bccsp.HashOpts) (h hash.Hash, err error) {
	if opts == nil {
		return primitives.NewHash(), nil
	}

	switch opts.Algorithm() {
	case bccsp.SHA, bccsp.DefaultHash:
		return primitives.NewHash(), nil
	default:
		return nil, fmt.Errorf("Algorithm not recognized [%s]", opts.Algorithm())
	}
}

// Sign signs digest using key k.
// The opts argument should be appropriate for the primitive used.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest).
func (csp *impl) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("Invalid Key. It must not be nil.")
	}
	if len(digest) == 0 {
		return nil, errors.New("Invalid digest. Cannot be empty.")
	}

	// Check key type
	switch k.(type) {
	case *ecdsaPrivateKey:
		return k.(*ecdsaPrivateKey).k.Sign(rand.Reader, digest, nil)
	case *rsaPrivateKey:
		if opts == nil {
			return nil, errors.New("Invalid options. Nil.")
		}

		return k.(*rsaPrivateKey).k.Sign(rand.Reader, digest, opts)
	default:
		return nil, fmt.Errorf("Key type not recognized [%s]", k)
	}
}

// Verify verifies signature against key k and digest
func (csp *impl) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	// Validate arguments
	if k == nil {
		return false, errors.New("Invalid Key. It must not be nil.")
	}
	if len(signature) == 0 {
		return false, errors.New("Invalid signature. Cannot be empty.")
	}
	if len(digest) == 0 {
		return false, errors.New("Invalid digest. Cannot be empty.")
	}

	// Check key type
	switch k.(type) {
	case *ecdsaPrivateKey:
		ecdsaSignature := new(primitives.ECDSASignature)
		_, err := asn1.Unmarshal(signature, ecdsaSignature)
		if err != nil {
			return false, fmt.Errorf("Failed unmashalling signature [%s]", err)
		}

		return ecdsa.Verify(&(k.(*ecdsaPrivateKey).k.PublicKey), digest, ecdsaSignature.R, ecdsaSignature.S), nil
	case *ecdsaPublicKey:
		ecdsaSignature := new(primitives.ECDSASignature)
		_, err := asn1.Unmarshal(signature, ecdsaSignature)
		if err != nil {
			return false, fmt.Errorf("Failed unmashalling signature [%s]", err)
		}

		return ecdsa.Verify(k.(*ecdsaPublicKey).k, digest, ecdsaSignature.R, ecdsaSignature.S), nil
	case *rsaPrivateKey:
		if opts == nil {
			return false, errors.New("Invalid options. It must not be nil.")
		}
		switch opts.(type) {
		case *rsa.PSSOptions:
			err := rsa.VerifyPSS(&(k.(*rsaPrivateKey).k.PublicKey),
				(opts.(*rsa.PSSOptions)).Hash,
				digest, signature, opts.(*rsa.PSSOptions))

			return err == nil, err
		default:
			return false, fmt.Errorf("Opts type not recognized [%s]", opts)
		}
	case *rsaPublicKey:
		if opts == nil {
			return false, errors.New("Invalid options. It must not be nil.")
		}
		switch opts.(type) {
		case *rsa.PSSOptions:
			err := rsa.VerifyPSS(k.(*rsaPublicKey).k,
				(opts.(*rsa.PSSOptions)).Hash,
				digest, signature, opts.(*rsa.PSSOptions))

			return err == nil, err
		default:
			return false, fmt.Errorf("Opts type not recognized [%s]", opts)
		}
	default:
		return false, fmt.Errorf("Key type not recognized [%s]", k)
	}
}

// Encrypt encrypts plaintext using key k.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("Invalid Key. It must not be nil.")
	}

	// Check key type
	switch k.(type) {
	case *aesPrivateKey:
		// check for mode
		switch opts.(type) {
		case *bccsp.AESCBCPKCS7ModeOpts, bccsp.AESCBCPKCS7ModeOpts:
			// AES in CBC mode with PKCS7 padding
			return primitives.CBCPKCS7Encrypt(k.(*aesPrivateKey).k, plaintext)
		default:
			return nil, fmt.Errorf("Mode not recognized [%s]", opts)
		}
	default:
		return nil, fmt.Errorf("Key type not recognized [%s]", k)
	}
}

// Decrypt decrypts ciphertext using key k.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("Invalid Key. It must not be nil.")
	}

	// Check key type
	switch k.(type) {
	case *aesPrivateKey:
		// check for mode
		switch opts.(type) {
		case *bccsp.AESCBCPKCS7ModeOpts, bccsp.AESCBCPKCS7ModeOpts:
			// AES in CBC mode with PKCS7 padding
			return primitives.CBCPKCS7Decrypt(k.(*aesPrivateKey).k, ciphertext)
		default:
			return nil, fmt.Errorf("Mode not recognized [%s]", opts)
		}
	default:
		return nil, fmt.Errorf("Key type not recognized [%s]", k)
	}
}
