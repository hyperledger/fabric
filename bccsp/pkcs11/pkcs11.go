/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"

	"github.com/miekg/pkcs11"
	"github.com/op/go-logging"
)

func loadLib(lib, pin, label string) (*pkcs11.Ctx, uint, *pkcs11.SessionHandle, error) {
	var slot uint = 0
	logger.Debugf("Loading pkcs11 library [%s]\n", lib)
	if lib == "" {
		return nil, slot, nil, fmt.Errorf("No PKCS11 library default")
	}

	ctx := pkcs11.New(lib)
	if ctx == nil {
		return nil, slot, nil, fmt.Errorf("Instantiate failed [%s]", lib)
	}

	ctx.Initialize()
	slots, err := ctx.GetSlotList(true)
	if err != nil {
		return nil, slot, nil, fmt.Errorf("Could not get Slot List [%s]", err)
	}
	found := false
	for _, s := range slots {
		info, err := ctx.GetTokenInfo(s)
		if err != nil {
			continue
		}
		logger.Debugf("Looking for %s, found label %s\n", label, info.Label)
		if label == info.Label {
			found = true
			slot = s
			break
		}
	}
	if !found {
		return nil, slot, nil, fmt.Errorf("Could not find token with label %s", label)
	}

	var session pkcs11.SessionHandle
	for i := 0; i < 10; i++ {
		session, err = ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
		if err != nil {
			logger.Warningf("OpenSession failed, retrying [%s]\n", err)
		} else {
			break
		}
	}
	if err != nil {
		logger.Fatalf("OpenSession [%s]\n", err)
	}
	logger.Debugf("Created new pkcs11 session %+v on slot %d\n", session, slot)

	if pin == "" {
		return nil, slot, nil, fmt.Errorf("No PIN set\n")
	}
	err = ctx.Login(session, pkcs11.CKU_USER, pin)
	if err != nil {
		if err != pkcs11.Error(pkcs11.CKR_USER_ALREADY_LOGGED_IN) {
			return nil, slot, nil, fmt.Errorf("Login failed [%s]\n", err)
		}
	}

	return ctx, slot, &session, nil
}

func (csp *impl) getSession() (session pkcs11.SessionHandle) {
	select {
	case session = <-csp.sessions:
		logger.Debugf("Reusing existing pkcs11 session %+v on slot %d\n", session, csp.slot)

	default:
		// cache is empty (or completely in use), create a new session
		var s pkcs11.SessionHandle
		var err error = nil
		for i := 0; i < 10; i++ {
			s, err = csp.ctx.OpenSession(csp.slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
			if err != nil {
				logger.Warningf("OpenSession failed, retrying [%s]\n", err)
			} else {
				break
			}
		}
		if err != nil {
			panic(fmt.Errorf("OpenSession failed [%s]\n", err))
		}
		logger.Debugf("Created new pkcs11 session %+v on slot %d\n", s, csp.slot)
		session = s
	}
	return session
}

func (csp *impl) returnSession(session pkcs11.SessionHandle) {
	select {
	case csp.sessions <- session:
		// returned session back to session cache
	default:
		// have plenty of sessions in cache, dropping
		csp.ctx.CloseSession(session)
	}
}

// Look for an EC key by SKI, stored in CKA_ID
// This function can probably be adapted for both EC and RSA keys.
func (csp *impl) getECKey(ski []byte) (pubKey *ecdsa.PublicKey, isPriv bool, err error) {
	p11lib := csp.ctx
	session := csp.getSession()
	defer csp.returnSession(session)
	isPriv = true
	_, err = findKeyPairFromSKI(p11lib, session, ski, privateKeyFlag)
	if err != nil {
		isPriv = false
		logger.Debugf("Private key not found [%s] for SKI [%s], looking for Public key", err, hex.EncodeToString(ski))
	}

	publicKey, err := findKeyPairFromSKI(p11lib, session, ski, publicKeyFlag)
	if err != nil {
		return nil, false, fmt.Errorf("Public key not found [%s] for SKI [%s]", err, hex.EncodeToString(ski))
	}

	ecpt, marshaledOid, err := ecPoint(p11lib, session, *publicKey)
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

func oidFromNamedCurve(curve elliptic.Curve) (asn1.ObjectIdentifier, bool) {
	switch curve {
	case elliptic.P224():
		return oidNamedCurveP224, true
	case elliptic.P256():
		return oidNamedCurveP256, true
	case elliptic.P384():
		return oidNamedCurveP384, true
	case elliptic.P521():
		return oidNamedCurveP521, true
	}

	return nil, false
}

func (csp *impl) generateECKey(curve asn1.ObjectIdentifier, ephemeral bool) (ski []byte, pubKey *ecdsa.PublicKey, err error) {
	p11lib := csp.ctx
	session := csp.getSession()
	defer csp.returnSession(session)

	id := nextIDCtr()
	publabel := fmt.Sprintf("BCPUB%s", id.Text(16))
	prvlabel := fmt.Sprintf("BCPRV%s", id.Text(16))

	marshaledOID, err := asn1.Marshal(curve)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not marshal OID [%s]", err.Error())
	}

	pubkey_t := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, !ephemeral),
		pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, marshaledOID),
		pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, false),

		pkcs11.NewAttribute(pkcs11.CKA_ID, publabel),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, publabel),
	}

	prvkey_t := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, !ephemeral),
		pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, true),
		pkcs11.NewAttribute(pkcs11.CKA_SIGN, true),

		pkcs11.NewAttribute(pkcs11.CKA_ID, prvlabel),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, prvlabel),

		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, !csp.noPrivImport),
	}

	pub, prv, err := p11lib.GenerateKeyPair(session,
		[]*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_EC_KEY_PAIR_GEN, nil)},
		pubkey_t, prvkey_t)

	if err != nil {
		return nil, nil, fmt.Errorf("P11: keypair generate failed [%s]\n", err)
	}

	ecpt, _, _ := ecPoint(p11lib, session, pub)
	hash := sha256.Sum256(ecpt)
	ski = hash[:]

	// set CKA_ID of the both keys to SKI(public key) and CKA_LABEL to hex string of SKI
	setski_t := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, hex.EncodeToString(ski)),
	}

	logger.Infof("Generated new P11 key, SKI %x\n", ski)
	err = p11lib.SetAttributeValue(session, pub, setski_t)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: set-ID-to-SKI[public] failed [%s]\n", err)
	}

	err = p11lib.SetAttributeValue(session, prv, setski_t)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: set-ID-to-SKI[private] failed [%s]\n", err)
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

	if logger.IsEnabledFor(logging.DEBUG) {
		listAttrs(p11lib, session, prv)
		listAttrs(p11lib, session, pub)
	}

	return ski, pubGoKey, nil
}

func (csp *impl) signP11ECDSA(ski []byte, msg []byte) (R, S *big.Int, err error) {
	p11lib := csp.ctx
	session := csp.getSession()
	defer csp.returnSession(session)

	privateKey, err := findKeyPairFromSKI(p11lib, session, ski, privateKeyFlag)
	if err != nil {
		return nil, nil, fmt.Errorf("Private key not found [%s]\n", err)
	}

	err = p11lib.SignInit(session, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil)}, *privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("Sign-initialize  failed [%s]\n", err)
	}

	var sig []byte

	sig, err = p11lib.Sign(session, msg)
	if err != nil {
		return nil, nil, fmt.Errorf("P11: sign failed [%s]\n", err)
	}

	R = new(big.Int)
	S = new(big.Int)
	R.SetBytes(sig[0 : len(sig)/2])
	S.SetBytes(sig[len(sig)/2:])

	return R, S, nil
}

func (csp *impl) verifyP11ECDSA(ski []byte, msg []byte, R, S *big.Int, byteSize int) (valid bool, err error) {
	p11lib := csp.ctx
	session := csp.getSession()
	defer csp.returnSession(session)

	logger.Debugf("Verify ECDSA\n")

	publicKey, err := findKeyPairFromSKI(p11lib, session, ski, publicKeyFlag)
	if err != nil {
		return false, fmt.Errorf("Public key not found [%s]\n", err)
	}

	r := R.Bytes()
	s := S.Bytes()

	// Pad front of R and S with Zeroes if needed
	sig := make([]byte, 2*byteSize)
	copy(sig[byteSize-len(r):byteSize], r)
	copy(sig[2*byteSize-len(s):], s)

	err = p11lib.VerifyInit(session, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil)},
		*publicKey)
	if err != nil {
		return false, fmt.Errorf("PKCS11: Verify-initialize [%s]\n", err)
	}
	err = p11lib.Verify(session, msg, sig)
	if err == pkcs11.Error(pkcs11.CKR_SIGNATURE_INVALID) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("PKCS11: Verify failed [%s]\n", err)
	}

	return true, nil
}

func (csp *impl) importECKey(curve asn1.ObjectIdentifier, privKey, ecPt []byte, ephemeral bool, keyType bool) (ski []byte, err error) {
	p11lib := csp.ctx
	session := csp.getSession()
	defer csp.returnSession(session)

	marshaledOID, err := asn1.Marshal(curve)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal OID [%s]", err.Error())
	}

	var keyTemplate []*pkcs11.Attribute
	if keyType == publicKeyFlag {
		logger.Debug("Importing Public EC Key")

		hash := sha256.Sum256(ecPt)
		ski = hash[:]

		publabel := hex.EncodeToString(ski)

		// Add DER encoding for the CKA_EC_POINT
		ecPt = append([]byte{0x04, byte(len(ecPt))}, ecPt...)

		keyTemplate = []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
			pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY),
			pkcs11.NewAttribute(pkcs11.CKA_TOKEN, !ephemeral),
			pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true),
			pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, marshaledOID),

			pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
			pkcs11.NewAttribute(pkcs11.CKA_LABEL, publabel),
			pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, ecPt),
			pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, false),
		}
	} else { // isPrivateKey
		ski, err = csp.importECKey(curve, nil, ecPt, ephemeral, publicKeyFlag)
		if err != nil {
			return nil, fmt.Errorf("Failed importing private EC Key [%s]\n", err)
		}

		logger.Debugf("Importing Private EC Key [%d]\n%s\n", len(privKey)*8, hex.Dump(privKey))
		prvlabel := hex.EncodeToString(ski)
		keyTemplate = []*pkcs11.Attribute{
			pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_EC),
			pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
			pkcs11.NewAttribute(pkcs11.CKA_TOKEN, !ephemeral),
			pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, false),
			pkcs11.NewAttribute(pkcs11.CKA_SIGN, true),
			pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, marshaledOID),

			pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
			pkcs11.NewAttribute(pkcs11.CKA_LABEL, prvlabel),
			pkcs11.NewAttribute(pkcs11.CKR_ATTRIBUTE_SENSITIVE, false),
			pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, true),
			pkcs11.NewAttribute(pkcs11.CKA_VALUE, privKey),
		}
	}

	keyHandle, err := p11lib.CreateObject(session, keyTemplate)
	if err != nil {
		return nil, fmt.Errorf("P11: keypair generate failed [%s]\n", err)
	}

	if logger.IsEnabledFor(logging.DEBUG) {
		listAttrs(p11lib, session, keyHandle)
	}

	return ski, nil
}

const (
	privateKeyFlag = true
	publicKeyFlag  = false
)

func findKeyPairFromSKI(mod *pkcs11.Ctx, session pkcs11.SessionHandle, ski []byte, keyType bool) (*pkcs11.ObjectHandle, error) {
	ktype := pkcs11.CKO_PUBLIC_KEY
	if keyType == privateKeyFlag {
		ktype = pkcs11.CKO_PRIVATE_KEY
	}

	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, ktype),
		pkcs11.NewAttribute(pkcs11.CKA_ID, ski),
	}
	if err := mod.FindObjectsInit(session, template); err != nil {
		return nil, err
	}

	// single session instance, assume one hit only
	objs, _, err := mod.FindObjects(session, 1)
	if err != nil {
		return nil, err
	}
	if err = mod.FindObjectsFinal(session); err != nil {
		return nil, err
	}

	if len(objs) == 0 {
		return nil, fmt.Errorf("Key not found [%s]", hex.Dump(ski))
	}

	return &objs[0], nil
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
func ecPoint(p11lib *pkcs11.Ctx, session pkcs11.SessionHandle, key pkcs11.ObjectHandle) (ecpt, oid []byte, err error) {
	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, nil),
		pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, nil),
	}

	attr, err := p11lib.GetAttributeValue(session, key, template)
	if err != nil {
		return nil, nil, fmt.Errorf("PKCS11: get(EC point) [%s]\n", err)
	}

	for _, a := range attr {
		if a.Type == pkcs11.CKA_EC_POINT {
			logger.Debugf("EC point: attr type %d/0x%x, len %d\n%s\n", a.Type, a.Type, len(a.Value), hex.Dump(a.Value))

			// workarounds, see above
			if (0 == (len(a.Value) % 2)) &&
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

func (csp *impl) getSecretValue(ski []byte) []byte {
	p11lib := csp.ctx
	session := csp.getSession()
	defer csp.returnSession(session)

	keyHandle, err := findKeyPairFromSKI(p11lib, session, ski, privateKeyFlag)

	var privKey []byte
	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_VALUE, privKey),
	}

	// certain errors are tolerated, if value is missing
	attr, err := p11lib.GetAttributeValue(session, *keyHandle, template)
	if err != nil {
		logger.Warningf("P11: get(attrlist) [%s]\n", err)
	}

	for _, a := range attr {
		// Would be friendlier if the bindings provided a way convert Attribute hex to string
		logger.Debugf("ListAttr: type %d/0x%x, length %d\n%s", a.Type, a.Type, len(a.Value), hex.Dump(a.Value))
		return a.Value
	}
	logger.Warningf("No Key Value found!", err)
	return nil
}

var (
	bigone   = new(big.Int).SetInt64(1)
	id_ctr   = new(big.Int)
	id_mutex sync.Mutex
)

func nextIDCtr() *big.Int {
	id_mutex.Lock()
	id_ctr = new(big.Int).Add(id_ctr, bigone)
	id_mutex.Unlock()
	return id_ctr
}
