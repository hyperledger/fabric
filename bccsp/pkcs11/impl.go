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
package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"math/big"
	"os"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/miekg/pkcs11"
	"github.com/pkg/errors"
)

var (
	logger           = flogging.MustGetLogger("bccsp_p11")
	sessionCacheSize = 10
)

// New returns a new instance of the software-based BCCSP
// set at the passed security level, hash family and KeyStore.
func New(opts PKCS11Opts, keyStore bccsp.KeyStore) (bccsp.BCCSP, error) {
	// Init config
	conf := &config{}
	err := conf.setSecurityLevel(opts.SecLevel, opts.HashFamily)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed initializing configuration")
	}

	swCSP, err := sw.New(opts.SecLevel, opts.HashFamily, keyStore)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed initializing fallback SW BCCSP")
	}

	// Check KeyStore
	if keyStore == nil {
		return nil, errors.New("Invalid bccsp.KeyStore instance. It must be different from nil.")
	}

	lib := opts.Library
	pin := opts.Pin
	label := opts.Label
	ctx, slot, session, err := loadLib(lib, pin, label)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed initializing PKCS11 library %s %s",
			lib, label)
	}

	sessions := make(chan pkcs11.SessionHandle, sessionCacheSize)
	csp := &impl{swCSP, conf, keyStore, ctx, sessions, slot, lib, opts.Sensitive, opts.SoftVerify}
	csp.returnSession(*session)
	return csp, nil
}

type impl struct {
	bccsp.BCCSP

	conf *config
	ks   bccsp.KeyStore

	ctx      *pkcs11.Ctx
	sessions chan pkcs11.SessionHandle
	slot     uint

	lib          string
	noPrivImport bool
	softVerify   bool
}

// KeyGen generates a key using opts.
func (csp *impl) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil.")
	}

	// Parse algorithm
	switch opts.(type) {
	case *bccsp.ECDSAKeyGenOpts:
		ski, pub, err := csp.generateECKey(csp.conf.ellipticCurve, opts.Ephemeral())
		if err != nil {
			return nil, errors.Wrapf(err, "Failed generating ECDSA key")
		}
		k = &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, pub}}

	case *bccsp.ECDSAP256KeyGenOpts:
		ski, pub, err := csp.generateECKey(oidNamedCurveP256, opts.Ephemeral())
		if err != nil {
			return nil, errors.Wrapf(err, "Failed generating ECDSA P256 key")
		}

		k = &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, pub}}

	case *bccsp.ECDSAP384KeyGenOpts:
		ski, pub, err := csp.generateECKey(oidNamedCurveP384, opts.Ephemeral())
		if err != nil {
			return nil, errors.Wrapf(err, "Failed generating ECDSA P384 key [%s]")
		}

		k = &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, pub}}

	default:
		return csp.BCCSP.KeyGen(opts)
	}

	return k, nil
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
	case *ecdsaPublicKey:
		// Validate opts
		if opts == nil {
			return nil, errors.New("Invalid Opts parameter. It must not be nil.")
		}

		ecdsaK := k.(*ecdsaPublicKey)

		switch opts.(type) {

		// Re-randomized an ECDSA public key
		case *bccsp.ECDSAReRandKeyOpts:
			pubKey := ecdsaK.pub
			if pubKey == nil {
				return nil, errors.New("Public base key cannot be nil.")
			}
			reRandOpts := opts.(*bccsp.ECDSAReRandKeyOpts)
			tempSK := &ecdsa.PublicKey{
				Curve: pubKey.Curve,
				X:     new(big.Int),
				Y:     new(big.Int),
			}

			var k = new(big.Int).SetBytes(reRandOpts.ExpansionValue())
			var one = new(big.Int).SetInt64(1)
			n := new(big.Int).Sub(pubKey.Params().N, one)
			k.Mod(k, n)
			k.Add(k, one)

			// Compute temporary public key
			tempX, tempY := pubKey.ScalarBaseMult(k.Bytes())
			tempSK.X, tempSK.Y = tempSK.Add(
				pubKey.X, pubKey.Y,
				tempX, tempY,
			)

			// Verify temporary public key is a valid point on the reference curve
			isOn := tempSK.Curve.IsOnCurve(tempSK.X, tempSK.Y)
			if !isOn {
				return nil, errors.New("Failed temporary public key IsOnCurve check.")
			}

			ecPt := elliptic.Marshal(tempSK.Curve, tempSK.X, tempSK.Y)
			oid, ok := oidFromNamedCurve(tempSK.Curve)
			if !ok {
				return nil, errors.New("Do not know OID for this Curve.")
			}

			ski, err := csp.importECKey(oid, nil, ecPt, opts.Ephemeral(), publicKeyFlag)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed getting importing EC Public Key")
			}
			reRandomizedKey := &ecdsaPublicKey{ski, tempSK}

			return reRandomizedKey, nil

		default:
			return nil, errors.Errorf("Unrecognized KeyDerivOpts provided [%s]", opts.Algorithm())

		}
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
			pubKey := ecdsaK.pub.pub
			if pubKey == nil {
				return nil, errors.New("Public base key cannot be nil.")
			}

			secret := csp.getSecretValue(ecdsaK.ski)
			if secret == nil {
				return nil, errors.New("Could not obtain EC Private Key")
			}
			bigSecret := new(big.Int).SetBytes(secret)

			tempSK := &ecdsa.PrivateKey{
				PublicKey: ecdsa.PublicKey{
					Curve: pubKey.Curve,
					X:     new(big.Int),
					Y:     new(big.Int),
				},
				D: new(big.Int),
			}

			var k = new(big.Int).SetBytes(reRandOpts.ExpansionValue())
			var one = new(big.Int).SetInt64(1)
			n := new(big.Int).Sub(pubKey.Params().N, one)
			k.Mod(k, n)
			k.Add(k, one)

			tempSK.D.Add(bigSecret, k)
			tempSK.D.Mod(tempSK.D, pubKey.Params().N)

			// Compute temporary public key
			tempSK.PublicKey.X, tempSK.PublicKey.Y = pubKey.ScalarBaseMult(tempSK.D.Bytes())

			// Verify temporary public key is a valid point on the reference curve
			isOn := tempSK.Curve.IsOnCurve(tempSK.PublicKey.X, tempSK.PublicKey.Y)
			if !isOn {
				return nil, errors.New("Failed temporary public key IsOnCurve check.")
			}

			ecPt := elliptic.Marshal(tempSK.Curve, tempSK.X, tempSK.Y)
			oid, ok := oidFromNamedCurve(tempSK.Curve)
			if !ok {
				return nil, errors.New("Do not know OID for this Curve.")
			}

			ski, err := csp.importECKey(oid, tempSK.D.Bytes(), ecPt, opts.Ephemeral(), privateKeyFlag)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed getting importing EC Public Key")
			}
			reRandomizedKey := &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, &tempSK.PublicKey}}

			return reRandomizedKey, nil

		default:
			return nil, errors.Errorf("Unrecognized KeyDerivOpts provided [%s]", opts.Algorithm())

		}

	default:
		return csp.BCCSP.KeyDeriv(k, opts)

	}
}

// KeyImport imports a key from its raw representation using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if raw == nil {
		return nil, errors.New("Invalid raw. Cannot be nil.")
	}

	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil.")
	}

	switch opts.(type) {

	case *bccsp.ECDSAPKIXPublicKeyImportOpts:
		der, ok := raw.([]byte)
		if !ok {
			return nil, errors.New("[ECDSAPKIXPublicKeyImportOpts] Invalid raw material. Expected byte array.")
		}

		if len(der) == 0 {
			return nil, errors.New("[ECDSAPKIXPublicKeyImportOpts] Invalid raw. It must not be nil.")
		}

		lowLevelKey, err := utils.DERToPublicKey(der)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed converting PKIX to ECDSA public key")
		}

		ecdsaPK, ok := lowLevelKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("Failed casting to ECDSA public key. Invalid raw material.")
		}

		ecPt := elliptic.Marshal(ecdsaPK.Curve, ecdsaPK.X, ecdsaPK.Y)
		oid, ok := oidFromNamedCurve(ecdsaPK.Curve)
		if !ok {
			return nil, errors.New("Do not know OID for this Curve.")
		}

		var ski []byte
		if csp.noPrivImport {
			// opencryptoki does not support public ec key imports. This is a sufficient
			// workaround for now to use soft verify
			hash := sha256.Sum256(ecPt)
			ski = hash[:]
		} else {
			// Warn about potential future problems
			if !csp.softVerify {
				logger.Debugf("opencryptoki workaround warning: Importing public EC Key does not store out to pkcs11 store,\n" +
					"so verify with this key will fail, unless key is already present in store. Enable 'softwareverify'\n" +
					"in pkcs11 options, if suspect this issue.")
			}
			ski, err = csp.importECKey(oid, nil, ecPt, opts.Ephemeral(), publicKeyFlag)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed getting importing EC Public Key")
			}
		}

		k = &ecdsaPublicKey{ski, ecdsaPK}
		return k, nil

	case *bccsp.ECDSAPrivateKeyImportOpts:
		if csp.noPrivImport {
			return nil, errors.New("[ECDSADERPrivateKeyImportOpts] PKCS11 options 'sensitivekeys' is set to true. Cannot import.")
		}

		der, ok := raw.([]byte)
		if !ok {
			return nil, errors.New("[ECDSADERPrivateKeyImportOpts] Invalid raw material. Expected byte array.")
		}

		if len(der) == 0 {
			return nil, errors.New("[ECDSADERPrivateKeyImportOpts] Invalid raw. It must not be nil.")
		}

		lowLevelKey, err := utils.DERToPrivateKey(der)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed converting PKIX to ECDSA public key [%s]")
		}

		ecdsaSK, ok := lowLevelKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("Failed casting to ECDSA public key. Invalid raw material.")
		}

		ecPt := elliptic.Marshal(ecdsaSK.Curve, ecdsaSK.X, ecdsaSK.Y)
		oid, ok := oidFromNamedCurve(ecdsaSK.Curve)
		if !ok {
			return nil, errors.New("Do not know OID for this Curve.")
		}

		ski, err := csp.importECKey(oid, ecdsaSK.D.Bytes(), ecPt, opts.Ephemeral(), privateKeyFlag)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed getting importing EC Private Key")
		}

		k = &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, &ecdsaSK.PublicKey}}
		return k, nil

	case *bccsp.ECDSAGoPublicKeyImportOpts:
		lowLevelKey, ok := raw.(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("[ECDSAGoPublicKeyImportOpts] Invalid raw material. Expected *ecdsa.PublicKey.")
		}

		ecPt := elliptic.Marshal(lowLevelKey.Curve, lowLevelKey.X, lowLevelKey.Y)
		oid, ok := oidFromNamedCurve(lowLevelKey.Curve)
		if !ok {
			return nil, errors.New("Do not know OID for this Curve.")
		}

		var ski []byte
		if csp.noPrivImport {
			// opencryptoki does not support public ec key imports. This is a sufficient
			// workaround for now to use soft verify
			hash := sha256.Sum256(ecPt)
			ski = hash[:]
		} else {
			// Warn about potential future problems
			if !csp.softVerify {
				logger.Debugf("opencryptoki workaround warning: Importing public EC Key does not store out to pkcs11 store,\n" +
					"so verify with this key will fail, unless key is already present in store. Enable 'softwareverify'\n" +
					"in pkcs11 options, if suspect this issue.")
			}
			ski, err = csp.importECKey(oid, nil, ecPt, opts.Ephemeral(), publicKeyFlag)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed getting importing EC Public Key")
			}
		}

		k = &ecdsaPublicKey{ski, lowLevelKey}
		return k, nil

	case *bccsp.X509PublicKeyImportOpts:
		x509Cert, ok := raw.(*x509.Certificate)
		if !ok {
			return nil, errors.New("[X509PublicKeyImportOpts] Invalid raw material. Expected *x509.Certificate.")
		}

		pk := x509Cert.PublicKey

		switch pk.(type) {
		case *ecdsa.PublicKey:
			return csp.KeyImport(pk, &bccsp.ECDSAGoPublicKeyImportOpts{Temporary: opts.Ephemeral()})
		case *rsa.PublicKey:
			return csp.KeyImport(pk, &bccsp.RSAGoPublicKeyImportOpts{Temporary: opts.Ephemeral()})
		default:
			return nil, errors.New("Certificate's public key type not recognized. Supported keys: [ECDSA, RSA]")
		}

	default:
		return csp.BCCSP.KeyImport(raw, opts)

	}
}

// GetKey returns the key this CSP associates to
// the Subject Key Identifier ski.
func (csp *impl) GetKey(ski []byte) (k bccsp.Key, err error) {
	pubKey, isPriv, err := csp.getECKey(ski)
	if err == nil {
		if isPriv {
			return &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, pubKey}}, nil
		} else {
			return &ecdsaPublicKey{ski, pubKey}, nil
		}
	}
	return csp.BCCSP.GetKey(ski)
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
		return csp.signECDSA(*k.(*ecdsaPrivateKey), digest, opts)
	default:
		return csp.BCCSP.Sign(k, digest, opts)
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
		return csp.verifyECDSA(k.(*ecdsaPrivateKey).pub, signature, digest, opts)
	case *ecdsaPublicKey:
		return csp.verifyECDSA(*k.(*ecdsaPublicKey), signature, digest, opts)
	default:
		return csp.BCCSP.Verify(k, signature, digest, opts)
	}
}

// Encrypt encrypts plaintext using key k.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {
	// TODO: Add PKCS11 support for encryption, when fabric starts requiring it
	return csp.BCCSP.Encrypt(k, plaintext, opts)
}

// Decrypt decrypts ciphertext using key k.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {
	return csp.BCCSP.Decrypt(k, ciphertext, opts)
}

// THIS IS ONLY USED FOR TESTING
// This is a convenience function. Useful to self-configure, for tests where usual configuration is not
// available
func FindPKCS11Lib() (lib, pin, label string) {
	//FIXME: Till we workout the configuration piece, look for the libraries in the familiar places
	lib = os.Getenv("PKCS11_LIB")
	if lib == "" {
		pin = "98765432"
		label = "ForFabric"
		possibilities := []string{
			"/usr/lib/softhsm/libsofthsm2.so",                            //Debian
			"/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so",           //Ubuntu
			"/usr/lib/s390x-linux-gnu/softhsm/libsofthsm2.so",            //Ubuntu
			"/usr/lib/powerpc64le-linux-gnu/softhsm/libsofthsm2.so",      //Power
			"/usr/local/Cellar/softhsm/2.1.0/lib/softhsm/libsofthsm2.so", //MacOS
		}
		for _, path := range possibilities {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				lib = path
				break
			}
		}
	} else {
		pin = os.Getenv("PKCS11_PIN")
		label = os.Getenv("PKCS11_LABEL")
	}
	return lib, pin, label
}
