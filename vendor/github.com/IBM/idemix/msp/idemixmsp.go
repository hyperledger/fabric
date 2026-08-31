/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	idemix "github.com/IBM/idemix/bccsp"
	"github.com/IBM/idemix/bccsp/keystore"
	idemixcrypto "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	bccsp "github.com/IBM/idemix/bccsp/types"
	im "github.com/IBM/idemix/msp/config"
	math "github.com/IBM/mathlib"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"
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

// Curve ID string constants matching the values written by idemixgen into IdemixMSPConfig.CurveId.
const (
	curveIDFP256BN_AMCL        = "FP256BN_AMCL"
	curveIDBN254               = "BN254"
	curveIDFP256BN_AMCL_MIRACL = "FP256BN_AMCL_MIRACL"
	curveIDBLS12_377_GURVY     = "BLS12_377_GURVY"
	curveIDBLS12_381_GURVY     = "BLS12_381_GURVY"
	curveIDBLS12_381           = "BLS12_381"
	curveIDBLS12_381_BBS       = "BLS12_381_BBS"
	curveIDBLS12_381_BBS_GURVY = "BLS12_381_BBS_GURVY"
)

// curveAndTranslator maps a curve_id string (as stored in IdemixMSPConfig) to the corresponding
// math.Curve and dlog Translator. Returns an error for unknown curve IDs.
func curveAndTranslator(curveID string) (*math.Curve, idemixcrypto.Translator, error) {
	switch curveID {
	case curveIDFP256BN_AMCL:
		c := math.Curves[math.FP256BN_AMCL]

		return c, &amcl.Fp256bn{C: c}, nil
	case curveIDBN254:
		c := math.Curves[math.BN254]

		return c, &amcl.Gurvy{C: c}, nil
	case curveIDFP256BN_AMCL_MIRACL:
		c := math.Curves[math.FP256BN_AMCL_MIRACL]

		return c, &amcl.Fp256bnMiracl{C: c}, nil
	case curveIDBLS12_377_GURVY:
		c := math.Curves[math.BLS12_377_GURVY]

		return c, &amcl.Gurvy{C: c}, nil
	case curveIDBLS12_381_GURVY:
		c := math.Curves[math.BLS12_381_GURVY]

		return c, &amcl.Gurvy{C: c}, nil
	case curveIDBLS12_381:
		c := math.Curves[math.BLS12_381]

		return c, &amcl.Gurvy{C: c}, nil
	case curveIDBLS12_381_BBS:
		c := math.Curves[math.BLS12_381_BBS]

		return c, &amcl.Gurvy{C: c}, nil
	case curveIDBLS12_381_BBS_GURVY:
		c := math.Curves[math.BLS12_381_BBS_GURVY]

		return c, &amcl.Gurvy{C: c}, nil
	default:
		return nil, nil, fmt.Errorf("unknown curve id %q", curveID)
	}
}

// Logger defines the logging interface required by Idemixmsp.
// This interface is compatible with the Go SDK log package and common logging facades.
type Logger interface {
	Debug(args ...any)
	Debugf(format string, args ...any)
	Errorf(format string, args ...any)
	IsEnabledFor(level zapcore.Level) bool
}

type Idemixmsp struct {
	csp          bccsp.BCCSP
	version      MSPVersion
	ipk          bccsp.Key
	signer       *IdemixSigningIdentity
	name         string
	revocationPK bccsp.Key
	epoch        int
	logger       Logger
	aries        bool
	exportable   bool
}

// defaultLogger is a simple logger implementation that wraps zap.SugaredLogger
// and satisfies the Logger interface.
type defaultLogger struct {
	*zap.SugaredLogger
	core zapcore.Core
}

// IsEnabledFor checks if the logger is enabled for the given level
func (l *defaultLogger) IsEnabledFor(level zapcore.Level) bool {
	return l.core.Enabled(level)
}

// newDefaultLogger creates a new logger instance compatible with the Logger interface.
// It uses zap for structured logging with a development configuration.
func newDefaultLogger(name string) Logger {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	zapLogger, err := config.Build()
	if err != nil {
		// Fallback to standard log if zap initialization fails
		log.Printf("failed to initialize zap logger: %v, using standard logger", err)

		return &stdLogger{prefix: name}
	}

	return &defaultLogger{
		SugaredLogger: zapLogger.Sugar().Named(name),
		core:          zapLogger.Core(),
	}
}

// stdLogger is a fallback logger using Go's standard log package
type stdLogger struct {
	prefix string
}

func (l *stdLogger) Debug(args ...any) {
	log.Println(append([]any{l.prefix + " [DEBUG]"}, args...)...)
}

func (l *stdLogger) Debugf(format string, args ...any) {
	log.Printf(l.prefix+" [DEBUG] "+format, args...)
}

func (l *stdLogger) Errorf(format string, args ...any) {
	log.Printf(l.prefix+" [ERROR] "+format, args...)
}

func (l *stdLogger) IsEnabledFor(level zapcore.Level) bool {
	return true // Standard logger always logs
}

// NewIdemixMsp creates a new instance of idemixmsp using the dlog scheme.
// The curve is determined at Setup time from IdemixMSPConfig.CurveId (default: FP256BN_AMCL).
func NewIdemixMsp(version MSPVersion) (MSP, error) {
	return NewIdemixMspWithLogger(version, newDefaultLogger("idemix"))
}

// NewIdemixMspWithLogger creates a new instance of idemixmsp using the dlog scheme with a custom logger.
// The curve is determined at Setup time from IdemixMSPConfig.CurveId (default: FP256BN_AMCL).
// If logger is nil, the default logger is used.
func NewIdemixMspWithLogger(version MSPVersion, logger Logger) (MSP, error) {
	if logger == nil {
		logger = newDefaultLogger("idemix")
	}
	logger.Debugf("Creating Idemix-based MSP instance")
	msp := Idemixmsp{logger: logger, version: version, aries: false, exportable: true}

	return &msp, nil
}

// NewIdemixMspAries creates a new instance of idemixmsp using the Aries/BBS+ scheme.
// The curve is determined at Setup time from IdemixMSPConfig.CurveId; only BLS12_381_BBS
// and BLS12_381_BBS_GURVY are accepted (default: BLS12_381_BBS).
func NewIdemixMspAries(version MSPVersion) (MSP, error) {
	return NewIdemixMspAriesWithLogger(version, newDefaultLogger("idemix"))
}

// NewIdemixMspAriesWithLogger creates a new instance of idemixmsp using the Aries/BBS+ scheme with a custom logger.
// The curve is determined at Setup time from IdemixMSPConfig.CurveId; only BLS12_381_BBS
// and BLS12_381_BBS_GURVY are accepted (default: BLS12_381_BBS).
// If logger is nil, the default logger is used.
func NewIdemixMspAriesWithLogger(version MSPVersion, logger Logger) (MSP, error) {
	if logger == nil {
		logger = newDefaultLogger("idemix")
	}
	logger.Debugf("Creating Idemix-based MSP instance")
	msp := Idemixmsp{logger: logger, version: version, aries: true, exportable: true}

	return &msp, nil
}

func (msp *Idemixmsp) Setup(conf1 *m.MSPConfig) error {
	msp.logger.Debugf("Setting up Idemix-based MSP instance")

	if conf1 == nil {
		return errors.New("setup error: nil conf reference")
	}

	var conf im.IdemixMSPConfig
	err := proto.Unmarshal(conf1.Config, &conf)
	if err != nil {
		return fmt.Errorf("failed unmarshalling idemix msp config: %w", err)
	}

	msp.name = conf.Name
	msp.logger.Debugf("Setting up Idemix MSP instance %s", msp.name)

	// Enforce that the config type matches the constructor used.
	if msp.aries {
		if conf1.Type != int32(IDEMIX_ARIES) {
			return fmt.Errorf("setup error: aries MSP requires config of type IDEMIX_ARIES, got %d", conf1.Type)
		}
	} else {
		if conf1.Type != int32(IDEMIX) {
			return fmt.Errorf("setup error: dlog MSP requires config of type IDEMIX, got %d", conf1.Type)
		}
	}

	// Determine the curve from config.CurveId.
	curveID := conf.CurveId
	if msp.aries {
		switch curveID {
		case "", curveIDBLS12_381_BBS:
			curveID = curveIDBLS12_381_BBS
		case curveIDBLS12_381_BBS_GURVY:
			// accepted
		default:
			return fmt.Errorf("setup error: aries MSP requires a BBS curve, got %q", curveID)
		}
	} else {
		if curveID == "" {
			curveID = curveIDFP256BN_AMCL
		}
	}

	curve, tr, err := curveAndTranslator(curveID)
	if err != nil {
		return fmt.Errorf("setup error: %w", err)
	}

	// Build the BCCSP using the curve selected from config.
	if msp.aries {
		msp.csp, err = idemix.NewAries(&keystore.Dummy{}, curve, tr, msp.exportable)
	} else {
		msp.csp, err = idemix.New(&keystore.Dummy{}, curve, tr, msp.exportable)
	}
	if err != nil {
		return fmt.Errorf("setup error: failed to create BCCSP: %w", err)
	}

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
		var importErr *bccsp.IdemixIssuerPublicKeyImporterError
		ok := errors.As(err, &importErr)
		if !ok {
			panic("unexpected condition, BCCSP did not return the expected *bccsp.IdemixIssuerPublicKeyImporterError")
		}
		switch importErr.Type {
		case bccsp.IdemixIssuerPublicKeyImporterUnmarshallingError:
			return fmt.Errorf("failed to unmarshal ipk from idemix msp config: %w", err)
		case bccsp.IdemixIssuerPublicKeyImporterHashError:
			return fmt.Errorf("setting the hash of the issuer public key failed: %w", err)
		case bccsp.IdemixIssuerPublicKeyImporterValidationError:
			return fmt.Errorf("cannot setup idemix msp with invalid public key: %w", err)
		case bccsp.IdemixIssuerPublicKeyImporterNumAttributesError:
			fallthrough
		case bccsp.IdemixIssuerPublicKeyImporterAttributeNameError:
			return errors.New("issuer public key must have attributes OU, Role, EnrollmentId, and RevocationHandle")
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
		return fmt.Errorf("failed to import revocation public key: %w", err)
	}
	msp.revocationPK = RevocationPublicKey

	if conf.Signer == nil {
		// No credential in config, so we don't setup a default signer
		msp.logger.Debug("idemix msp setup as verification only msp (no key material found)")

		return nil
	}

	// A credential is present in the config, so we setup a default signer

	// Import User secret key
	UserKey, err := msp.csp.KeyImport(conf.Signer.Sk, &bccsp.IdemixUserSecretKeyImportOpts{Temporary: true})
	if err != nil {
		return fmt.Errorf("failed importing signer secret key: %w", err)
	}

	// Derive NymPublicKey
	NymKey, err := msp.csp.KeyDeriv(UserKey, &bccsp.IdemixNymKeyDerivationOpts{Temporary: true, IssuerPK: IssuerPublicKey})
	if err != nil {
		return fmt.Errorf("failed deriving nym: %w", err)
	}
	NymPublicKey, err := NymKey.PublicKey()
	if err != nil {
		return fmt.Errorf("failed getting public nym key: %w", err)
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
	if err != nil {
		return fmt.Errorf("credential is not cryptographically valid: %w", err)
	}
	if !valid {
		return errors.New("credential is not cryptographically valid")
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
		return fmt.Errorf("failed to setup cryptographic proof of identity: %w", err)
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
	msp.logger.Debugf("Obtaining default idemix signing identity")

	if msp.signer == nil {
		return nil, errors.New("no default signer setup")
	}

	return msp.signer, nil
}

func (msp *Idemixmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	sID := &m.SerializedIdentity{}
	err := proto.Unmarshal(serializedID, sID)
	if err != nil {
		return nil, fmt.Errorf("could not deserialize a SerializedIdentity: %w", err)
	}

	if sID.Mspid != msp.name {
		return nil, fmt.Errorf("expected MSP ID %s, received %s", msp.name, sID.Mspid)
	}

	return msp.DeserializeIdentityInternal(sID.GetIdBytes())
}

func (msp *Idemixmsp) DeserializeIdentityInternal(serializedID []byte) (Identity, error) {
	msp.logger.Debug("idemixmsp: deserializing identity")
	serialized := new(im.SerializedIdemixIdentity)
	err := proto.Unmarshal(serializedID, serialized)
	if err != nil {
		return nil, fmt.Errorf("could not deserialize a SerializedIdemixIdentity: %w", err)
	}
	if serialized.NymX == nil || serialized.NymY == nil {
		return nil, errors.New("unable to deserialize idemix identity: pseudonym is invalid")
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
		return nil, fmt.Errorf("failed to import nym public key: %w", err)
	}

	// OU
	ou := &m.OrganizationUnit{}
	err = proto.Unmarshal(serialized.Ou, ou)
	if err != nil {
		return nil, fmt.Errorf("cannot deserialize the OU of the identity: %w", err)
	}

	// Role
	role := &m.MSPRole{}
	err = proto.Unmarshal(serialized.Role, role)
	if err != nil {
		return nil, fmt.Errorf("cannot deserialize the role of the identity: %w", err)
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
		return fmt.Errorf("identity type %T is not recognized", t)
	}

	msp.logger.Debugf("Validating identity %+v", identity)
	if identity.GetMSPIdentifier() != msp.name {
		return errors.New("the supplied identity does not belong to this msp")
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
			Nym:      id.NymPublicKey,
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
		return fmt.Errorf("identity is not valid with respect to this MSP: %w", err)
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
			return fmt.Errorf("could not unmarshal MSPRole from principal: %w", err)
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if mspRole.MspIdentifier != msp.name {
			return fmt.Errorf("the identity is a member of a different MSP (expected %s, got %s)", mspRole.MspIdentifier, id.GetMSPIdentifier())
		}

		// now we validate the different msp roles
		switch mspRole.Role {
		case m.MSPRole_MEMBER:
			// in the case of member, we simply check
			// whether this identity is valid for the MSP
			msp.logger.Debugf("Checking if identity satisfies MEMBER role for %s", msp.name)

			return nil
		case m.MSPRole_ADMIN:
			msp.logger.Debugf("Checking if identity satisfies ADMIN role for %s", msp.name)
			if id.(*Idemixidentity).Role.Role != m.MSPRole_ADMIN {
				return errors.New("user is not an admin")
			}

			return nil
		case m.MSPRole_PEER:
			if msp.version >= MSPv1_3 {
				return errors.New("idemixmsp only supports client use, so it cannot satisfy an MSPRole PEER principal")
			}

			fallthrough
		case m.MSPRole_CLIENT:
			if msp.version >= MSPv1_3 {
				return nil // any valid idemixmsp member must be a client
			}

			fallthrough
		default:
			return fmt.Errorf("invalid MSP role type %d", int32(mspRole.Role))
		}
		// in this case we have to serialize this instance
		// and compare it byte-by-byte with Principal
	case m.MSPPrincipal_IDENTITY:
		msp.logger.Debugf("Checking if identity satisfies IDENTITY principal")
		idBytes, err := id.Serialize()
		if err != nil {
			return fmt.Errorf("could not serialize this identity instance: %w", err)
		}

		rv := bytes.Compare(idBytes, principal.Principal)
		if rv == 0 {
			return nil
		}

		return errors.New("the identities do not match")

	case m.MSPPrincipal_ORGANIZATION_UNIT:
		ou := &m.OrganizationUnit{}
		err := proto.Unmarshal(principal.Principal, ou)
		if err != nil {
			return fmt.Errorf("could not unmarshal OU from principal: %w", err)
		}

		msp.logger.Debugf("Checking if identity is part of OU \"%s\" of mspid \"%s\"", ou.OrganizationalUnitIdentifier, ou.MspIdentifier)

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if ou.MspIdentifier != msp.name {
			return fmt.Errorf("the identity is a member of a different MSP (expected %s, got %s)", ou.MspIdentifier, id.GetMSPIdentifier())
		}

		if ou.OrganizationalUnitIdentifier != id.(*Idemixidentity).OU.OrganizationalUnitIdentifier {
			return errors.New("user is not part of the desired organizational unit")
		}

		return nil
	case m.MSPPrincipal_COMBINED:
		if msp.version <= MSPv1_1 {
			return errors.New("combined MSP Principals are unsupported in MSPv1_1")
		}

		// Principal is a combination of multiple principals.
		principals := &m.CombinedPrincipal{}
		err := proto.Unmarshal(principal.Principal, principals)
		if err != nil {
			return fmt.Errorf("could not unmarshal CombinedPrincipal from principal: %w", err)
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
			return errors.New("anonymity MSP Principals are unsupported in MSPv1_1")
		}

		anon := &m.MSPIdentityAnonymity{}
		err := proto.Unmarshal(principal.Principal, anon)
		if err != nil {
			return fmt.Errorf("could not unmarshal MSPIdentityAnonymity from principal: %w", err)
		}
		switch anon.AnonymityType {
		case m.MSPIdentityAnonymity_ANONYMOUS:
			return nil
		case m.MSPIdentityAnonymity_NOMINAL:
			return errors.New("principal is nominal, but idemix MSP is anonymous")
		default:
			return fmt.Errorf("unknown principal anonymity type: %d", anon.AnonymityType)
		}
	default:
		return fmt.Errorf("invalid principal type %d", int32(principal.PrincipalClassification))
	}
}

// IsWellFormed checks if the given identity can be deserialized into its provider-specific .
// In this MSP implementation, an identity is considered well formed if it contains a
// marshaled SerializedIdemixIdentity protobuf message.
func (id *Idemixmsp) IsWellFormed(identity *m.SerializedIdentity) error {
	sId := new(im.SerializedIdemixIdentity)
	err := proto.Unmarshal(identity.IdBytes, sId)
	if err != nil {
		return fmt.Errorf("not an idemix identity: %w", err)
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
		id.msp.logger.Errorf("failed to marshal ipk in GetOrganizationalUnits: %s", err)

		return nil
	}

	return []*OUIdentifier{{certifiersIdentifier, id.OU.OrganizationalUnitIdentifier}}
}

func (id *Idemixidentity) Validate() error {
	return id.msp.Validate(id)
}

func (id *Idemixidentity) Verify(msg []byte, sig []byte) error {
	if id.msp.logger.IsEnabledFor(zapcore.DebugLevel) {
		id.msp.logger.Debugf("Verify Idemix sig: msg = %s", hex.Dump(msg))
		id.msp.logger.Debugf("Verify Idemix sig: sig = %s", hex.Dump(sig))
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
	serialized := &im.SerializedIdemixIdentity{}

	raw, err := id.NymPublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("could not serialize nym of identity %s: %w", id.id, err)
	}
	// This is an assumption on how the underlying idemix implementation work.
	// TODO: change this in future version
	serialized.NymX = raw[:len(raw)/2]
	serialized.NymY = raw[len(raw)/2:]
	ouBytes, err := proto.Marshal(id.OU)
	if err != nil {
		return nil, fmt.Errorf("could not marshal OU of identity %s: %w", id.id, err)
	}

	roleBytes, err := proto.Marshal(id.Role)
	if err != nil {
		return nil, fmt.Errorf("could not marshal role of identity %s: %w", id.id, err)
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
		return nil, fmt.Errorf("could not marshal a SerializedIdentity structure for identity %s: %w", id.id, err)
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
	id.msp.logger.Debugf("Idemix identity %s is signing", id.GetIdentifier())

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
	fileCont, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", file, err)
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
func GetIdemixMspConfig(dir string, ID string) (*m.MSPConfig, error) {
	return GetIdemixMspConfigWithType(dir, ID, IDEMIX)
}

// GetIdemixMspConfigWithType returns the configuration for the Idemix MSP of the specified type
func GetIdemixMspConfigWithType(dir string, ID string, mspType ProviderType) (*m.MSPConfig, error) {
	if mspType < 0 {
		return nil, fmt.Errorf("msp type %d is not supported", mspType)
	}

	ipkBytes, err := readFile(filepath.Join(dir, IdemixConfigDirMsp, IdemixConfigFileIssuerPublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to read issuer public key file: %w", err)
	}

	revocationPkBytes, err := readFile(filepath.Join(dir, IdemixConfigDirMsp, IdemixConfigFileRevocationPublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to read revocation public key file: %w", err)
	}

	idemixConfig := &im.IdemixMSPConfig{
		Name:         ID,
		Ipk:          ipkBytes,
		RevocationPk: revocationPkBytes,
	}

	signerBytes, err := readFile(filepath.Join(dir, IdemixConfigDirUser, IdemixConfigFileSigner))
	if err == nil {
		signerConfig := &im.IdemixMSPSignerConfig{}
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

	return &m.MSPConfig{Config: confBytes, Type: int32(mspType)}, nil //nolint:gosec
}
