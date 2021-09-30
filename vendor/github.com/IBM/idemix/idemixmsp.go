/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	idemix "github.com/IBM/idemix/bccsp"
	"github.com/IBM/idemix/bccsp/keystore"
	bccsp "github.com/IBM/idemix/bccsp/schemes"
	"github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	"github.com/IBM/idemix/common/flogging"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	// AttributeIndexOU contains the index of the OU attribute in the idemix credential attributes
	AttributeIndexOU = iota

	// AttributeIndexRole contains the index of the Role attribute in the idemix credential attributes
	AttributeIndexRole

	// AttributeIndexEnrollmentId contains the index of the Enrollment ID attribute in the idemix credential attributes
	AttributeIndexEnrollmentId

	// AttributeIndexRevocationHandle contains the index of the Revocation Handle attribute in the idemix credential attributes
	AttributeIndexRevocationHandle
)

const (
	// AttributeNameOU is the attribute name of the Organization Unit attribute
	AttributeNameOU = "OU"

	// AttributeNameRole is the attribute name of the Role attribute
	AttributeNameRole = "Role"

	// AttributeNameEnrollmentId is the attribute name of the Enrollment ID attribute
	AttributeNameEnrollmentId = "EnrollmentID"

	// AttributeNameRevocationHandle is the attribute name of the revocation handle attribute
	AttributeNameRevocationHandle = "RevocationHandle"
)

type MSPVersion int

const (
	MSPv1_0 = iota
	MSPv1_1
	MSPv1_3
	MSPv1_4_3
)

// index of the revocation handle attribute in the credential
const rhIndex = 3
const eidIndex = 2

type Idemixmsp struct {
	csp          bccsp.BCCSP
	version      MSPVersion
	ipk          bccsp.Key
	signer       *IdemixSigningIdentity
	name         string
	revocationPK bccsp.Key
	epoch        int
}

var mspLogger = flogging.MustGetLogger("idemix")
var mspIdentityLogger = flogging.MustGetLogger("idemix.identity")

// NewIdemixMsp creates a new instance of idemixmsp
func NewIdemixMsp(version MSPVersion) (MSP, error) {
	mspLogger.Debugf("Creating Idemix-based MSP instance")

	curve := math.Curves[math.FP256BN_AMCL]
	csp, err := idemix.New(&keystore.Dummy{}, curve, &amcl.Fp256bn{C: curve}, true)
	if err != nil {
		panic(fmt.Sprintf("unexpected condition, error received [%s]", err))
	}

	msp := Idemixmsp{csp: csp}
	msp.version = version
	return &msp, nil
}

func (msp *Idemixmsp) Setup(conf1 *m.MSPConfig) error {
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

	// Import Issuer Public Key
	IssuerPublicKey, err := msp.csp.KeyImport(
		conf.Ipk,
		&bccsp.IdemixIssuerPublicKeyImportOpts{
			Temporary: true,
			AttributeNames: []string{
				AttributeNameOU,
				AttributeNameRole,
				AttributeNameEnrollmentId,
				AttributeNameRevocationHandle,
			},
		})
	if err != nil {
		importErr, ok := errors.Cause(err).(*bccsp.IdemixIssuerPublicKeyImporterError)
		if !ok {
			panic("unexpected condition, BCCSP did not return the expected *bccsp.IdemixIssuerPublicKeyImporterError")
		}
		switch importErr.Type {
		case bccsp.IdemixIssuerPublicKeyImporterUnmarshallingError:
			return errors.WithMessage(err, "failed to unmarshal ipk from idemix msp config")
		case bccsp.IdemixIssuerPublicKeyImporterHashError:
			return errors.WithMessage(err, "setting the hash of the issuer public key failed")
		case bccsp.IdemixIssuerPublicKeyImporterValidationError:
			return errors.WithMessage(err, "cannot setup idemix msp with invalid public key")
		case bccsp.IdemixIssuerPublicKeyImporterNumAttributesError:
			fallthrough
		case bccsp.IdemixIssuerPublicKeyImporterAttributeNameError:
			return errors.Errorf("issuer public key must have have attributes OU, Role, EnrollmentId, and RevocationHandle")
		default:
			panic(fmt.Sprintf("unexpected condtion, issuer public key import error not valid, got [%d]", importErr.Type))
		}
	}
	msp.ipk = IssuerPublicKey

	// Import revocation public key
	RevocationPublicKey, err := msp.csp.KeyImport(
		conf.RevocationPk,
		&bccsp.IdemixRevocationPublicKeyImportOpts{Temporary: true},
	)
	if err != nil {
		return errors.WithMessage(err, "failed to import revocation public key")
	}
	msp.revocationPK = RevocationPublicKey

	if conf.Signer == nil {
		// No credential in config, so we don't setup a default signer
		mspLogger.Debug("idemix msp setup as verification only msp (no key material found)")
		return nil
	}

	// A credential is present in the config, so we setup a default signer

	// Import User secret key
	UserKey, err := msp.csp.KeyImport(conf.Signer.Sk, &bccsp.IdemixUserSecretKeyImportOpts{Temporary: true})
	if err != nil {
		return errors.WithMessage(err, "failed importing signer secret key")
	}

	// Derive NymPublicKey
	NymKey, err := msp.csp.KeyDeriv(UserKey, &bccsp.IdemixNymKeyDerivationOpts{Temporary: true, IssuerPK: IssuerPublicKey})
	if err != nil {
		return errors.WithMessage(err, "failed deriving nym")
	}
	NymPublicKey, err := NymKey.PublicKey()
	if err != nil {
		return errors.Wrapf(err, "failed getting public nym key")
	}

	role := &m.MSPRole{
		MspIdentifier: msp.name,
		Role:          m.MSPRole_MEMBER,
	}
	if checkRole(int(conf.Signer.Role), ADMIN) {
		role.Role = m.MSPRole_ADMIN
	}

	ou := &m.OrganizationUnit{
		MspIdentifier:                msp.name,
		OrganizationalUnitIdentifier: conf.Signer.OrganizationalUnitIdentifier,
		CertifiersIdentifier:         IssuerPublicKey.SKI(),
	}

	enrollmentId := conf.Signer.EnrollmentId

	// Verify credential
	valid, err := msp.csp.Verify(
		UserKey,
		conf.Signer.Cred,
		nil,
		&bccsp.IdemixCredentialSignerOpts{
			IssuerPK: IssuerPublicKey,
			Attributes: []bccsp.IdemixAttribute{
				{Type: bccsp.IdemixBytesAttribute, Value: []byte(conf.Signer.OrganizationalUnitIdentifier)},
				{Type: bccsp.IdemixIntAttribute, Value: getIdemixRoleFromMSPRole(role)},
				{Type: bccsp.IdemixBytesAttribute, Value: []byte(enrollmentId)},
				{Type: bccsp.IdemixHiddenAttribute},
			},
		},
	)
	if err != nil || !valid {
		return errors.WithMessage(err, "Credential is not cryptographically valid")
	}

	// Create the cryptographic evidence that this identity is valid
	proof, err := msp.csp.Sign(
		UserKey,
		nil,
		&bccsp.IdemixSignerOpts{
			Credential: conf.Signer.Cred,
			Nym:        NymKey,
			IssuerPK:   IssuerPublicKey,
			Attributes: []bccsp.IdemixAttribute{
				{Type: bccsp.IdemixBytesAttribute},
				{Type: bccsp.IdemixIntAttribute},
				{Type: bccsp.IdemixHiddenAttribute},
				{Type: bccsp.IdemixHiddenAttribute},
			},
			RhIndex:  rhIndex,
			EidIndex: eidIndex,
			CRI:      conf.Signer.CredentialRevocationInformation,
		},
	)
	if err != nil {
		return errors.WithMessage(err, "Failed to setup cryptographic proof of identity")
	}

	// Set up default signer
	msp.signer = &IdemixSigningIdentity{
		Idemixidentity: newIdemixIdentity(msp, NymPublicKey, role, ou, proof),
		Cred:           conf.Signer.Cred,
		UserKey:        UserKey,
		NymKey:         NymKey,
		enrollmentId:   enrollmentId}

	return nil
}

// GetVersion returns the version of this MSP
func (msp *Idemixmsp) GetVersion() MSPVersion {
	return msp.version
}

func (msp *Idemixmsp) GetType() ProviderType {
	return IDEMIX
}

func (msp *Idemixmsp) GetIdentifier() (string, error) {
	return msp.name, nil
}

func (msp *Idemixmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Debugf("Obtaining default idemix signing identity")

	if msp.signer == nil {
		return nil, errors.Errorf("no default signer setup")
	}
	return msp.signer, nil
}

func (msp *Idemixmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	sID := &m.SerializedIdentity{}
	err := proto.Unmarshal(serializedID, sID)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdentity")
	}

	if sID.Mspid != msp.name {
		return nil, errors.Errorf("expected MSP ID %s, received %s", msp.name, sID.Mspid)
	}

	return msp.DeserializeIdentityInternal(sID.GetIdBytes())
}

func (msp *Idemixmsp) DeserializeIdentityInternal(serializedID []byte) (Identity, error) {
	mspLogger.Debug("idemixmsp: deserializing identity")
	serialized := new(m.SerializedIdemixIdentity)
	err := proto.Unmarshal(serializedID, serialized)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdemixIdentity")
	}
	if serialized.NymX == nil || serialized.NymY == nil {
		return nil, errors.Errorf("unable to deserialize idemix identity: pseudonym is invalid")
	}

	// Import NymPublicKey
	var rawNymPublicKey []byte
	rawNymPublicKey = append(rawNymPublicKey, serialized.NymX...)
	rawNymPublicKey = append(rawNymPublicKey, serialized.NymY...)
	NymPublicKey, err := msp.csp.KeyImport(
		rawNymPublicKey,
		&bccsp.IdemixNymPublicKeyImportOpts{Temporary: true},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to import nym public key")
	}

	// OU
	ou := &m.OrganizationUnit{}
	err = proto.Unmarshal(serialized.Ou, ou)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the OU of the identity")
	}

	// Role
	role := &m.MSPRole{}
	err = proto.Unmarshal(serialized.Role, role)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the role of the identity")
	}

	return newIdemixIdentity(msp, NymPublicKey, role, ou, serialized.Proof), nil
}

func (msp *Idemixmsp) Validate(id Identity) error {
	var identity *Idemixidentity
	switch t := id.(type) {
	case *Idemixidentity:
		identity = id.(*Idemixidentity)
	case *IdemixSigningIdentity:
		identity = id.(*IdemixSigningIdentity).Idemixidentity
	default:
		return errors.Errorf("identity type %T is not recognized", t)
	}

	mspLogger.Debugf("Validating identity %+v", identity)
	if identity.GetMSPIdentifier() != msp.name {
		return errors.Errorf("the supplied identity does not belong to this msp")
	}
	return identity.verifyProof()
}

func (id *Idemixidentity) verifyProof() error {
	// Verify signature
	valid, err := id.msp.csp.Verify(
		id.msp.ipk,
		id.associationProof,
		nil,
		&bccsp.IdemixSignerOpts{
			RevocationPublicKey: id.msp.revocationPK,
			Attributes: []bccsp.IdemixAttribute{
				{Type: bccsp.IdemixBytesAttribute, Value: []byte(id.OU.OrganizationalUnitIdentifier)},
				{Type: bccsp.IdemixIntAttribute, Value: getIdemixRoleFromMSPRole(id.Role)},
				{Type: bccsp.IdemixHiddenAttribute},
				{Type: bccsp.IdemixHiddenAttribute},
			},
			RhIndex:  rhIndex,
			EidIndex: eidIndex,
			Epoch:    id.msp.epoch,
		},
	)
	if err == nil && !valid {
		panic("unexpected condition, an error should be returned for an invalid signature")
	}

	return err
}

func (msp *Idemixmsp) SatisfiesPrincipal(id Identity, principal *m.MSPPrincipal) error {
	err := msp.Validate(id)
	if err != nil {
		return errors.Wrap(err, "identity is not valid with respect to this MSP")
	}

	return msp.satisfiesPrincipalValidated(id, principal)
}

// satisfiesPrincipalValidated performs all the tasks of satisfiesPrincipal except the identity validation,
// such that combined principals will not cause multiple expensive identity validations.
func (msp *Idemixmsp) satisfiesPrincipalValidated(id Identity, principal *m.MSPPrincipal) error {
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

		// now we validate the different msp roles
		switch mspRole.Role {
		case m.MSPRole_MEMBER:
			// in the case of member, we simply check
			// whether this identity is valid for the MSP
			mspLogger.Debugf("Checking if identity satisfies MEMBER role for %s", msp.name)
			return nil
		case m.MSPRole_ADMIN:
			mspLogger.Debugf("Checking if identity satisfies ADMIN role for %s", msp.name)
			if id.(*Idemixidentity).Role.Role != m.MSPRole_ADMIN {
				return errors.Errorf("user is not an admin")
			}
			return nil
		case m.MSPRole_PEER:
			if msp.version >= MSPv1_3 {
				return errors.Errorf("idemixmsp only supports client use, so it cannot satisfy an MSPRole PEER principal")
			}
			fallthrough
		case m.MSPRole_CLIENT:
			if msp.version >= MSPv1_3 {
				return nil // any valid idemixmsp member must be a client
			}
			fallthrough
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

		if ou.OrganizationalUnitIdentifier != id.(*Idemixidentity).OU.OrganizationalUnitIdentifier {
			return errors.Errorf("user is not part of the desired organizational unit")
		}

		return nil
	case m.MSPPrincipal_COMBINED:
		if msp.version <= MSPv1_1 {
			return errors.Errorf("Combined MSP Principals are unsupported in MSPv1_1")
		}

		// Principal is a combination of multiple principals.
		principals := &m.CombinedPrincipal{}
		err := proto.Unmarshal(principal.Principal, principals)
		if err != nil {
			return errors.Wrap(err, "could not unmarshal CombinedPrincipal from principal")
		}
		// Return an error if there are no principals in the combined principal.
		if len(principals.Principals) == 0 {
			return errors.New("no principals in CombinedPrincipal")
		}
		// Recursively call msp.SatisfiesPrincipal for all combined principals.
		// There is no limit for the levels of nesting for the combined principals.
		for _, cp := range principals.Principals {
			err = msp.satisfiesPrincipalValidated(id, cp)
			if err != nil {
				return err
			}
		}
		// The identity satisfies all the principals
		return nil
	case m.MSPPrincipal_ANONYMITY:
		if msp.version <= MSPv1_1 {
			return errors.Errorf("Anonymity MSP Principals are unsupported in MSPv1_1")
		}

		anon := &m.MSPIdentityAnonymity{}
		err := proto.Unmarshal(principal.Principal, anon)
		if err != nil {
			return errors.Wrap(err, "could not unmarshal MSPIdentityAnonymity from principal")
		}
		switch anon.AnonymityType {
		case m.MSPIdentityAnonymity_ANONYMOUS:
			return nil
		case m.MSPIdentityAnonymity_NOMINAL:
			return errors.New("principal is nominal, but idemix MSP is anonymous")
		default:
			return errors.Errorf("unknown principal anonymity type: %d", anon.AnonymityType)
		}
	default:
		return errors.Errorf("invalid principal type %d", int32(principal.PrincipalClassification))
	}
}

// IsWellFormed checks if the given identity can be deserialized into its provider-specific .
// In this MSP implementation, an identity is considered well formed if it contains a
// marshaled SerializedIdemixIdentity protobuf message.
func (id *Idemixmsp) IsWellFormed(identity *m.SerializedIdentity) error {
	sId := new(m.SerializedIdemixIdentity)
	err := proto.Unmarshal(identity.IdBytes, sId)
	if err != nil {
		return errors.Wrap(err, "not an idemix identity")
	}
	return nil
}

func (msp *Idemixmsp) GetTLSRootCerts() [][]byte {
	// TODO
	return nil
}

func (msp *Idemixmsp) GetTLSIntermediateCerts() [][]byte {
	// TODO
	return nil
}

type Idemixidentity struct {
	NymPublicKey bccsp.Key
	msp          *Idemixmsp
	id           *IdentityIdentifier
	Role         *m.MSPRole
	OU           *m.OrganizationUnit
	// associationProof contains cryptographic proof that this identity
	// belongs to the MSP id.msp, i.e., it proves that the pseudonym
	// is constructed from a secret key on which the CA issued a credential.
	associationProof []byte
}

func (id *Idemixidentity) Anonymous() bool {
	return true
}

func newIdemixIdentity(msp *Idemixmsp, NymPublicKey bccsp.Key, role *m.MSPRole, ou *m.OrganizationUnit, proof []byte) *Idemixidentity {
	id := &Idemixidentity{}
	id.NymPublicKey = NymPublicKey
	id.msp = msp
	id.Role = role
	id.OU = ou
	id.associationProof = proof

	raw, err := NymPublicKey.Bytes()
	if err != nil {
		panic(fmt.Sprintf("unexpected condition, failed marshalling nym public key [%s]", err))
	}
	id.id = &IdentityIdentifier{
		Mspid: msp.name,
		Id:    bytes.NewBuffer(raw).String(),
	}

	return id
}

func (id *Idemixidentity) ExpiresAt() time.Time {
	// Idemix MSP currently does not use expiration dates or revocation,
	// so we return the zero time to indicate this.
	return time.Time{}
}

func (id *Idemixidentity) GetIdentifier() *IdentityIdentifier {
	return id.id
}

func (id *Idemixidentity) GetMSPIdentifier() string {
	mspid, _ := id.msp.GetIdentifier()
	return mspid
}

func (id *Idemixidentity) GetOrganizationalUnits() []*OUIdentifier {
	// we use the (serialized) public key of this MSP as the CertifiersIdentifier
	certifiersIdentifier, err := id.msp.ipk.Bytes()
	if err != nil {
		mspIdentityLogger.Errorf("Failed to marshal ipk in GetOrganizationalUnits: %s", err)
		return nil
	}

	return []*OUIdentifier{{certifiersIdentifier, id.OU.OrganizationalUnitIdentifier}}
}

func (id *Idemixidentity) Validate() error {
	return id.msp.Validate(id)
}

func (id *Idemixidentity) Verify(msg []byte, sig []byte) error {
	if mspIdentityLogger.IsEnabledFor(zapcore.DebugLevel) {
		mspIdentityLogger.Debugf("Verify Idemix sig: msg = %s", hex.Dump(msg))
		mspIdentityLogger.Debugf("Verify Idemix sig: sig = %s", hex.Dump(sig))
	}

	_, err := id.msp.csp.Verify(
		id.NymPublicKey,
		sig,
		msg,
		&bccsp.IdemixNymSignerOpts{
			IssuerPK: id.msp.ipk,
		},
	)
	return err
}

func (id *Idemixidentity) SatisfiesPrincipal(principal *m.MSPPrincipal) error {
	return id.msp.SatisfiesPrincipal(id, principal)
}

func (id *Idemixidentity) Serialize() ([]byte, error) {
	serialized := &m.SerializedIdemixIdentity{}

	raw, err := id.NymPublicKey.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "could not serialize nym of identity %s", id.id)
	}
	// This is an assumption on how the underlying idemix implementation work.
	// TODO: change this in future version
	serialized.NymX = raw[:len(raw)/2]
	serialized.NymY = raw[len(raw)/2:]
	ouBytes, err := proto.Marshal(id.OU)
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal OU of identity %s", id.id)
	}

	roleBytes, err := proto.Marshal(id.Role)
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal role of identity %s", id.id)
	}

	serialized.Ou = ouBytes
	serialized.Role = roleBytes
	serialized.Proof = id.associationProof

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

type IdemixSigningIdentity struct {
	*Idemixidentity
	Cred         []byte
	UserKey      bccsp.Key
	NymKey       bccsp.Key
	enrollmentId string
}

func (id *IdemixSigningIdentity) Sign(msg []byte) ([]byte, error) {
	mspLogger.Debugf("Idemix identity %s is signing", id.GetIdentifier())

	sig, err := id.msp.csp.Sign(
		id.UserKey,
		msg,
		&bccsp.IdemixNymSignerOpts{
			Nym:      id.NymKey,
			IssuerPK: id.msp.ipk,
		},
	)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

func (id *IdemixSigningIdentity) GetPublicVersion() Identity {
	return id.Idemixidentity
}

func readFile(file string) ([]byte, error) {
	fileCont, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read file %s", file)
	}

	return fileCont, nil
}

const (
	IdemixConfigDirMsp                  = "msp"
	IdemixConfigDirUser                 = "user"
	IdemixConfigFileIssuerPublicKey     = "IssuerPublicKey"
	IdemixConfigFileRevocationPublicKey = "RevocationPublicKey"
	IdemixConfigFileSigner              = "SignerConfig"
)

// GetIdemixMspConfig returns the configuration for the Idemix MSP
func GetIdemixMspConfig(dir string, ID string) (*msp.MSPConfig, error) {
	ipkBytes, err := readFile(filepath.Join(dir, IdemixConfigDirMsp, IdemixConfigFileIssuerPublicKey))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read issuer public key file")
	}

	revocationPkBytes, err := readFile(filepath.Join(dir, IdemixConfigDirMsp, IdemixConfigFileRevocationPublicKey))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read revocation public key file")
	}

	idemixConfig := &msp.IdemixMSPConfig{
		Name:         ID,
		Ipk:          ipkBytes,
		RevocationPk: revocationPkBytes,
	}

	signerBytes, err := readFile(filepath.Join(dir, IdemixConfigDirUser, IdemixConfigFileSigner))
	if err == nil {
		signerConfig := &msp.IdemixMSPSignerConfig{}
		err = proto.Unmarshal(signerBytes, signerConfig)
		if err != nil {
			return nil, err
		}
		idemixConfig.Signer = signerConfig
	}

	confBytes, err := proto.Marshal(idemixConfig)
	if err != nil {
		return nil, err
	}

	return &msp.MSPConfig{Config: confBytes, Type: int32(IDEMIX)}, nil
}
