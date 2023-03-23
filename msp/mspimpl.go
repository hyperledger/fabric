/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"strings"

	"github.com/golang/protobuf/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/signer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/pkg/errors"
)

// mspSetupFuncType is the prototype of the setup function
type mspSetupFuncType func(config *m.FabricMSPConfig) error

// validateIdentityOUsFuncType is the prototype of the function to validate identity's OUs
type validateIdentityOUsFuncType func(id *identity) error

// satisfiesPrincipalInternalFuncType is the prototype of the function to check if principals are satisfied
type satisfiesPrincipalInternalFuncType func(id Identity, principal *m.MSPPrincipal) error

// setupAdminInternalFuncType is a prototype of the function to setup the admins
type setupAdminInternalFuncType func(conf *m.FabricMSPConfig) error

// This is an instantiation of an MSP that
// uses BCCSP for its cryptographic primitives.
type bccspmsp struct {
	// version specifies the behaviour of this msp
	version MSPVersion
	// The following function pointers are used to change the behaviour
	// of this MSP depending on its version.
	// internalSetupFunc is the pointer to the setup function
	internalSetupFunc mspSetupFuncType

	// internalValidateIdentityOusFunc is the pointer to the function to validate identity's OUs
	internalValidateIdentityOusFunc validateIdentityOUsFuncType

	// internalSatisfiesPrincipalInternalFunc is the pointer to the function to check if principals are satisfied
	internalSatisfiesPrincipalInternalFunc satisfiesPrincipalInternalFuncType

	// internalSetupAdmin is the pointer to the function that setup the administrators of this msp
	internalSetupAdmin setupAdminInternalFuncType

	// list of CA certs we trust
	rootCerts []Identity

	// list of intermediate certs we trust
	intermediateCerts []Identity

	// list of CA TLS certs we trust
	tlsRootCerts [][]byte

	// list of intermediate TLS certs we trust
	tlsIntermediateCerts [][]byte

	// certificationTreeInternalNodesMap whose keys correspond to the raw material
	// (DER representation) of a certificate casted to a string, and whose values
	// are boolean. True means that the certificate is an internal node of the certification tree.
	// False means that the certificate corresponds to a leaf of the certification tree.
	certificationTreeInternalNodesMap map[string]bool

	// list of signing identities
	signer SigningIdentity

	// list of admin identities
	admins []Identity

	// the crypto provider
	bccsp bccsp.BCCSP

	// the provider identifier for this MSP
	name string

	// verification options for MSP members
	opts *x509.VerifyOptions

	// list of certificate revocation lists
	CRL []*pkix.CertificateList

	// list of OUs
	ouIdentifiers map[string][][]byte

	// cryptoConfig contains
	cryptoConfig *m.FabricCryptoConfig

	// NodeOUs configuration
	ouEnforcement bool
	// These are the OUIdentifiers of the clients, peers, admins and orderers.
	// They are used to tell apart these entities
	clientOU, peerOU, adminOU, ordererOU *OUIdentifier
}

// newBccspMsp returns an MSP instance backed up by a BCCSP
// crypto provider. It handles x.509 certificates and can
// generate identities and signing identities backed by
// certificates and keypairs
func newBccspMsp(version MSPVersion, defaultBCCSP bccsp.BCCSP) (MSP, error) {
	mspLogger.Debugf("Creating BCCSP-based MSP instance")

	theMsp := &bccspmsp{}
	theMsp.version = version
	theMsp.bccsp = defaultBCCSP
	switch version {
	case MSPv1_0:
		theMsp.internalSetupFunc = theMsp.setupV1
		theMsp.internalValidateIdentityOusFunc = theMsp.validateIdentityOUsV1
		theMsp.internalSatisfiesPrincipalInternalFunc = theMsp.satisfiesPrincipalInternalPreV13
		theMsp.internalSetupAdmin = theMsp.setupAdminsPreV142
	case MSPv1_1:
		theMsp.internalSetupFunc = theMsp.setupV11
		theMsp.internalValidateIdentityOusFunc = theMsp.validateIdentityOUsV11
		theMsp.internalSatisfiesPrincipalInternalFunc = theMsp.satisfiesPrincipalInternalPreV13
		theMsp.internalSetupAdmin = theMsp.setupAdminsPreV142
	case MSPv1_3:
		theMsp.internalSetupFunc = theMsp.setupV11
		theMsp.internalValidateIdentityOusFunc = theMsp.validateIdentityOUsV11
		theMsp.internalSatisfiesPrincipalInternalFunc = theMsp.satisfiesPrincipalInternalV13
		theMsp.internalSetupAdmin = theMsp.setupAdminsPreV142
	case MSPv1_4_3:
		theMsp.internalSetupFunc = theMsp.setupV142
		theMsp.internalValidateIdentityOusFunc = theMsp.validateIdentityOUsV142
		theMsp.internalSatisfiesPrincipalInternalFunc = theMsp.satisfiesPrincipalInternalV142
		theMsp.internalSetupAdmin = theMsp.setupAdminsV142
	default:
		return nil, errors.Errorf("Invalid MSP version [%v]", version)
	}

	return theMsp, nil
}

// NewBccspMspWithKeyStore allows to create a BCCSP-based MSP whose underlying
// crypto material is available through the passed keystore
func NewBccspMspWithKeyStore(version MSPVersion, keyStore bccsp.KeyStore, bccsp bccsp.BCCSP) (MSP, error) {
	thisMSP, err := newBccspMsp(version, bccsp)
	if err != nil {
		return nil, err
	}

	csp, err := sw.NewWithParams(
		factory.GetDefaultOpts().SW.Security,
		factory.GetDefaultOpts().SW.Hash,
		keyStore)
	if err != nil {
		return nil, err
	}
	thisMSP.(*bccspmsp).bccsp = csp

	return thisMSP, nil
}

func (msp *bccspmsp) getCertFromPem(idBytes []byte) (*x509.Certificate, error) {
	if idBytes == nil {
		return nil, errors.New("getCertFromPem error: nil idBytes")
	}

	// Decode the pem bytes
	pemCert, _ := pem.Decode(idBytes)
	if pemCert == nil {
		return nil, errors.Errorf("getCertFromPem error: could not decode pem bytes [%v]", idBytes)
	}

	// get a cert
	var cert *x509.Certificate
	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "getCertFromPem error: failed to parse x509 cert")
	}

	return cert, nil
}

func (msp *bccspmsp) getIdentityFromConf(idBytes []byte) (Identity, bccsp.Key, error) {
	// get a cert
	cert, err := msp.getCertFromPem(idBytes)
	if err != nil {
		return nil, nil, err
	}

	// get the public key in the right format
	certPubK, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, nil, err
	}

	mspId, err := newIdentity(cert, certPubK, msp)
	if err != nil {
		return nil, nil, err
	}

	return mspId, certPubK, nil
}

func (msp *bccspmsp) getSigningIdentityFromConf(sidInfo *m.SigningIdentityInfo) (SigningIdentity, error) {
	if sidInfo == nil {
		return nil, errors.New("getIdentityFromBytes error: nil sidInfo")
	}

	// Extract the public part of the identity
	idPub, pubKey, err := msp.getIdentityFromConf(sidInfo.PublicSigner)
	if err != nil {
		return nil, err
	}

	// Find the matching private key in the BCCSP keystore
	privKey, err := msp.bccsp.GetKey(pubKey.SKI())
	// Less Secure: Attempt to import Private Key from KeyInfo, if BCCSP was not able to find the key
	if err != nil {
		mspLogger.Debugf("Could not find SKI [%s], trying KeyMaterial field: %+v\n", hex.EncodeToString(pubKey.SKI()), err)
		if sidInfo.PrivateSigner == nil || sidInfo.PrivateSigner.KeyMaterial == nil {
			return nil, errors.New("KeyMaterial not found in SigningIdentityInfo")
		}

		pemKey, _ := pem.Decode(sidInfo.PrivateSigner.KeyMaterial)
		if pemKey == nil {
			return nil, errors.Errorf("%s: wrong PEM encoding", sidInfo.PrivateSigner.KeyIdentifier)
		}
		privKey, err = msp.bccsp.KeyImport(pemKey.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
		if err != nil {
			return nil, errors.WithMessage(err, "getIdentityFromBytes error: Failed to import EC private key")
		}
	}

	// get the peer signer
	peerSigner, err := signer.New(msp.bccsp, privKey)
	if err != nil {
		return nil, errors.WithMessage(err, "getIdentityFromBytes error: Failed initializing bccspCryptoSigner")
	}

	return newSigningIdentity(idPub.(*identity).cert, idPub.(*identity).pk, peerSigner, msp)
}

// Setup sets up the internal data structures
// for this MSP, given an MSPConfig ref; it
// returns nil in case of success or an error otherwise
func (msp *bccspmsp) Setup(conf1 *m.MSPConfig) error {
	if conf1 == nil {
		return errors.New("Setup error: nil conf reference")
	}

	// given that it's an msp of type fabric, extract the MSPConfig instance
	conf := &m.FabricMSPConfig{}
	err := proto.Unmarshal(conf1.Config, conf)
	if err != nil {
		return errors.Wrap(err, "failed unmarshalling fabric msp config")
	}

	// set the name for this msp
	msp.name = conf.Name
	mspLogger.Debugf("Setting up MSP instance %s", msp.name)

	// setup
	return msp.internalSetupFunc(conf)
}

// GetVersion returns the version of this MSP
func (msp *bccspmsp) GetVersion() MSPVersion {
	return msp.version
}

// GetType returns the type for this MSP
func (msp *bccspmsp) GetType() ProviderType {
	return FABRIC
}

// GetIdentifier returns the MSP identifier for this instance
func (msp *bccspmsp) GetIdentifier() (string, error) {
	return msp.name, nil
}

// GetTLSRootCerts returns the root certificates for this MSP
func (msp *bccspmsp) GetTLSRootCerts() [][]byte {
	return msp.tlsRootCerts
}

// GetTLSIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *bccspmsp) GetTLSIntermediateCerts() [][]byte {
	return msp.tlsIntermediateCerts
}

// GetDefaultSigningIdentity returns the
// default signing identity for this MSP (if any)
func (msp *bccspmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Debugf("Obtaining default signing identity")

	if msp.signer == nil {
		return nil, errors.New("this MSP does not possess a valid default signing identity")
	}

	return msp.signer, nil
}

// Validate attempts to determine whether
// the supplied identity is valid according
// to this MSP's roots of trust; it returns
// nil in case the identity is valid or an
// error otherwise
func (msp *bccspmsp) Validate(id Identity) error {
	mspLogger.Debugf("MSP %s validating identity", msp.name)

	switch id := id.(type) {
	// If this identity is of this specific type,
	// this is how I can validate it given the
	// root of trust this MSP has
	case *identity:
		return msp.validateIdentity(id)
	default:
		return errors.New("identity type not recognized")
	}
}

// hasOURole checks that the identity belongs to the organizational unit
// associated to the specified MSPRole.
// This function does not check the certifiers identifier.
// Appropriate validation needs to be enforced before.
func (msp *bccspmsp) hasOURole(id Identity, mspRole m.MSPRole_MSPRoleType) error {
	// Check NodeOUs
	if !msp.ouEnforcement {
		return errors.New("NodeOUs not activated. Cannot tell apart identities.")
	}

	mspLogger.Debugf("MSP %s checking if the identity is a client", msp.name)

	switch id := id.(type) {
	// If this identity is of this specific type,
	// this is how I can validate it given the
	// root of trust this MSP has
	case *identity:
		return msp.hasOURoleInternal(id, mspRole)
	default:
		return errors.New("Identity type not recognized")
	}
}

func (msp *bccspmsp) hasOURoleInternal(id *identity, mspRole m.MSPRole_MSPRoleType) error {
	var nodeOU *OUIdentifier
	switch mspRole {
	case m.MSPRole_CLIENT:
		nodeOU = msp.clientOU
	case m.MSPRole_PEER:
		nodeOU = msp.peerOU
	case m.MSPRole_ADMIN:
		nodeOU = msp.adminOU
	case m.MSPRole_ORDERER:
		nodeOU = msp.ordererOU
	default:
		return errors.New("Invalid MSPRoleType. It must be CLIENT, PEER, ADMIN or ORDERER")
	}

	if nodeOU == nil {
		return errors.Errorf("cannot test for classification, node ou for type [%s], not defined, msp: [%s]", mspRole, msp.name)
	}

	for _, OU := range id.GetOrganizationalUnits() {
		if OU.OrganizationalUnitIdentifier == nodeOU.OrganizationalUnitIdentifier {
			return nil
		}
	}

	return errors.Errorf("The identity does not contain OU [%s], MSP: [%s]", mspRole, msp.name)
}

// DeserializeIdentity returns an Identity given the byte-level
// representation of a SerializedIdentity struct
func (msp *bccspmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	mspLogger.Debug("Obtaining identity")

	// We first deserialize to a SerializedIdentity to get the MSP ID
	sId := &m.SerializedIdentity{}
	err := proto.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdentity")
	}

	if sId.Mspid != msp.name {
		return nil, errors.Errorf("expected MSP ID %s, received %s", msp.name, sId.Mspid)
	}

	return msp.deserializeIdentityInternal(sId.IdBytes)
}

// deserializeIdentityInternal returns an identity given its byte-level representation
func (msp *bccspmsp) deserializeIdentityInternal(serializedIdentity []byte) (Identity, error) {
	// This MSP will always deserialize certs this way
	bl, _ := pem.Decode(serializedIdentity)
	if bl == nil {
		return nil, errors.New("could not decode the PEM structure")
	}
	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parseCertificate failed")
	}

	// Now we have the certificate; make sure that its fields
	// (e.g. the Issuer.OU or the Subject.OU) match with the
	// MSP id that this MSP has; otherwise it might be an attack
	// TODO!
	// We can't do it yet because there is no standardized way
	// (yet) to encode the MSP ID into the x.509 body of a cert

	pub, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to import certificate's public key")
	}

	return newIdentity(cert, pub, msp)
}

// SatisfiesPrincipal returns nil if the identity matches the principal or an error otherwise
func (msp *bccspmsp) SatisfiesPrincipal(id Identity, principal *m.MSPPrincipal) error {
	principals, err := collectPrincipals(principal, msp.GetVersion())
	if err != nil {
		return err
	}
	for _, principal := range principals {
		err = msp.internalSatisfiesPrincipalInternalFunc(id, principal)
		if err != nil {
			return err
		}
	}
	return nil
}

// collectPrincipals collects principals from combined principals into a single MSPPrincipal slice.
func collectPrincipals(principal *m.MSPPrincipal, mspVersion MSPVersion) ([]*m.MSPPrincipal, error) {
	switch principal.PrincipalClassification {
	case m.MSPPrincipal_COMBINED:
		// Combined principals are not supported in MSP v1.0 or v1.1
		if mspVersion <= MSPv1_1 {
			return nil, errors.Errorf("invalid principal type %d", int32(principal.PrincipalClassification))
		}
		// Principal is a combination of multiple principals.
		principals := &m.CombinedPrincipal{}
		err := proto.Unmarshal(principal.Principal, principals)
		if err != nil {
			return nil, errors.Wrap(err, "could not unmarshal CombinedPrincipal from principal")
		}
		// Return an error if there are no principals in the combined principal.
		if len(principals.Principals) == 0 {
			return nil, errors.New("No principals in CombinedPrincipal")
		}
		// Recursively call msp.collectPrincipals for all combined principals.
		// There is no limit for the levels of nesting for the combined principals.
		var principalsSlice []*m.MSPPrincipal
		for _, cp := range principals.Principals {
			internalSlice, err := collectPrincipals(cp, mspVersion)
			if err != nil {
				return nil, err
			}
			principalsSlice = append(principalsSlice, internalSlice...)
		}
		// All the combined principals have been collected into principalsSlice
		return principalsSlice, nil
	default:
		return []*m.MSPPrincipal{principal}, nil
	}
}

// satisfiesPrincipalInternalPreV13 takes as arguments the identity and the principal.
// The function returns an error if one occurred.
// The function implements the behavior of an MSP up to and including v1.1.
func (msp *bccspmsp) satisfiesPrincipalInternalPreV13(id Identity, principal *m.MSPPrincipal) error {
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
			return msp.Validate(id)
		case m.MSPRole_ADMIN:
			mspLogger.Debugf("Checking if identity satisfies ADMIN role for %s", msp.name)
			// in the case of admin, we check that the
			// id is exactly one of our admins
			if msp.isInAdmins(id.(*identity)) {
				return nil
			}
			return errors.New("This identity is not an admin")
		case m.MSPRole_CLIENT:
			fallthrough
		case m.MSPRole_PEER:
			mspLogger.Debugf("Checking if identity satisfies role [%s] for %s", m.MSPRole_MSPRoleType_name[int32(mspRole.Role)], msp.name)
			if err := msp.Validate(id); err != nil {
				return errors.Wrapf(err, "The identity is not valid under this MSP [%s]", msp.name)
			}

			if err := msp.hasOURole(id, mspRole.Role); err != nil {
				return errors.Wrapf(err, "The identity is not a [%s] under this MSP [%s]", m.MSPRole_MSPRoleType_name[int32(mspRole.Role)], msp.name)
			}
			return nil
		default:
			return errors.Errorf("invalid MSP role type %d", int32(mspRole.Role))
		}
	case m.MSPPrincipal_IDENTITY:
		// in this case we have to deserialize the principal's identity
		// and compare it byte-by-byte with our cert
		principalId, err := msp.DeserializeIdentity(principal.Principal)
		if err != nil {
			return errors.WithMessage(err, "invalid identity principal, not a certificate")
		}

		if bytes.Equal(id.(*identity).cert.Raw, principalId.(*identity).cert.Raw) {
			return principalId.Validate()
		}

		return errors.New("The identities do not match")
	case m.MSPPrincipal_ORGANIZATION_UNIT:
		// Principal contains the OrganizationUnit
		OU := &m.OrganizationUnit{}
		err := proto.Unmarshal(principal.Principal, OU)
		if err != nil {
			return errors.Wrap(err, "could not unmarshal OrganizationUnit from principal")
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if OU.MspIdentifier != msp.name {
			return errors.Errorf("the identity is a member of a different MSP (expected %s, got %s)", OU.MspIdentifier, id.GetMSPIdentifier())
		}

		// we then check if the identity is valid with this MSP
		// and fail if it is not
		err = msp.Validate(id)
		if err != nil {
			return err
		}

		// now we check whether any of this identity's OUs match the requested one
		for _, ou := range id.GetOrganizationalUnits() {
			if ou.OrganizationalUnitIdentifier == OU.OrganizationalUnitIdentifier &&
				bytes.Equal(ou.CertifiersIdentifier, OU.CertifiersIdentifier) {
				return nil
			}
		}

		// if we are here, no match was found, return an error
		return errors.New("The identities do not match")
	default:
		return errors.Errorf("invalid principal type %d", int32(principal.PrincipalClassification))
	}
}

// satisfiesPrincipalInternalV13 takes as arguments the identity and the principal.
// The function returns an error if one occurred.
// The function implements the additional behavior expected of an MSP starting from v1.3.
// For pre-v1.3 functionality, the function calls the satisfiesPrincipalInternalPreV13.
func (msp *bccspmsp) satisfiesPrincipalInternalV13(id Identity, principal *m.MSPPrincipal) error {
	switch principal.PrincipalClassification {
	case m.MSPPrincipal_COMBINED:
		return errors.New("SatisfiesPrincipalInternal shall not be called with a CombinedPrincipal")
	case m.MSPPrincipal_ANONYMITY:
		anon := &m.MSPIdentityAnonymity{}
		err := proto.Unmarshal(principal.Principal, anon)
		if err != nil {
			return errors.Wrap(err, "could not unmarshal MSPIdentityAnonymity from principal")
		}
		switch anon.AnonymityType {
		case m.MSPIdentityAnonymity_ANONYMOUS:
			return errors.New("Principal is anonymous, but X.509 MSP does not support anonymous identities")
		case m.MSPIdentityAnonymity_NOMINAL:
			return nil
		default:
			return errors.Errorf("Unknown principal anonymity type: %d", anon.AnonymityType)
		}

	default:
		// Use the pre-v1.3 function to check other principal types
		return msp.satisfiesPrincipalInternalPreV13(id, principal)
	}
}

// satisfiesPrincipalInternalV142 takes as arguments the identity and the principal.
// The function returns an error if one occurred.
// The function implements the additional behavior expected of an MSP starting from v2.0.
// For v1.3 functionality, the function calls the satisfiesPrincipalInternalPreV13.
func (msp *bccspmsp) satisfiesPrincipalInternalV142(id Identity, principal *m.MSPPrincipal) error {
	_, okay := id.(*identity)
	if !okay {
		return errors.New("invalid identity type, expected *identity")
	}

	switch principal.PrincipalClassification {
	case m.MSPPrincipal_ROLE:
		if !msp.ouEnforcement {
			break
		}

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

		// now we validate the admin role only, the other roles are left to the v1.3 function
		switch mspRole.Role {
		case m.MSPRole_ADMIN:
			mspLogger.Debugf("Checking if identity has been named explicitly as an admin for %s", msp.name)
			// in the case of admin, we check that the
			// id is exactly one of our admins
			if msp.isInAdmins(id.(*identity)) {
				return nil
			}

			// or it carries the Admin OU, in this case check that the identity is valid as well.
			mspLogger.Debugf("Checking if identity carries the admin ou for %s", msp.name)
			if err := msp.Validate(id); err != nil {
				return errors.Wrapf(err, "The identity is not valid under this MSP [%s]", msp.name)
			}

			if err := msp.hasOURole(id, m.MSPRole_ADMIN); err != nil {
				return errors.Wrapf(err, "The identity is not an admin under this MSP [%s]", msp.name)
			}

			return nil
		case m.MSPRole_ORDERER:
			mspLogger.Debugf("Checking if identity satisfies role [%s] for %s", m.MSPRole_MSPRoleType_name[int32(mspRole.Role)], msp.name)
			if err := msp.Validate(id); err != nil {
				return errors.Wrapf(err, "The identity is not valid under this MSP [%s]", msp.name)
			}

			if err := msp.hasOURole(id, mspRole.Role); err != nil {
				return errors.Wrapf(err, "The identity is not a [%s] under this MSP [%s]", m.MSPRole_MSPRoleType_name[int32(mspRole.Role)], msp.name)
			}
			return nil
		}
	}

	// Use the v1.3 function to check other principal types
	return msp.satisfiesPrincipalInternalV13(id, principal)
}

func (msp *bccspmsp) isInAdmins(id *identity) bool {
	for _, admincert := range msp.admins {
		if bytes.Equal(id.cert.Raw, admincert.(*identity).cert.Raw) {
			// we do not need to check whether the admin is a valid identity
			// according to this MSP, since we already check this at Setup time
			// if there is a match, we can just return
			return true
		}
	}
	return false
}

// getCertificationChain returns the certification chain of the passed identity within this msp
func (msp *bccspmsp) getCertificationChain(id Identity) ([]*x509.Certificate, error) {
	mspLogger.Debugf("MSP %s getting certification chain", msp.name)

	switch id := id.(type) {
	// If this identity is of this specific type,
	// this is how I can validate it given the
	// root of trust this MSP has
	case *identity:
		return msp.getCertificationChainForBCCSPIdentity(id)
	default:
		return nil, errors.New("identity type not recognized")
	}
}

// getCertificationChainForBCCSPIdentity returns the certification chain of the passed bccsp identity within this msp
func (msp *bccspmsp) getCertificationChainForBCCSPIdentity(id *identity) ([]*x509.Certificate, error) {
	if id == nil {
		return nil, errors.New("Invalid bccsp identity. Must be different from nil.")
	}

	// we expect to have a valid VerifyOptions instance
	if msp.opts == nil {
		return nil, errors.New("Invalid msp instance")
	}

	// CAs cannot be directly used as identities..
	if id.cert.IsCA {
		return nil, errors.New("An X509 certificate with Basic Constraint: " +
			"Certificate Authority equals true cannot be used as an identity")
	}

	return msp.getValidationChain(id.cert, false)
}

func (msp *bccspmsp) getUniqueValidationChain(cert *x509.Certificate, opts x509.VerifyOptions) ([]*x509.Certificate, error) {
	// ask golang to validate the cert for us based on the options that we've built at setup time
	if msp.opts == nil {
		return nil, errors.New("the supplied identity has no verify options")
	}
	validationChains, err := cert.Verify(opts)
	if err != nil {
		return nil, errors.WithMessage(err, "the supplied identity is not valid")
	}

	// we only support a single validation chain;
	// if there's more than one then there might
	// be unclarity about who owns the identity
	if len(validationChains) != 1 {
		return nil, errors.Errorf("this MSP only supports a single validation chain, got %d", len(validationChains))
	}

	// Make the additional verification checks that were done in Go 1.14.
	err = verifyLegacyNameConstraints(validationChains[0])
	if err != nil {
		return nil, errors.WithMessage(err, "the supplied identity is not valid")
	}

	return validationChains[0], nil
}

var (
	oidExtensionSubjectAltName  = asn1.ObjectIdentifier{2, 5, 29, 17}
	oidExtensionNameConstraints = asn1.ObjectIdentifier{2, 5, 29, 30}
)

// verifyLegacyNameConstraints exercises the name constraint validation rules
// that were part of the certificate verification process in Go 1.14.
//
// If a signing certificate contains a name constratint, the leaf certificate
// does not include SAN extensions, and the leaf's common name looks like a
// host name, the validation would fail with an x509.CertificateInvalidError
// and a rason of x509.NameConstraintsWithoutSANs.
func verifyLegacyNameConstraints(chain []*x509.Certificate) error {
	if len(chain) < 2 {
		return nil
	}

	// Leaf certificates with SANs are fine.
	if oidInExtensions(oidExtensionSubjectAltName, chain[0].Extensions) {
		return nil
	}
	// Leaf certificates without a hostname in CN are fine.
	if !validHostname(chain[0].Subject.CommonName) {
		return nil
	}
	// If an intermediate or root have a name constraint, validation
	// would fail in Go 1.14.
	for _, c := range chain[1:] {
		if oidInExtensions(oidExtensionNameConstraints, c.Extensions) {
			return x509.CertificateInvalidError{Cert: chain[0], Reason: x509.NameConstraintsWithoutSANs}
		}
	}
	return nil
}

func oidInExtensions(oid asn1.ObjectIdentifier, exts []pkix.Extension) bool {
	for _, ext := range exts {
		if ext.Id.Equal(oid) {
			return true
		}
	}
	return false
}

// validHostname reports whether host is a valid hostname that can be matched or
// matched against according to RFC 6125 2.2, with some leniency to accommodate
// legacy values.
//
// This implementation is sourced from the standard library.
func validHostname(host string) bool {
	host = strings.TrimSuffix(host, ".")

	if len(host) == 0 {
		return false
	}

	for i, part := range strings.Split(host, ".") {
		if part == "" {
			// Empty label.
			return false
		}
		if i == 0 && part == "*" {
			// Only allow full left-most wildcards, as those are the only ones
			// we match, and matching literal '*' characters is probably never
			// the expected behavior.
			continue
		}
		for j, c := range part {
			if 'a' <= c && c <= 'z' {
				continue
			}
			if '0' <= c && c <= '9' {
				continue
			}
			if 'A' <= c && c <= 'Z' {
				continue
			}
			if c == '-' && j != 0 {
				continue
			}
			if c == '_' || c == ':' {
				// Not valid characters in hostnames, but commonly
				// found in deployments outside the WebPKI.
				continue
			}
			return false
		}
	}

	return true
}

func (msp *bccspmsp) getValidationChain(cert *x509.Certificate, isIntermediateChain bool) ([]*x509.Certificate, error) {
	validationChain, err := msp.getUniqueValidationChain(cert, msp.getValidityOptsForCert(cert))
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting validation chain")
	}

	// we expect a chain of length at least 2
	if len(validationChain) < 2 {
		return nil, errors.Errorf("expected a chain of length at least 2, got %d", len(validationChain))
	}

	// check that the parent is a leaf of the certification tree
	// if validating an intermediate chain, the first certificate will the parent
	parentPosition := 1
	if isIntermediateChain {
		parentPosition = 0
	}
	if msp.certificationTreeInternalNodesMap[string(validationChain[parentPosition].Raw)] {
		return nil, errors.Errorf("invalid validation chain. Parent certificate should be a leaf of the certification tree [%v]", cert.Raw)
	}
	return validationChain, nil
}

// getCertificationChainIdentifier returns the certification chain identifier of the passed identity within this msp.
// The identifier is computes as the SHA256 of the concatenation of the certificates in the chain.
func (msp *bccspmsp) getCertificationChainIdentifier(id Identity) ([]byte, error) {
	chain, err := msp.getCertificationChain(id)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting certification chain for [%v]", id)
	}

	// chain[0] is the certificate representing the identity.
	// It will be discarded
	return msp.getCertificationChainIdentifierFromChain(chain[1:])
}

func (msp *bccspmsp) getCertificationChainIdentifierFromChain(chain []*x509.Certificate) ([]byte, error) {
	// Hash the chain
	// Use the hash of the identity's certificate as id in the IdentityIdentifier
	hashOpt, err := bccsp.GetHashOpt(msp.cryptoConfig.IdentityIdentifierHashFunction)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting hash function options")
	}

	hf, err := msp.bccsp.GetHash(hashOpt)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting hash function when computing certification chain identifier")
	}
	for i := 0; i < len(chain); i++ {
		hf.Write(chain[i].Raw)
	}
	return hf.Sum(nil), nil
}

// sanitizeCert ensures that x509 certificates signed using ECDSA
// do have signatures in Low-S. If this is not the case, the certificate
// is regenerated to have a Low-S signature.
func (msp *bccspmsp) sanitizeCert(cert *x509.Certificate) (*x509.Certificate, error) {
	var err error
	if isECDSASignedCert(cert) {
		isRootCACert := false
		validityOpts := msp.getValidityOptsForCert(cert)
		if cert.IsCA && cert.CheckSignatureFrom(cert) == nil {
			// this is a root CA we can already sanitize it
			cert, err = sanitizeECDSASignedCert(cert, cert)
			if err != nil {
				return nil, err
			}
			isRootCACert = true
			validityOpts.Roots = x509.NewCertPool()
			validityOpts.Roots.AddCert(cert)
		}
		// Lookup for a parent certificate to perform the sanitization
		// run cert validation at any rate, if this is a root CA
		// we will validate already sanitized cert
		chain, err := msp.getUniqueValidationChain(cert, validityOpts)
		if err != nil {
			return nil, err
		}

		// once we finish validation and this is already
		// sanitized certificate, there is no need to
		// sanitize it once again hence we can just return it
		if isRootCACert {
			return cert, nil
		}

		// ok, this is no a root CA cert, and now we
		// then we have chain of certs and can get parent
		// to sanitize the cert whenever it's intermediate or leaf certificate
		parentCert := chain[1]

		// Sanitize
		return sanitizeECDSASignedCert(cert, parentCert)
	}
	return cert, nil
}

// IsWellFormed checks if the given identity can be deserialized into its provider-specific form.
// In this MSP implementation, well formed means that the PEM has a Type which is either
// the string 'CERTIFICATE' or the Type is missing altogether.
func (msp *bccspmsp) IsWellFormed(identity *m.SerializedIdentity) error {
	bl, rest := pem.Decode(identity.IdBytes)
	if bl == nil {
		return errors.New("PEM decoding resulted in an empty block")
	}
	if len(rest) > 0 {
		return errors.Errorf("identity %s for MSP %s has trailing bytes", string(identity.IdBytes), identity.Mspid)
	}

	// Important: This method looks very similar to getCertFromPem(idBytes []byte) (*x509.Certificate, error)
	// But we:
	// 1) Must ensure PEM block is of type CERTIFICATE or is empty
	// 2) Must not replace getCertFromPem with this method otherwise we will introduce
	//    a change in validation logic which will result in a chain fork.
	if bl.Type != "CERTIFICATE" && bl.Type != "" {
		return errors.Errorf("pem type is %s, should be 'CERTIFICATE' or missing", bl.Type)
	}
	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return err
	}

	if !isECDSASignedCert(cert) {
		return nil
	}

	return isIdentitySignedInCanonicalForm(cert.Signature, identity.Mspid, identity.IdBytes)
}

func isIdentitySignedInCanonicalForm(sig []byte, mspID string, pemEncodedIdentity []byte) error {
	r, s, err := utils.UnmarshalECDSASignature(sig)
	if err != nil {
		return err
	}

	expectedSig, err := utils.MarshalECDSASignature(r, s)
	if err != nil {
		return err
	}

	if !bytes.Equal(expectedSig, sig) {
		return errors.Errorf("identity %s for MSP %s has a non canonical signature",
			string(pemEncodedIdentity), mspID)
	}

	return nil
}
