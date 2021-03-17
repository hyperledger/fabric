/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/miekg/pkcs11"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const createSessionRetries = 10

var (
	logger           = flogging.MustGetLogger("bccsp_p11")
	regex            = regexp.MustCompile(".*0xB.:\\sCKR.+")
	sessionCacheSize = 10
)

type impl struct {
	bccsp.BCCSP

	slot       uint
	pin        string
	ctx        *pkcs11.Ctx
	conf       *config
	softVerify bool
	immutable  bool

	getKeyIDForSKI func(ski []byte) []byte

	sessLock sync.Mutex
	sessPool chan pkcs11.SessionHandle
	sessions map[pkcs11.SessionHandle]struct{}

	cacheLock   sync.RWMutex
	handleCache map[string]pkcs11.ObjectHandle
	keyCache    map[string]bccsp.Key
}

// An Option is used to configure the Provider.
type Option func(p *impl) error

// WithKeyMapper returns an option that configures the Provider to use the
// provided function to map a subject key identifier to a cryptoki CKA_ID
// identifer.
func WithKeyMapper(mapper func([]byte) []byte) Option {
	return func(i *impl) error {
		i.getKeyIDForSKI = mapper
		return nil
	}
}

// New returns a new instance of a BCCSP that uses PKCS#11 standard interfaces
// to generate and use elliptic curve key pairs for signing and verification using
// curves that satisfy the requested security level from opts.
//
// All other cryptographic functions are delegated to a software based BCCSP
// implementation that is configured to use the security level and hashing
// familly from opts and the key store that is provided.
func New(opts PKCS11Opts, keyStore bccsp.KeyStore, options ...Option) (bccsp.BCCSP, error) {
	// Init config
	conf := &config{}
	err := conf.setSecurityLevel(opts.SecLevel, opts.HashFamily)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed initializing configuration")
	}

	swCSP, err := sw.NewWithParams(opts.SecLevel, opts.HashFamily, keyStore)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed initializing fallback SW BCCSP")
	}

	var sessPool chan pkcs11.SessionHandle
	if sessionCacheSize > 0 {
		sessPool = make(chan pkcs11.SessionHandle, sessionCacheSize)
	}

	csp := &impl{
		BCCSP:          swCSP,
		conf:           conf,
		getKeyIDForSKI: func(ski []byte) []byte { return ski },
		sessPool:       sessPool,
		sessions:       map[pkcs11.SessionHandle]struct{}{},
		handleCache:    map[string]pkcs11.ObjectHandle{},
		softVerify:     opts.SoftVerify,
		keyCache:       map[string]bccsp.Key{},
		immutable:      opts.Immutable,
	}

	for _, o := range options {
		if err := o(csp); err != nil {
			return nil, err
		}
	}

	return csp.initialize(opts)
}

func (csp *impl) initialize(opts PKCS11Opts) (*impl, error) {
	if opts.Library == "" {
		return nil, fmt.Errorf("pkcs11: library path not provided")
	}

	ctx := pkcs11.New(opts.Library)
	if ctx == nil {
		return nil, fmt.Errorf("pkcs11: instantiation failed for %s", opts.Library)
	}
	if err := ctx.Initialize(); err != nil {
		logger.Debugf("initialize failed: %v", err)
	}

	csp.ctx = ctx
	csp.pin = opts.Pin

	slots, err := ctx.GetSlotList(true)
	if err != nil {
		return nil, errors.Wrap(err, "pkcs11: get slot list")
	}

	for _, s := range slots {
		info, err := ctx.GetTokenInfo(s)
		if err != nil || opts.Label != info.Label {
			continue
		}

		csp.slot = s

		session, err := csp.createSession()
		if err != nil {
			return nil, err
		}

		csp.returnSession(session)
		return csp, nil
	}

	return nil, errors.Errorf("pkcs11: could not find token with label %s", opts.Label)
}

// KeyGen generates a key using opts.
func (csp *impl) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil")
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
			return nil, errors.Wrapf(err, "Failed generating ECDSA P384 key")
		}

		k = &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, pub}}

	default:
		return csp.BCCSP.KeyGen(opts)
	}

	return k, nil
}

// KeyImport imports a key from its raw representation using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if raw == nil {
		return nil, errors.New("Invalid raw. Cannot be nil")
	}

	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil")
	}

	switch opts.(type) {

	case *bccsp.X509PublicKeyImportOpts:
		x509Cert, ok := raw.(*x509.Certificate)
		if !ok {
			return nil, errors.New("[X509PublicKeyImportOpts] Invalid raw material. Expected *x509.Certificate")
		}

		pk := x509Cert.PublicKey

		switch pk.(type) {
		case *ecdsa.PublicKey:
			return csp.KeyImport(pk, &bccsp.ECDSAGoPublicKeyImportOpts{Temporary: opts.Ephemeral()})
		default:
			return nil, errors.New("Certificate's public key type not recognized. Supported keys: [ECDSA]")
		}

	default:
		return csp.BCCSP.KeyImport(raw, opts)

	}
}

func (csp *impl) cacheKey(ski []byte, key bccsp.Key) {
	csp.cacheLock.Lock()
	csp.keyCache[hex.EncodeToString(ski)] = key
	csp.cacheLock.Unlock()
}

func (csp *impl) cachedKey(ski []byte) (bccsp.Key, bool) {
	csp.cacheLock.RLock()
	defer csp.cacheLock.RUnlock()
	key, ok := csp.keyCache[hex.EncodeToString(ski)]
	return key, ok
}

// GetKey returns the key this CSP associates to
// the Subject Key Identifier ski.
func (csp *impl) GetKey(ski []byte) (bccsp.Key, error) {
	if key, ok := csp.cachedKey(ski); ok {
		return key, nil
	}

	pubKey, isPriv, err := csp.getECKey(ski)
	if err != nil {
		logger.Debugf("Key not found using PKCS11: %v", err)
		return csp.BCCSP.GetKey(ski)
	}

	var key bccsp.Key = &ecdsaPublicKey{ski, pubKey}
	if isPriv {
		key = &ecdsaPrivateKey{ski, ecdsaPublicKey{ski, pubKey}}
	}

	csp.cacheKey(ski, key)
	return key, nil
}

// Sign signs digest using key k.
// The opts argument should be appropriate for the primitive used.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest).
func (csp *impl) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	// Validate arguments
	if k == nil {
		return nil, errors.New("Invalid Key. It must not be nil")
	}
	if len(digest) == 0 {
		return nil, errors.New("Invalid digest. Cannot be empty")
	}

	// Check key type
	switch key := k.(type) {
	case *ecdsaPrivateKey:
		return csp.signECDSA(*key, digest, opts)
	default:
		return csp.BCCSP.Sign(key, digest, opts)
	}
}

// Verify verifies signature against key k and digest
func (csp *impl) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	// Validate arguments
	if k == nil {
		return false, errors.New("Invalid Key. It must not be nil")
	}
	if len(signature) == 0 {
		return false, errors.New("Invalid signature. Cannot be empty")
	}
	if len(digest) == 0 {
		return false, errors.New("Invalid digest. Cannot be empty")
	}

	// Check key type
	switch key := k.(type) {
	case *ecdsaPrivateKey:
		return csp.verifyECDSA(key.pub, signature, digest, opts)
	case *ecdsaPublicKey:
		return csp.verifyECDSA(*key, signature, digest, opts)
	default:
		return csp.BCCSP.Verify(k, signature, digest, opts)
	}
}

// Encrypt encrypts plaintext using key k.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) ([]byte, error) {
	// TODO: Add PKCS11 support for encryption, when fabric starts requiring it
	return csp.BCCSP.Encrypt(k, plaintext, opts)
}

// Decrypt decrypts ciphertext using key k.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) ([]byte, error) {
	return csp.BCCSP.Decrypt(k, ciphertext, opts)
}

func (csp *impl) getSession() (session pkcs11.SessionHandle, err error) {
	for {
		select {
		case session = <-csp.sessPool:
			return
		default:
			// cache is empty (or completely in use), create a new session
			return csp.createSession()
		}
	}
}

func (csp *impl) createSession() (pkcs11.SessionHandle, error) {
	var sess pkcs11.SessionHandle
	var err error

	// attempt to open a session with a 100ms delay after each attempt
	for i := 0; i < createSessionRetries; i++ {
		sess, err = csp.ctx.OpenSession(csp.slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
		if err == nil {
			logger.Debugf("Created new pkcs11 session %d on slot %d\n", sess, csp.slot)
			break
		}

		logger.Warningf("OpenSession failed, retrying [%s]\n", err)
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return 0, errors.Wrap(err, "OpenSession failed")
	}

	err = csp.ctx.Login(sess, pkcs11.CKU_USER, csp.pin)
	if err != nil && err != pkcs11.Error(pkcs11.CKR_USER_ALREADY_LOGGED_IN) {
		csp.ctx.CloseSession(sess)
		return 0, errors.Wrap(err, "Login failed")
	}

	csp.sessLock.Lock()
	csp.sessions[sess] = struct{}{}
	csp.sessLock.Unlock()

	return sess, nil
}

func (csp *impl) closeSession(session pkcs11.SessionHandle) {
	if err := csp.ctx.CloseSession(session); err != nil {
		logger.Debug("CloseSession failed", err)
	}

	csp.sessLock.Lock()
	defer csp.sessLock.Unlock()

	// purge the handle cache if the last session closes
	delete(csp.sessions, session)
	if len(csp.sessions) == 0 {
		csp.clearCaches()
	}
}

func (csp *impl) returnSession(session pkcs11.SessionHandle) {
	select {
	case csp.sessPool <- session:
		// returned session back to session cache
	default:
		// have plenty of sessions in cache, dropping
		csp.closeSession(session)
	}
}

// Look for an EC key by SKI, stored in CKA_ID
func (csp *impl) getECKey(ski []byte) (pubKey *ecdsa.PublicKey, isPriv bool, err error) {
	session, err := csp.getSession()
	if err != nil {
		return nil, false, err
	}
	defer func() { csp.handleSessionReturn(err, session) }()

	isPriv = true
	_, err = csp.findKeyPairFromSKI(session, ski, privateKeyType)
	if err != nil {
		isPriv = false
		logger.Debugf("Private key not found [%s] for SKI [%s], looking for Public key", err, hex.EncodeToString(ski))
	}

	publicKey, err := csp.findKeyPairFromSKI(session, ski, publicKeyType)
	if err != nil {
		return nil, false, fmt.Errorf("Public key not found [%s] for SKI [%s]", err, hex.EncodeToString(ski))
	}

	ecpt, marshaledOid, err := csp.ecPoint(session, publicKey)
	if err != nil {
		return nil, false, fmt.Errorf("Public key not found [%s] for SKI [%s]", err, hex.EncodeToString(ski))
	}

	curveOid := new(asn1.ObjectIdentifier)
	_, err = asn1.Unmarshal(marshaledOid, curveOid)
	if err != nil {
		return nil, false, fmt.Errorf("Failed Unmarshaling Curve OID [%s]\n%s", err.Error(), hex.EncodeToString(marshaledOid))
	}

	curve := namedCurveFromOID(*curveOid)
	if curve == nil {
		return nil, false, fmt.Errorf("Cound not recognize Curve from OID")
	}
	x, y := elliptic.Unmarshal(curve, ecpt)
	if x == nil {
		return nil, false, fmt.Errorf("Failed Unmarshaling Public Key")
	}

	pubKey = &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
	return pubKey, isPriv, nil
}

// RFC 5480, 2.1.1.1. Named Curve
//
// secp224r1 OBJECT IDENTIFIER ::= {
//   iso(1) identified-organization(3) certicom(132) curve(0) 33 }
//
// secp256r1 OBJECT IDENTIFIER ::= {
//   iso(1) member-body(2) us(840) ansi-X9-62(10045) curves(3)
//   prime(1) 7 }
//
// secp384r1 OBJECT IDENTIFIER ::= {
//   iso(1) identified-organization(3) certicom(132) curve(0) 34 }
//
// secp521r1 OBJECT IDENTIFIER ::= {
//   iso(1) identified-organization(3) certicom(132) curve(0) 35 }
//
var (
	oidNamedCurveP224 = asn1.ObjectIdentifier{1, 3, 132, 0, 33}
	oidNamedCurveP256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7}
	oidNamedCurveP384 = asn1.ObjectIdentifier{1, 3, 132, 0, 34}
	oidNamedCurveP521 = asn1.ObjectIdentifier{1, 3, 132, 0, 35}
)

func namedCurveFromOID(oid asn1.ObjectIdentifier) elliptic.Curve {
	switch {
	case oid.Equal(oidNamedCurveP224):
		return elliptic.P224()
	case oid.Equal(oidNamedCurveP256):
		return elliptic.P256()
	case oid.Equal(oidNamedCurveP384):
		return elliptic.P384()
	case oid.Equal(oidNamedCurveP521):
		return elliptic.P521()
	}
	return nil
}

func (csp *impl) generateECKey(curve asn1.ObjectIdentifier, ephemeral bool) (ski []byte, pubKey *ecdsa.PublicKey, err error) {
	session, err := csp.getSession()
	if err != nil {
		return nil, nil, err
	}
	defer func() { csp.handleSessionReturn(err, session) }()

	id := nextIDCtr()
	publabel := fmt.Sprintf("BCPUB%s", id.Text(16))
	prvlabel := fmt.Sprintf("BCPRV%s", id.Text(16))

	marshaledOID, err := asn1.Marshal(curve)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not marshal OID [%s]", err.Error())
	}

	pubkeyT := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, !ephemeral),
		pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, marshaledOID),

		pkcs11.NewAttribute(pkcs11.CKA_ID, publabel),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, publabel),
	}

	prvkeyT := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, !ephemeral),
		pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, true),
		pkcs11.NewAttribute(pkcs11.CKA_SIGN, true),

		pkcs11.NewAttribute(pkcs11.CKA_ID, prvlabel),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, prvlabel),

		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false),
		pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true),
	}

	pub, prv, err := csp.ctx.GenerateKeyPair(session,
		[]*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_EC_KEY_PAIR_GEN, nil)},
		pubkeyT,
		prvkeyT,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: keypair generate failed [%s]", err)
	}

	ecpt, _, err := csp.ecPoint(session, pub)
	if err != nil {
		return nil, nil, fmt.Errorf("Error querying EC-point: [%s]", err)
	}
	hash := sha256.Sum256(ecpt)
	ski = hash[:]

	// set CKA_ID of the both keys to SKI(public key) and CKA_LABEL to hex string of SKI
	setskiT := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, hex.EncodeToString(ski)),
	}

	logger.Infof("Generated new P11 key, SKI %x\n", ski)
	err = csp.ctx.SetAttributeValue(session, pub, setskiT)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: set-ID-to-SKI[public] failed [%s]", err)
	}

	err = csp.ctx.SetAttributeValue(session, prv, setskiT)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: set-ID-to-SKI[private] failed [%s]", err)
	}

	//Set CKA_Modifible to false for both public key and private keys
	if csp.immutable {
		setCKAModifiable := []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_MODIFIABLE, false),
		}

		_, pubCopyerror := csp.ctx.CopyObject(session, pub, setCKAModifiable)
		if pubCopyerror != nil {
			return nil, nil, fmt.Errorf("P11: Public Key copy failed with error [%s] . Please contact your HSM vendor", pubCopyerror)
		}

		pubKeyDestroyError := csp.ctx.DestroyObject(session, pub)
		if pubKeyDestroyError != nil {
			return nil, nil, fmt.Errorf("P11: Public Key destroy failed with error [%s]. Please contact your HSM vendor", pubCopyerror)
		}

		_, prvCopyerror := csp.ctx.CopyObject(session, prv, setCKAModifiable)
		if prvCopyerror != nil {
			return nil, nil, fmt.Errorf("P11: Private Key copy failed with error [%s]. Please contact your HSM vendor", prvCopyerror)
		}
		prvKeyDestroyError := csp.ctx.DestroyObject(session, prv)
		if prvKeyDestroyError != nil {
			return nil, nil, fmt.Errorf("P11: Private Key destroy failed with error [%s]. Please contact your HSM vendor", prvKeyDestroyError)
		}
	}

	nistCurve := namedCurveFromOID(curve)
	if curve == nil {
		return nil, nil, fmt.Errorf("Cound not recognize Curve from OID")
	}
	x, y := elliptic.Unmarshal(nistCurve, ecpt)
	if x == nil {
		return nil, nil, fmt.Errorf("Failed Unmarshaling Public Key")
	}

	pubGoKey := &ecdsa.PublicKey{Curve: nistCurve, X: x, Y: y}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		listAttrs(csp.ctx, session, prv)
		listAttrs(csp.ctx, session, pub)
	}

	return ski, pubGoKey, nil
}

func (csp *impl) signP11ECDSA(ski []byte, msg []byte) (R, S *big.Int, err error) {
	session, err := csp.getSession()
	if err != nil {
		return nil, nil, err
	}
	defer func() { csp.handleSessionReturn(err, session) }()

	privateKey, err := csp.findKeyPairFromSKI(session, ski, privateKeyType)
	if err != nil {
		return nil, nil, fmt.Errorf("Private key not found [%s]", err)
	}

	err = csp.ctx.SignInit(session, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil)}, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Sign-initialize  failed [%s]", err)
	}

	var sig []byte

	sig, err = csp.ctx.Sign(session, msg)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: sign failed [%s]", err)
	}

	R = new(big.Int)
	S = new(big.Int)
	R.SetBytes(sig[0 : len(sig)/2])
	S.SetBytes(sig[len(sig)/2:])

	return R, S, nil
}

func (csp *impl) verifyP11ECDSA(ski []byte, msg []byte, R, S *big.Int, byteSize int) (bool, error) {
	session, err := csp.getSession()
	if err != nil {
		return false, err
	}
	defer func() { csp.handleSessionReturn(err, session) }()

	logger.Debugf("Verify ECDSA\n")

	publicKey, err := csp.findKeyPairFromSKI(session, ski, publicKeyType)
	if err != nil {
		return false, fmt.Errorf("Public key not found [%s]", err)
	}

	r := R.Bytes()
	s := S.Bytes()

	// Pad front of R and S with Zeroes if needed
	sig := make([]byte, 2*byteSize)
	copy(sig[byteSize-len(r):byteSize], r)
	copy(sig[2*byteSize-len(s):], s)

	err = csp.ctx.VerifyInit(
		session,
		[]*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil)},
		publicKey,
	)
	if err != nil {
		return false, fmt.Errorf("PKCS11: Verify-initialize [%s]", err)
	}
	err = csp.ctx.Verify(session, msg, sig)
	if err == pkcs11.Error(pkcs11.CKR_SIGNATURE_INVALID) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("PKCS11: Verify failed [%s]", err)
	}

	return true, nil
}

type keyType int8

const (
	publicKeyType keyType = iota
	privateKeyType
)

func (csp *impl) cachedHandle(keyType keyType, ski []byte) (pkcs11.ObjectHandle, bool) {
	cacheKey := hex.EncodeToString(append([]byte{byte(keyType)}, ski...))
	csp.cacheLock.RLock()
	defer csp.cacheLock.RUnlock()

	handle, ok := csp.handleCache[cacheKey]
	return handle, ok
}

func (csp *impl) cacheHandle(keyType keyType, ski []byte, handle pkcs11.ObjectHandle) {
	cacheKey := hex.EncodeToString(append([]byte{byte(keyType)}, ski...))
	csp.cacheLock.Lock()
	defer csp.cacheLock.Unlock()

	csp.handleCache[cacheKey] = handle
}

func (csp *impl) clearCaches() {
	csp.cacheLock.Lock()
	defer csp.cacheLock.Unlock()
	csp.handleCache = map[string]pkcs11.ObjectHandle{}
	csp.keyCache = map[string]bccsp.Key{}
}

func (csp *impl) findKeyPairFromSKI(session pkcs11.SessionHandle, ski []byte, keyType keyType) (pkcs11.ObjectHandle, error) {
	// check for cached handle
	if handle, ok := csp.cachedHandle(keyType, ski); ok {
		return handle, nil
	}

	ktype := pkcs11.CKO_PUBLIC_KEY
	if keyType == privateKeyType {
		ktype = pkcs11.CKO_PRIVATE_KEY
	}

	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, ktype),
		pkcs11.NewAttribute(pkcs11.CKA_ID, csp.getKeyIDForSKI(ski)),
	}
	if err := csp.ctx.FindObjectsInit(session, template); err != nil {
		return 0, err
	}
	defer csp.ctx.FindObjectsFinal(session)

	// single session instance, assume one hit only
	objs, _, err := csp.ctx.FindObjects(session, 1)
	if err != nil {
		return 0, err
	}

	if len(objs) == 0 {
		return 0, fmt.Errorf("Key not found [%s]", hex.Dump(ski))
	}

	// cache the found handle
	csp.cacheHandle(keyType, ski, objs[0])

	return objs[0], nil
}

// Fairly straightforward EC-point query, other than opencryptoki
// mis-reporting length, including the 04 Tag of the field following
// the SPKI in EP11-returned MACed publickeys:
//
// attr type 385/x181, length 66 b  -- SHOULD be 1+64
// EC point:
// 00000000  04 ce 30 31 6d 5a fd d3  53 2d 54 9a 27 54 d8 7c
// 00000010  d9 80 35 91 09 2d 6f 06  5a 8e e3 cb c0 01 b7 c9
// 00000020  13 5d 70 d4 e5 62 f2 1b  10 93 f7 d5 77 41 ba 9d
// 00000030  93 3e 18 3e 00 c6 0a 0e  d2 36 cc 7f be 50 16 ef
// 00000040  06 04
//
// cf. correct field:
//   0  89: SEQUENCE {
//   2  19:   SEQUENCE {
//   4   7:     OBJECT IDENTIFIER ecPublicKey (1 2 840 10045 2 1)
//  13   8:     OBJECT IDENTIFIER prime256v1 (1 2 840 10045 3 1 7)
//        :     }
//  23  66:   BIT STRING
//        :     04 CE 30 31 6D 5A FD D3 53 2D 54 9A 27 54 D8 7C
//        :     D9 80 35 91 09 2D 6F 06 5A 8E E3 CB C0 01 B7 C9
//        :     13 5D 70 D4 E5 62 F2 1B 10 93 F7 D5 77 41 BA 9D
//        :     93 3E 18 3E 00 C6 0A 0E D2 36 CC 7F BE 50 16 EF
//        :     06
//        :   }
//
// as a short-term workaround, remove the trailing byte if:
//   - receiving an even number of bytes == 2*prime-coordinate +2 bytes
//   - starting byte is 04: uncompressed EC point
//   - trailing byte is 04: assume it belongs to the next OCTET STRING
//
// [mis-parsing encountered with v3.5.1, 2016-10-22]
//
// SoftHSM reports extra two bytes before the uncompressed point
// 0x04 || <Length*2+1>
//                 VV< Actual start of point
// 00000000  04 41 04 6c c8 57 32 13  02 12 6a 19 23 1d 5a 64  |.A.l.W2...j.#.Zd|
// 00000010  33 0c eb 75 4d e8 99 22  92 35 96 b2 39 58 14 1e  |3..uM..".5..9X..|
// 00000020  19 de ef 32 46 50 68 02  24 62 36 db ed b1 84 7b  |...2FPh.$b6....{|
// 00000030  93 d8 40 c3 d5 a6 b7 38  16 d2 35 0a 53 11 f9 51  |..@....8..5.S..Q|
// 00000040  fc a7 16                                          |...|
func (csp *impl) ecPoint(session pkcs11.SessionHandle, key pkcs11.ObjectHandle) (ecpt, oid []byte, err error) {
	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, nil),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, nil),
	}

	attr, err := csp.ctx.GetAttributeValue(session, key, template)
	if err != nil {
		return nil, nil, fmt.Errorf("PKCS11: get(EC point) [%s]", err)
	}

	for _, a := range attr {
		if a.Type == pkcs11.CKA_EC_POINT {
			logger.Debugf("EC point: attr type %d/0x%x, len %d\n%s\n", a.Type, a.Type, len(a.Value), hex.Dump(a.Value))

			// workarounds, see above
			if ((len(a.Value) % 2) == 0) &&
				(byte(0x04) == a.Value[0]) &&
				(byte(0x04) == a.Value[len(a.Value)-1]) {
				logger.Debugf("Detected opencryptoki bug, trimming trailing 0x04")
				ecpt = a.Value[0 : len(a.Value)-1] // Trim trailing 0x04
			} else if byte(0x04) == a.Value[0] && byte(0x04) == a.Value[2] {
				logger.Debugf("Detected SoftHSM bug, trimming leading 0x04 0xXX")
				ecpt = a.Value[2:len(a.Value)]
			} else {
				ecpt = a.Value
			}
		} else if a.Type == pkcs11.CKA_EC_PARAMS {
			logger.Debugf("EC point: attr type %d/0x%x, len %d\n%s\n", a.Type, a.Type, len(a.Value), hex.Dump(a.Value))

			oid = a.Value
		}
	}
	if oid == nil || ecpt == nil {
		return nil, nil, fmt.Errorf("CKA_EC_POINT not found, perhaps not an EC Key?")
	}

	return ecpt, oid, nil
}

func (csp *impl) handleSessionReturn(err error, session pkcs11.SessionHandle) {
	if err != nil {
		if regex.MatchString(err.Error()) {
			logger.Infof("PKCS11 session invalidated, closing session: %v", err)
			csp.closeSession(session)
			return
		}
	}
	csp.returnSession(session)
}

func listAttrs(p11lib *pkcs11.Ctx, session pkcs11.SessionHandle, obj pkcs11.ObjectHandle) {
	var cktype, ckclass uint
	var ckaid, cklabel []byte

	if p11lib == nil {
		return
	}

	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, ckclass),
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, cktype),
		pkcs11.NewAttribute(pkcs11.CKA_ID, ckaid),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, cklabel),
	}

	// certain errors are tolerated, if value is missing
	attr, err := p11lib.GetAttributeValue(session, obj, template)
	if err != nil {
		logger.Debugf("P11: get(attrlist) [%s]\n", err)
	}

	for _, a := range attr {
		// Would be friendlier if the bindings provided a way convert Attribute hex to string
		logger.Debugf("ListAttr: type %d/0x%x, length %d\n%s", a.Type, a.Type, len(a.Value), hex.Dump(a.Value))
	}
}

var (
	bigone  = new(big.Int).SetInt64(1)
	idCtr   = new(big.Int)
	idMutex sync.Mutex
)

func nextIDCtr() *big.Int {
	idMutex.Lock()
	idCtr = new(big.Int).Add(idCtr, bigone)
	idMutex.Unlock()
	return idCtr
}

// FindPKCS11Lib IS ONLY USED FOR TESTING
// This is a convenience function. Useful to self-configure, for tests where
// usual configuration is not available.
func FindPKCS11Lib() (lib, pin, label string) {
	if lib = os.Getenv("PKCS11_LIB"); lib == "" {
		possibilities := []string{
			"/usr/lib/softhsm/libsofthsm2.so",                  //Debian
			"/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so", //Ubuntu
		}
		for _, path := range possibilities {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				lib = path
				break
			}
		}
	}
	if pin = os.Getenv("PKCS11_PIN"); pin == "" {
		pin = "98765432"
	}
	if label = os.Getenv("PKCS11_LABEL"); label == "" {
		label = "ForFabric"
	}

	return lib, pin, label
}
