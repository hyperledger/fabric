/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var mspIdentityLogger = flogging.MustGetLogger("msp.identity")

type identity struct {
	// id contains the identifier (MSPID and identity identifier) for this instance
	id *IdentityIdentifier

	// cert contains the x.509 certificate that signs the public key of this instance
	cert *x509.Certificate

	// this is the public key of this instance
	pk bccsp.Key

	// reference to the MSP that "owns" this identity
	msp *bccspmsp

	// validationMutex is used to synchronise memory operation
	// over validated and validationErr
	validationMutex sync.Mutex

	// validated is true when the validateIdentity function
	// has been called on this instance
	validated bool

	// validationErr contains the validation error for this
	// instance. It can be read if validated is true
	validationErr error
}

func newIdentity(cert *x509.Certificate, pk bccsp.Key, msp *bccspmsp) (Identity, error) {
	if mspIdentityLogger.IsEnabledFor(zapcore.DebugLevel) {
		mspIdentityLogger.Debugf("Creating identity instance for cert %s", certToPEM(cert))
	}

	// Sanitize first the certificate
	cert, err := msp.sanitizeCert(cert)
	if err != nil {
		return nil, err
	}

	// Compute identity identifier

	// Use the hash of the identity's certificate as id in the IdentityIdentifier
	hashOpt, err := bccsp.GetHashOpt(msp.cryptoConfig.IdentityIdentifierHashFunction)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting hash function options")
	}

	digest, err := msp.bccsp.Hash(cert.Raw, hashOpt)
	if err != nil {
		return nil, errors.WithMessage(err, "failed hashing raw certificate to compute the id of the IdentityIdentifier")
	}

	id := &IdentityIdentifier{
		Mspid: msp.name,
		Id:    hex.EncodeToString(digest),
	}

	return &identity{id: id, cert: cert, pk: pk, msp: msp}, nil
}

// ExpiresAt returns the time at which the Identity expires.
func (id *identity) ExpiresAt() time.Time {
	return id.cert.NotAfter
}

// SatisfiesPrincipal returns nil if this instance matches the supplied principal or an error otherwise
func (id *identity) SatisfiesPrincipal(principal *msp.MSPPrincipal) error {
	return id.msp.SatisfiesPrincipal(id, principal)
}

// GetIdentifier returns the identifier (MSPID/IDID) for this instance
func (id *identity) GetIdentifier() *IdentityIdentifier {
	return id.id
}

// GetMSPIdentifier returns the MSP identifier for this instance
func (id *identity) GetMSPIdentifier() string {
	return id.id.Mspid
}

// Validate returns nil if this instance is a valid identity or an error otherwise
func (id *identity) Validate() error {
	return id.msp.Validate(id)
}

type OUIDs []*OUIdentifier

func (o OUIDs) String() string {
	var res []string
	for _, id := range o {
		res = append(res, fmt.Sprintf("%s(%X)", id.OrganizationalUnitIdentifier, id.CertifiersIdentifier[0:8]))
	}

	return fmt.Sprintf("%s", res)
}

// GetOrganizationalUnits returns the OU for this instance
func (id *identity) GetOrganizationalUnits() []*OUIdentifier {
	if id.cert == nil {
		return nil
	}

	cid, err := id.msp.getCertificationChainIdentifier(id)
	if err != nil {
		mspIdentityLogger.Errorf("Failed getting certification chain identifier for [%v]: [%+v]", id, err)

		return nil
	}

	var res []*OUIdentifier
	for _, unit := range id.cert.Subject.OrganizationalUnit {
		res = append(res, &OUIdentifier{
			OrganizationalUnitIdentifier: unit,
			CertifiersIdentifier:         cid,
		})
	}

	return res
}

// Anonymous returns true if this identity provides anonymity
func (id *identity) Anonymous() bool {
	return false
}

// NewSerializedIdentity returns a serialized identity
// having as content the passed mspID and x509 certificate in PEM format.
// This method does not check the validity of certificate nor
// any consistency of the mspID with it.
func NewSerializedIdentity(mspID string, certPEM []byte) ([]byte, error) {
	// We serialize identities by prepending the MSPID
	// and appending the x509 cert in PEM format
	sId := &msp.SerializedIdentity{Mspid: mspID, IdBytes: certPEM}
	raw, err := proto.Marshal(sId)
	if err != nil {
		return nil, errors.Wrapf(err, "failed serializing identity [%s][%X]", mspID, certPEM)
	}
	return raw, nil
}

// Verify checks against a signature and a message
// to determine whether this identity produced the
// signature; it returns nil if so or an error otherwise
func (id *identity) Verify(msg []byte, sig []byte) error {
	// mspIdentityLogger.Infof("Verifying signature")

	// Compute Hash
	hashOpt, err := id.getHashOpt(id.msp.cryptoConfig.SignatureHashFamily)
	if err != nil {
		return errors.WithMessage(err, "failed getting hash function options")
	}

	digest, err := id.msp.bccsp.Hash(msg, hashOpt)
	if err != nil {
		return errors.WithMessage(err, "failed computing digest")
	}

	if mspIdentityLogger.IsEnabledFor(zapcore.DebugLevel) {
		mspIdentityLogger.Debugf("Verify: signer identity (certificate subject=%s issuer=%s serialnumber=%d)", id.cert.Subject, id.cert.Issuer, id.cert.SerialNumber)
		// mspIdentityLogger.Debugf("Verify: digest = %s", hex.Dump(digest))
		// mspIdentityLogger.Debugf("Verify: sig = %s", hex.Dump(sig))
	}

	valid, err := id.msp.bccsp.Verify(id.pk, sig, digest, nil)
	if err != nil {
		return errors.WithMessage(err, "could not determine the validity of the signature")
	} else if !valid {
		mspIdentityLogger.Warnf("The signature is invalid for (certificate subject=%s issuer=%s serialnumber=%d)", id.cert.Subject, id.cert.Issuer, id.cert.SerialNumber)
		return errors.New("The signature is invalid")
	}

	return nil
}

// Serialize returns a byte array representation of this identity
func (id *identity) Serialize() ([]byte, error) {
	pb := &pem.Block{Bytes: id.cert.Raw, Type: "CERTIFICATE"}
	pemBytes := pem.EncodeToMemory(pb)
	if pemBytes == nil {
		return nil, errors.New("encoding of identity failed")
	}

	// We serialize identities by prepending the MSPID and appending the ASN.1 DER content of the cert
	sId := &msp.SerializedIdentity{Mspid: id.id.Mspid, IdBytes: pemBytes}
	idBytes, err := proto.Marshal(sId)
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal a SerializedIdentity structure for identity %s", id.id)
	}

	return idBytes, nil
}

func (id *identity) getHashOpt(hashFamily string) (bccsp.HashOpts, error) {
	switch hashFamily {
	case bccsp.SHA2:
		return bccsp.GetHashOpt(bccsp.SHA256)
	case bccsp.SHA3:
		return bccsp.GetHashOpt(bccsp.SHA3_256)
	}
	return nil, errors.Errorf("hash family not recognized [%s]", hashFamily)
}

type signingidentity struct {
	// we embed everything from a base identity
	identity

	// signer corresponds to the object that can produce signatures from this identity
	signer crypto.Signer
}

func newSigningIdentity(cert *x509.Certificate, pk bccsp.Key, signer crypto.Signer, msp *bccspmsp) (SigningIdentity, error) {
	// mspIdentityLogger.Infof("Creating signing identity instance for ID %s", id)
	mspId, err := newIdentity(cert, pk, msp)
	if err != nil {
		return nil, err
	}
	return &signingidentity{
		identity: identity{
			id:   mspId.(*identity).id,
			cert: mspId.(*identity).cert,
			msp:  mspId.(*identity).msp,
			pk:   mspId.(*identity).pk,
		},
		signer: signer,
	}, nil
}

// Sign produces a signature over msg, signed by this instance
func (id *signingidentity) Sign(msg []byte) ([]byte, error) {
	// mspIdentityLogger.Infof("Signing message")

	// Compute Hash
	hashOpt, err := id.getHashOpt(id.msp.cryptoConfig.SignatureHashFamily)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting hash function options")
	}

	digest, err := id.msp.bccsp.Hash(msg, hashOpt)
	if err != nil {
		return nil, errors.WithMessage(err, "failed computing digest")
	}

	if len(msg) < 32 {
		mspIdentityLogger.Debugf("Sign: plaintext: %X \n", msg)
	} else {
		mspIdentityLogger.Debugf("Sign: plaintext: %X...%X \n", msg[0:16], msg[len(msg)-16:])
	}
	mspIdentityLogger.Debugf("Sign: digest: %X \n", digest)

	// Sign
	return id.signer.Sign(rand.Reader, digest, nil)
}

// GetPublicVersion returns the public version of this identity,
// namely, the one that is only able to verify messages and not sign them
func (id *signingidentity) GetPublicVersion() Identity {
	return &id.identity
}
