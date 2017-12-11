/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"encoding/hex"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/idemix"
	m "github.com/hyperledger/fabric/protos/msp"
	"github.com/milagro-crypto/amcl/version3/go/amcl"
	"github.com/milagro-crypto/amcl/version3/go/amcl/FP256BN"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

const (
	// AttributeNameOU is the attribute name of the Organization Unit attribute
	AttributeNameOU = "OU"

	// AttributeNameRole is the attribute name of the Role attribute
	AttributeNameRole = "Role"
)

// discloseFlags will be passed to the idemix signing and verification routines.
// It informs idemix to disclose both attributes (OU and Role) when signing.
var discloseFlags = []byte{1, 1}

type idemixmsp struct {
	ipk    *idemix.IssuerPublicKey
	rng    *amcl.RAND
	signer *idemixSigningIdentity
	name   string
}

// newIdemixMsp creates a new instance of idemixmsp
func newIdemixMsp() (MSP, error) {
	mspLogger.Debugf("Creating Idemix-based MSP instance")

	msp := idemixmsp{}
	return &msp, nil
}

func (msp *idemixmsp) Setup(conf1 *m.MSPConfig) error {
	mspLogger.Debugf("Setting up Idemix-based MSP instance")

	if conf1 == nil {
		return errors.Errorf("setup error: nil conf reference")
	}

	if conf1.Type != int32(IDEMIX) {
		return errors.Errorf("setup error: config is not of type IDEMIX")
	}

	var conf m.IdemixMSPConfig
	err := proto.Unmarshal(conf1.Config, &conf)
	if err != nil {
		return errors.Wrap(err, "failed unmarshalling idemix msp config")
	}

	msp.name = conf.Name
	mspLogger.Debugf("Setting up Idemix MSP instance %s", msp.name)

	ipk := new(idemix.IssuerPublicKey)
	err = proto.Unmarshal(conf.IPk, ipk)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal ipk from idemix msp config")
	}
	err = ipk.SetHash()
	if err != nil {
		return errors.WithMessage(err, "setting the hash of the issuer public key failed")
	}

	if len(ipk.AttributeNames) < 2 || ipk.AttributeNames[0] != AttributeNameOU || ipk.AttributeNames[1] != AttributeNameRole {
		return errors.Errorf("ipk must have have attributes OU and Role")
	}

	err = ipk.Check()
	if err != nil {
		return errors.WithMessage(err, "cannot setup idemix msp with invalid public key")
	}
	msp.ipk = ipk

	rng, err := idemix.GetRand()
	if err != nil {
		return errors.Wrap(err, "error initializing PRNG for idemix msp")
	}

	msp.rng = rng

	if conf.Signer == nil {
		// No credential in config, so we don't setup a default signer
		mspLogger.Debug("idemix msp setup as verification only msp (no key material found)")
		return nil
	}

	// A credential is present in the config, so we setup a default signer
	cred := new(idemix.Credential)
	err = proto.Unmarshal(conf.Signer.Cred, cred)
	if err != nil {
		return errors.Wrap(err, "Failed to unmarshal credential from config")
	}

	sk := FP256BN.FromBytes(conf.Signer.Sk)

	Nym, RandNym := idemix.MakeNym(sk, msp.ipk, rng)
	role := &m.MSPRole{
		MspIdentifier: msp.name,
		Role:          m.MSPRole_MEMBER,
	}
	if conf.Signer.IsAdmin {
		role.Role = m.MSPRole_ADMIN
	}

	ou := &m.OrganizationUnit{
		MspIdentifier:                msp.name,
		OrganizationalUnitIdentifier: conf.Signer.OrganizationalUnitIdentifier,
		CertifiersIdentifier:         ipk.Hash,
	}

	// Check if credential contains the right amount of attribute values (Role and OU)
	if len(cred.Attrs) != 2 {
		return errors.Errorf("Credential contains %d attribute values, but expected 2", len(cred.Attrs))
	}

	// Check if credential contains the correct OU attribute value
	ouBytes := []byte(conf.Signer.OrganizationalUnitIdentifier)
	if !bytes.Equal(idemix.BigToBytes(idemix.HashModOrder(ouBytes)), cred.Attrs[0]) {
		return errors.New("Credential does not contain the correct OU attribute value")
	}

	// Check if credential contains the correct OU attribute value
	if !bytes.Equal(idemix.BigToBytes(FP256BN.NewBIGint(int(role.Role))), cred.Attrs[1]) {
		return errors.New("Credential does not contain the correct Role attribute value")
	}

	// Verify that the credential is cryptographically valid
	err = cred.Ver(sk, msp.ipk)
	if err != nil {
		return errors.Wrap(err, "Credential is not cryptographically valid")
	}

	// Create the cryptographic evidence that this identity is valid
	proof, err := idemix.NewSignature(cred, sk, Nym, RandNym, ipk, discloseFlags, nil, rng)
	if err != nil {
		return errors.Wrap(err, "Failed to setup cryptographic proof of identity")
	}

	// Set up default signer
	msp.signer = &idemixSigningIdentity{newIdemixIdentity(msp, Nym, role, ou, proof), rng, cred, sk, RandNym}

	return nil
}

// GetVersion returns the version of this MSP
func (msp *idemixmsp) GetVersion() MSPVersion {
	return MSPv1_1
}

func (msp *idemixmsp) GetType() ProviderType {
	return IDEMIX
}

func (msp *idemixmsp) GetIdentifier() (string, error) {
	return msp.name, nil
}

func (msp *idemixmsp) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	return nil, errors.Errorf("GetSigningIdentity not implemented")
}

func (msp *idemixmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Debugf("Obtaining default idemix signing identity")

	if msp.signer == nil {
		return nil, errors.Errorf("no default signer setup")
	}
	return msp.signer, nil
}

func (msp *idemixmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	sID := &m.SerializedIdentity{}
	err := proto.Unmarshal(serializedID, sID)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdentity")
	}

	if sID.Mspid != msp.name {
		return nil, errors.Errorf("expected MSP ID %s, received %s", msp.name, sID.Mspid)
	}

	return msp.deserializeIdentityInternal(sID.GetIdBytes())
}

func (msp *idemixmsp) deserializeIdentityInternal(serializedID []byte) (Identity, error) {
	mspLogger.Debug("idemixmsp: deserializing identity")
	serialized := new(m.SerializedIdemixIdentity)
	err := proto.Unmarshal(serializedID, serialized)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdemixIdentity")
	}
	if serialized.NymX == nil || serialized.NymY == nil {
		return nil, errors.Errorf("unable to deserialize idemix identity: pseudonym is invalid")
	}
	Nym := FP256BN.NewECPbigs(FP256BN.FromBytes(serialized.NymX), FP256BN.FromBytes(serialized.NymY))

	ou := &m.OrganizationUnit{}
	err = proto.Unmarshal(serialized.OU, ou)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the OU of the identity")
	}
	role := &m.MSPRole{}
	err = proto.Unmarshal(serialized.Role, role)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the role of the identity")
	}

	proof := &idemix.Signature{}
	err = proto.Unmarshal(serialized.Proof, proof)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the proof of the identity")
	}

	return newIdemixIdentity(msp, Nym, role, ou, proof), nil

}

func (msp *idemixmsp) Validate(id Identity) error {
	mspLogger.Infof("Validating identity %s", id)

	var identity *idemixidentity
	switch t := id.(type) {
	case *idemixidentity:
		identity = id.(*idemixidentity)
	case *idemixSigningIdentity:
		identity = id.(*idemixSigningIdentity).idemixidentity
	default:
		return errors.Errorf("identity type %T is not recognized", t)
	}
	if identity.GetMSPIdentifier() != msp.name {
		return errors.Errorf("the supplied identity does not belong to this msp")
	}
	return identity.verifyProof()
}

func (id *idemixidentity) verifyProof() error {
	ouBytes := []byte(id.OU.OrganizationalUnitIdentifier)
	attributeValues := []*FP256BN.BIG{idemix.HashModOrder(ouBytes), FP256BN.NewBIGint(int(id.Role.Role))}

	return id.associationProof.Ver(discloseFlags, id.msp.ipk, nil, attributeValues)
}

func (msp *idemixmsp) SatisfiesPrincipal(id Identity, principal *m.MSPPrincipal) error {
	switch principal.PrincipalClassification {
	// in this case, we have to check whether the
	// identity has a role in the msp - member or admin
	case m.MSPPrincipal_ROLE:
		// Principal contains the msp role
		mspRole := &m.MSPRole{}
		err := proto.Unmarshal(principal.Principal, mspRole)
		if err != nil {
			return errors.Wrap(err, "could not unmarshal MSPRole from principal")
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if mspRole.MspIdentifier != msp.name {
			return errors.Errorf("the identity is a member of a different MSP (expected %s, got %s)", mspRole.MspIdentifier, id.GetMSPIdentifier())
		}

		// check whether this identity is valid
		err = msp.Validate(id)
		if err != nil {
			return errors.Wrap(err, "identity is not valid with respect to this MSP")
		}
		// now we validate the different msp roles
		switch mspRole.Role {
		case m.MSPRole_MEMBER:
			// in the case of member, we simply check
			// whether this identity is valid for the MSP
			mspLogger.Debugf("Checking if identity satisfies MEMBER role for %s", msp.name)
			return nil
		case m.MSPRole_ADMIN:
			mspLogger.Debugf("Checking if identity satisfies ADMIN role for %s", msp.name)
			if id.(*idemixidentity).Role.Role != m.MSPRole_ADMIN {
				return errors.Errorf("user is not an admin")
			}
			return nil
		default:
			return errors.Errorf("invalid MSP role type %d", int32(mspRole.Role))
		}
	// in this case we have to serialize this instance
	// and compare it byte-by-byte with Principal
	case m.MSPPrincipal_IDENTITY:
		mspLogger.Debugf("Checking if identity satisfies IDENTITY principal")
		idBytes, err := id.Serialize()
		if err != nil {
			return errors.Wrap(err, "could not serialize this identity instance")
		}

		rv := bytes.Compare(idBytes, principal.Principal)
		if rv == 0 {
			return nil
		}
		return errors.Errorf("the identities do not match")

	case m.MSPPrincipal_ORGANIZATION_UNIT:
		ou := &m.OrganizationUnit{}
		err := proto.Unmarshal(principal.Principal, ou)
		if err != nil {
			return errors.Wrap(err, "could not unmarshal OU from principal")
		}

		mspLogger.Debugf("Checking if identity is part of OU \"%s\" of mspid \"%s\"", ou.OrganizationalUnitIdentifier, ou.MspIdentifier)

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if ou.MspIdentifier != msp.name {
			return errors.Errorf("the identity is a member of a different MSP (expected %s, got %s)", ou.MspIdentifier, id.GetMSPIdentifier())
		}

		// we then check if the identity is valid with this MSP
		// and fail if it is not
		err = msp.Validate(id)
		if err != nil {
			return err
		}

		if ou.OrganizationalUnitIdentifier != id.(*idemixidentity).OU.OrganizationalUnitIdentifier {
			return errors.Errorf("user is not part of the desired organizational unit")
		}

		return nil
	default:
		return errors.Errorf("invalid principal type %d", int32(principal.PrincipalClassification))
	}
}

// IsWellFormed checks if the given identity can be deserialized into its provider-specific .
// In this MSP implementation, an identity is considered well formed if it contains a
// marshaled SerializedIdemixIdentity protobuf message.
func (id *idemixmsp) IsWellFormed(identity *m.SerializedIdentity) error {
	sId := new(m.SerializedIdemixIdentity)
	err := proto.Unmarshal(identity.IdBytes, sId)
	if err != nil {
		return errors.Wrap(err, "not an idemix identity")
	}
	return nil
}

func (msp *idemixmsp) GetTLSRootCerts() [][]byte {
	// TODO
	return nil
}

func (msp *idemixmsp) GetTLSIntermediateCerts() [][]byte {
	// TODO
	return nil
}

type idemixidentity struct {
	Nym  *FP256BN.ECP
	msp  *idemixmsp
	id   *IdentityIdentifier
	Role *m.MSPRole
	OU   *m.OrganizationUnit
	// associationProof contains cryptographic proof that this identity
	// belongs to the MSP id.msp, i.e., it proves that the pseudonym
	// is constructed from a secret key on which the CA issued a credential.
	associationProof *idemix.Signature
}

func newIdemixIdentity(msp *idemixmsp, nym *FP256BN.ECP, role *m.MSPRole, ou *m.OrganizationUnit, proof *idemix.Signature) *idemixidentity {
	id := &idemixidentity{}
	id.Nym = nym
	id.msp = msp
	id.id = &IdentityIdentifier{Mspid: msp.name, Id: proto.MarshalTextString(idemix.EcpToProto(nym))}
	id.Role = role
	id.OU = ou
	id.associationProof = proof
	return id
}

func (id *idemixidentity) ExpiresAt() time.Time {
	// Idemix MSP currently does not use expiration dates or revocation,
	// so we return the zero time to indicate this.
	return time.Time{}
}

func (id *idemixidentity) GetIdentifier() *IdentityIdentifier {
	return id.id
}

func (id *idemixidentity) GetMSPIdentifier() string {
	mspid, _ := id.msp.GetIdentifier()
	return mspid
}

func (id *idemixidentity) GetOrganizationalUnits() []*OUIdentifier {
	// we use the (serialized) public key of this MSP as the CertifiersIdentifier
	certifiersIdentifier, err := proto.Marshal(id.msp.ipk)
	if err != nil {
		mspIdentityLogger.Errorf("Failed to marshal ipk in GetOrganizationalUnits: %s", err)
		return nil
	}
	return []*OUIdentifier{{certifiersIdentifier, id.OU.OrganizationalUnitIdentifier}}
}

func (id *idemixidentity) Validate() error {
	return id.msp.Validate(id)
}

func (id *idemixidentity) Verify(msg []byte, sig []byte) error {
	if mspLogger.IsEnabledFor(logging.DEBUG) {
		mspIdentityLogger.Debugf("Verify Idemix sig: msg = %s", hex.Dump(msg))
		mspIdentityLogger.Debugf("Verify Idemix sig: sig = %s", hex.Dump(sig))
	}

	signature := new(idemix.NymSignature)
	err := proto.Unmarshal(sig, signature)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling signature")
	}

	return signature.Ver(id.Nym, id.msp.ipk, msg)
}

func (id *idemixidentity) SatisfiesPrincipal(principal *m.MSPPrincipal) error {
	return id.msp.SatisfiesPrincipal(id, principal)
}

func (id *idemixidentity) Serialize() ([]byte, error) {
	serialized := &m.SerializedIdemixIdentity{}
	serialized.NymX = idemix.BigToBytes(id.Nym.GetX())
	serialized.NymY = idemix.BigToBytes(id.Nym.GetY())
	ouBytes, err := proto.Marshal(id.OU)
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal OU of identity %s", id.id)
	}

	roleBytes, err := proto.Marshal(id.Role)
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal role of identity %s", id.id)
	}

	serialized.OU = ouBytes
	serialized.Role = roleBytes

	serialized.Proof, err = proto.Marshal(id.associationProof)
	if err != nil {
		return nil, err
	}

	idemixIDBytes, err := proto.Marshal(serialized)
	if err != nil {
		return nil, err
	}

	sID := &m.SerializedIdentity{Mspid: id.GetMSPIdentifier(), IdBytes: idemixIDBytes}
	idBytes, err := proto.Marshal(sID)
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal a SerializedIdentity structure for identity %s", id.id)
	}

	return idBytes, nil
}

type idemixSigningIdentity struct {
	*idemixidentity
	rng     *amcl.RAND
	Cred    *idemix.Credential
	Sk      *FP256BN.BIG
	RandNym *FP256BN.BIG
}

func (id *idemixSigningIdentity) Sign(msg []byte) ([]byte, error) {
	mspLogger.Debugf("Idemix identity %s is signing", id.GetIdentifier())
	sig, err := idemix.NewNymSignature(id.Sk, id.Nym, id.RandNym, id.msp.ipk, msg, id.rng)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(sig)
}

func (id *idemixSigningIdentity) GetPublicVersion() Identity {
	return id.idemixidentity
}
