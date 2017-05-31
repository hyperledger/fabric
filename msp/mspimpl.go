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

package msp

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/signer"
	m "github.com/hyperledger/fabric/protos/msp"
)

// This is an instantiation of an MSP that
// uses BCCSP for its cryptographic primitives.
type bccspmsp struct {
	// list of CA certs we trust
	rootCerts []Identity

	// list of intermediate certs we trust
	intermediateCerts []Identity

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
}

// NewBccspMsp returns an MSP instance backed up by a BCCSP
// crypto provider. It handles x.509 certificates and can
// generate identities and signing identities backed by
// certificates and keypairs
func NewBccspMsp() (MSP, error) {
	mspLogger.Debugf("Creating BCCSP-based MSP instance")

	bccsp := factory.GetDefault()
	theMsp := &bccspmsp{}
	theMsp.bccsp = bccsp

	return theMsp, nil
}

func (msp *bccspmsp) getCertFromPem(idBytes []byte) (*x509.Certificate, error) {
	if idBytes == nil {
		return nil, fmt.Errorf("getIdentityFromConf error: nil idBytes")
	}

	// Decode the pem bytes
	pemCert, _ := pem.Decode(idBytes)
	if pemCert == nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: could not decode pem bytes [%v]", idBytes)
	}

	// get a cert
	var cert *x509.Certificate
	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: failed to parse x509 cert, err %s", err)
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

	// Use the hash of the identity's certificate as id in the IdentityIdentifier
	hashOpt, err := bccsp.GetHashOpt(msp.cryptoConfig.IdentityIdentifierHashFunction)
	if err != nil {
		return nil, nil, fmt.Errorf("getIdentityFromConf failed getting hash function options [%s]", err)
	}

	digest, err := msp.bccsp.Hash(cert.Raw, hashOpt)
	if err != nil {
		return nil, nil, fmt.Errorf("getIdentityFromConf failed hashing raw certificate to compute the id of the IdentityIdentifier [%s]", err)
	}

	id := &IdentityIdentifier{
		Mspid: msp.name,
		Id:    hex.EncodeToString(digest)}

	mspId, err := newIdentity(id, cert, certPubK, msp)
	if err != nil {
		return nil, nil, err
	}

	return mspId, certPubK, nil
}

func (msp *bccspmsp) getSigningIdentityFromConf(sidInfo *m.SigningIdentityInfo) (SigningIdentity, error) {
	if sidInfo == nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: nil sidInfo")
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
		mspLogger.Debugf("Could not find SKI [%s], trying KeyMaterial field: %s\n", hex.EncodeToString(pubKey.SKI()), err)
		if sidInfo.PrivateSigner == nil || sidInfo.PrivateSigner.KeyMaterial == nil {
			return nil, fmt.Errorf("KeyMaterial not found in SigningIdentityInfo")
		}

		pemKey, _ := pem.Decode(sidInfo.PrivateSigner.KeyMaterial)
		privKey, err = msp.bccsp.KeyImport(pemKey.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
		if err != nil {
			return nil, fmt.Errorf("getIdentityFromBytes error: Failed to import EC private key, err %s", err)
		}
	}

	// get the peer signer
	peerSigner, err := signer.New(msp.bccsp, privKey)
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: Failed initializing bccspCryptoSigner, err %s", err)
	}

	// Use the hash of the identity's certificate as id in the IdentityIdentifier
	hashOpt, err := bccsp.GetHashOpt(msp.cryptoConfig.IdentityIdentifierHashFunction)
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes failed getting hash function options [%s]", err)
	}

	digest, err := msp.bccsp.Hash(idPub.(*identity).cert.Raw, hashOpt)
	if err != nil {
		return nil, fmt.Errorf("Failed hashing raw certificate to compute the id of the IdentityIdentifier [%s]", err)
	}

	id := &IdentityIdentifier{
		Mspid: msp.name,
		Id:    hex.EncodeToString(digest)}

	return newSigningIdentity(id, idPub.(*identity).cert, idPub.(*identity).pk, peerSigner, msp)
}

/*
   This is the definition of the ASN.1 marshalling of AuthorityKeyIdentifier
   from https://www.ietf.org/rfc/rfc5280.txt

   AuthorityKeyIdentifier ::= SEQUENCE {
      keyIdentifier             [0] KeyIdentifier           OPTIONAL,
      authorityCertIssuer       [1] GeneralNames            OPTIONAL,
      authorityCertSerialNumber [2] CertificateSerialNumber OPTIONAL  }

   KeyIdentifier ::= OCTET STRING

   CertificateSerialNumber  ::=  INTEGER

*/

type authorityKeyIdentifier struct {
	KeyIdentifier             []byte  `asn1:"optional,tag:0"`
	AuthorityCertIssuer       []byte  `asn1:"optional,tag:1"`
	AuthorityCertSerialNumber big.Int `asn1:"optional,tag:2"`
}

// getAuthorityKeyIdentifierFromCrl returns the Authority Key Identifier
// for the supplied CRL. The authority key identifier can be used to identify
// the public key corresponding to the private key which was used to sign the CRL.
func getAuthorityKeyIdentifierFromCrl(crl *pkix.CertificateList) ([]byte, error) {
	aki := authorityKeyIdentifier{}

	for _, ext := range crl.TBSCertList.Extensions {
		// Authority Key Identifier is identified by the following ASN.1 tag
		// authorityKeyIdentifier (2 5 29 35) (see https://tools.ietf.org/html/rfc3280.html)
		if reflect.DeepEqual(ext.Id, asn1.ObjectIdentifier{2, 5, 29, 35}) {
			_, err := asn1.Unmarshal(ext.Value, &aki)
			if err != nil {
				return nil, fmt.Errorf("Failed to unmarshal AKI, error %s", err)
			}

			return aki.KeyIdentifier, nil
		}
	}

	return nil, errors.New("authorityKeyIdentifier not found in certificate")
}

// getSubjectKeyIdentifierFromCert returns the Subject Key Identifier for the supplied certificate
// Subject Key Identifier is an identifier of the public key of this certificate
func getSubjectKeyIdentifierFromCert(cert *x509.Certificate) ([]byte, error) {
	var SKI []byte

	for _, ext := range cert.Extensions {
		// Subject Key Identifier is identified by the following ASN.1 tag
		// subjectKeyIdentifier (2 5 29 14) (see https://tools.ietf.org/html/rfc3280.html)
		if reflect.DeepEqual(ext.Id, asn1.ObjectIdentifier{2, 5, 29, 14}) {
			_, err := asn1.Unmarshal(ext.Value, &SKI)
			if err != nil {
				return nil, fmt.Errorf("Failed to unmarshal Subject Key Identifier, err %s", err)
			}

			return SKI, nil
		}
	}

	return nil, errors.New("subjectKeyIdentifier not found in certificate")
}

// isCACert does a few checks on the certificate,
// assuming it's a CA; it returns true if all looks good
// and false otherwise
func isCACert(cert *x509.Certificate) bool {
	_, err := getSubjectKeyIdentifierFromCert(cert)
	if err != nil {
		return false
	}

	if !cert.IsCA {
		return false
	}

	return true
}

// Setup sets up the internal data structures
// for this MSP, given an MSPConfig ref; it
// returns nil in case of success or an error otherwise
func (msp *bccspmsp) Setup(conf1 *m.MSPConfig) error {
	if conf1 == nil {
		return fmt.Errorf("Setup error: nil conf reference")
	}

	// given that it's an msp of type fabric, extract the MSPConfig instance
	var conf m.FabricMSPConfig
	err := proto.Unmarshal(conf1.Config, &conf)
	if err != nil {
		return fmt.Errorf("Failed unmarshalling fabric msp config, err %s", err)
	}

	// set the name for this msp
	msp.name = conf.Name
	mspLogger.Debugf("Setting up MSP instance %s", msp.name)

	// setup crypto config
	msp.cryptoConfig = conf.CryptoConfig
	if msp.cryptoConfig == nil {
		// Move to defaults
		msp.cryptoConfig = &m.FabricCryptoConfig{
			SignatureHashFamily:            bccsp.SHA2,
			IdentityIdentifierHashFunction: bccsp.SHA256,
		}
		mspLogger.Debugf("CryptoConfig was nil. Move to defaults.")
	}
	if msp.cryptoConfig.SignatureHashFamily == "" {
		msp.cryptoConfig.SignatureHashFamily = bccsp.SHA2
		mspLogger.Debugf("CryptoConfig.SignatureHashFamily was nil. Move to defaults.")
	}
	if msp.cryptoConfig.IdentityIdentifierHashFunction == "" {
		msp.cryptoConfig.IdentityIdentifierHashFunction = bccsp.SHA256
		mspLogger.Debugf("CryptoConfig.IdentityIdentifierHashFunction was nil. Move to defaults.")
	}

	// make and fill the set of CA certs - we expect them to be there
	if len(conf.RootCerts) == 0 {
		return errors.New("Expected at least one CA certificate")
	}

	// pre-create the verify options with roots and intermediates.
	// This is needed to make certificate sanitation working.
	// Recall that sanitization is applied also to root CA and intermediate
	// CA certificates. After their sanitization is done, the opts
	// will be recreated using the sanitized certs.
	msp.opts = &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}
	for _, v := range conf.RootCerts {
		cert, err := msp.getCertFromPem(v)
		if err != nil {
			return err
		}
		msp.opts.Roots.AddCert(cert)
	}
	for _, v := range conf.IntermediateCerts {
		cert, err := msp.getCertFromPem(v)
		if err != nil {
			return err
		}
		msp.opts.Intermediates.AddCert(cert)
	}

	// Load root and intermediate CA identities
	// Recall that when an identity is created, its certificate gets sanitized
	msp.rootCerts = make([]Identity, len(conf.RootCerts))
	for i, trustedCert := range conf.RootCerts {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.rootCerts[i] = id
	}

	// make and fill the set of intermediate certs (if present)
	msp.intermediateCerts = make([]Identity, len(conf.IntermediateCerts))
	for i, trustedCert := range conf.IntermediateCerts {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.intermediateCerts[i] = id
	}

	// root CA and intermediate CA certificates are sanitized, they can be reimported
	msp.opts = &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}
	for _, id := range msp.rootCerts {
		msp.opts.Roots.AddCert(id.(*identity).cert)
	}
	for _, id := range msp.intermediateCerts {
		msp.opts.Intermediates.AddCert(id.(*identity).cert)
	}

	// make and fill the set of admin certs (if present)
	msp.admins = make([]Identity, len(conf.Admins))
	for i, admCert := range conf.Admins {
		id, _, err := msp.getIdentityFromConf(admCert)
		if err != nil {
			return err
		}

		msp.admins[i] = id
	}

	// setup the CRL (if present)
	msp.CRL = make([]*pkix.CertificateList, len(conf.RevocationList))
	for i, crlbytes := range conf.RevocationList {
		crl, err := x509.ParseCRL(crlbytes)
		if err != nil {
			return fmt.Errorf("Could not parse RevocationList, err %s", err)
		}

		// TODO: pre-verify the signature on the CRL and create a map
		//       of CA certs to respective CRLs so that later upon
		//       validation we can already look up the CRL given the
		//       chain of the certificate to be validated

		msp.CRL[i] = crl
	}

	// ensure that our CAs are properly formed and that they are valid
	for _, id := range append(append([]Identity{}, msp.rootCerts...), msp.intermediateCerts...) {
		if !isCACert(id.(*identity).cert) {
			return fmt.Errorf("CA Certificate did not have the Subject Key Identifier extension, (SN: %s)", id.(*identity).cert.SerialNumber)
		}

		if err := msp.validateCAIdentity(id.(*identity)); err != nil {
			return fmt.Errorf("CA Certificate is not valid, (SN: %s) [%s]", id.(*identity).cert.SerialNumber, err)
		}
	}

	// populate certificationTreeInternalNodesMap to mark the internal nodes of the
	// certification tree
	msp.certificationTreeInternalNodesMap = make(map[string]bool)
	for _, cert := range append([]Identity{}, msp.intermediateCerts...) {
		chain, err := msp.getUniqueValidationChain(cert.(*identity).cert)
		if err != nil {
			return fmt.Errorf("Failed getting validation chain, (SN: %s)", cert.(*identity).cert.SerialNumber)
		}

		// Recall chain[0] is cert.(*identity).cert so it does not count as a parent
		for i := 1; i < len(chain); i++ {
			msp.certificationTreeInternalNodesMap[string(chain[i].Raw)] = true
		}
	}

	// setup the signer (if present)
	if conf.SigningIdentity != nil {
		sid, err := msp.getSigningIdentityFromConf(conf.SigningIdentity)
		if err != nil {
			return err
		}

		msp.signer = sid
	}

	// setup the OUs
	if err := msp.setupOUs(conf); err != nil {
		return err
	}

	return nil
}

// GetType returns the type for this MSP
func (msp *bccspmsp) GetType() ProviderType {
	return FABRIC
}

// GetIdentifier returns the MSP identifier for this instance
func (msp *bccspmsp) GetIdentifier() (string, error) {
	return msp.name, nil
}

// GetRootCerts returns the root certificates for this MSP
func (msp *bccspmsp) GetRootCerts() []Identity {
	return msp.rootCerts
}

// GetIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *bccspmsp) GetIntermediateCerts() []Identity {
	return msp.intermediateCerts
}

// GetDefaultSigningIdentity returns the
// default signing identity for this MSP (if any)
func (msp *bccspmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Debugf("Obtaining default signing identity")

	if msp.signer == nil {
		return nil, fmt.Errorf("This MSP does not possess a valid default signing identity")
	}

	return msp.signer, nil
}

// GetSigningIdentity returns a specific signing
// identity identified by the supplied identifier
func (msp *bccspmsp) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	// TODO
	return nil, fmt.Errorf("No signing identity for %#v", identifier)
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
		return fmt.Errorf("Identity type not recognized")
	}
}

// DeserializeIdentity returns an Identity given the byte-level
// representation of a SerializedIdentity struct
func (msp *bccspmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	mspLogger.Infof("Obtaining identity")

	// We first deserialize to a SerializedIdentity to get the MSP ID
	sId := &m.SerializedIdentity{}
	err := proto.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, fmt.Errorf("Could not deserialize a SerializedIdentity, err %s", err)
	}

	if sId.Mspid != msp.name {
		return nil, fmt.Errorf("Expected MSP ID %s, received %s", msp.name, sId.Mspid)
	}

	return msp.deserializeIdentityInternal(sId.IdBytes)
}

// deserializeIdentityInternal returns an identity given its byte-level representation
func (msp *bccspmsp) deserializeIdentityInternal(serializedIdentity []byte) (Identity, error) {
	// This MSP will always deserialize certs this way
	bl, _ := pem.Decode(serializedIdentity)
	if bl == nil {
		return nil, fmt.Errorf("Could not decode the PEM structure")
	}
	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ParseCertificate failed %s", err)
	}

	// Now we have the certificate; make sure that its fields
	// (e.g. the Issuer.OU or the Subject.OU) match with the
	// MSP id that this MSP has; otherwise it might be an attack
	// TODO!
	// We can't do it yet because there is no standardized way
	// (yet) to encode the MSP ID into the x.509 body of a cert

	// Use the hash of the identity's certificate as id in the IdentityIdentifier
	hashOpt, err := bccsp.GetHashOpt(msp.cryptoConfig.IdentityIdentifierHashFunction)
	if err != nil {
		return nil, fmt.Errorf("Failed getting hash function options [%s]", err)
	}

	digest, err := msp.bccsp.Hash(cert.Raw, hashOpt)
	if err != nil {
		return nil, fmt.Errorf("Failed hashing raw certificate to compute the id of the IdentityIdentifier [%s]", err)
	}

	id := &IdentityIdentifier{
		Mspid: msp.name,
		Id:    hex.EncodeToString(digest)}

	pub, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("Failed to import certitifacate's public key [%s]", err)
	}

	return newIdentity(id, cert, pub, msp)
}

// SatisfiesPrincipal returns null if the identity matches the principal or an error otherwise
func (msp *bccspmsp) SatisfiesPrincipal(id Identity, principal *m.MSPPrincipal) error {
	switch principal.PrincipalClassification {
	// in this case, we have to check whether the
	// identity has a role in the msp - member or admin
	case m.MSPPrincipal_ROLE:
		// Principal contains the msp role
		mspRole := &m.MSPRole{}
		err := proto.Unmarshal(principal.Principal, mspRole)
		if err != nil {
			return fmt.Errorf("Could not unmarshal MSPRole from principal, err %s", err)
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if mspRole.MspIdentifier != msp.name {
			return fmt.Errorf("The identity is a member of a different MSP (expected %s, got %s)", mspRole.MspIdentifier, id.GetMSPIdentifier())
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
			for _, admincert := range msp.admins {
				if bytes.Equal(id.(*identity).cert.Raw, admincert.(*identity).cert.Raw) {
					return nil
				}
			}

			return errors.New("This identity is not an admin")
		default:
			return fmt.Errorf("Invalid MSP role type %d", int32(mspRole.Role))
		}
	case m.MSPPrincipal_IDENTITY:
		// in this case we have to deserialize the principal's identity
		// and compare it byte-by-byte with our cert
		principalId, err := msp.DeserializeIdentity(principal.Principal)
		if err != nil {
			return fmt.Errorf("Invalid identity principal, not a certificate. Error %s", err)
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
			return fmt.Errorf("Could not unmarshal OrganizationUnit from principal, err %s", err)
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if OU.MspIdentifier != msp.name {
			return fmt.Errorf("The identity is a member of a different MSP (expected %s, got %s)", OU.MspIdentifier, id.GetMSPIdentifier())
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
		return fmt.Errorf("Invalid principal type %d", int32(principal.PrincipalClassification))
	}
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
		return nil, fmt.Errorf("Identity type not recognized")
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
		return nil, errors.New("A CA certificate cannot be used directly by this MSP")
	}

	return msp.getValidationChain(id.cert, false)
}

func (msp *bccspmsp) getUniqueValidationChain(cert *x509.Certificate) ([]*x509.Certificate, error) {
	// ask golang to validate the cert for us based on the options that we've built at setup time
	if msp.opts == nil {
		return nil, fmt.Errorf("The supplied identity has no verify options")
	}
	validationChains, err := cert.Verify(msp.getValidityOptsForCert(cert))
	if err != nil {
		return nil, fmt.Errorf("The supplied identity is not valid, Verify() returned %s", err)
	}

	// we only support a single validation chain;
	// if there's more than one then there might
	// be unclarity about who owns the identity
	if len(validationChains) != 1 {
		return nil, fmt.Errorf("This MSP only supports a single validation chain, got %d", len(validationChains))
	}

	return validationChains[0], nil
}

func (msp *bccspmsp) getValidationChain(cert *x509.Certificate, isIntermediateChain bool) ([]*x509.Certificate, error) {
	validationChain, err := msp.getUniqueValidationChain(cert)
	if err != nil {
		return nil, fmt.Errorf("Failed getting validation chain %s", err)
	}

	// we expect a chain of length at least 2
	if len(validationChain) < 2 {
		return nil, fmt.Errorf("Expected a chain of length at least 2, got %d", len(validationChain))
	}

	// check that the parent is a leaf of the certification tree
	// if validating an intermediate chain, the first certificate will the parent
	parentPosition := 1
	if isIntermediateChain {
		parentPosition = 0
	}
	if msp.certificationTreeInternalNodesMap[string(validationChain[parentPosition].Raw)] {
		return nil, fmt.Errorf("Invalid validation chain. Parent certificate should be a leaf of the certification tree [%v].", cert.Raw)
	}
	return validationChain, nil
}

// getCertificationChainIdentifier returns the certification chain identifier of the passed identity within this msp.
// The identifier is computes as the SHA256 of the concatenation of the certificates in the chain.
func (msp *bccspmsp) getCertificationChainIdentifier(id Identity) ([]byte, error) {
	chain, err := msp.getCertificationChain(id)
	if err != nil {
		return nil, fmt.Errorf("Failed getting certification chain for [%v]: [%s]", id, err)
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
		return nil, fmt.Errorf("Failed getting hash function options [%s]", err)
	}

	hf, err := msp.bccsp.GetHash(hashOpt)
	if err != nil {
		return nil, fmt.Errorf("Failed getting hash function when computing certification chain identifier: [%s]", err)
	}
	for i := 0; i < len(chain); i++ {
		hf.Write(chain[i].Raw)
	}
	return hf.Sum(nil), nil
}

func (msp *bccspmsp) setupOUs(conf m.FabricMSPConfig) error {
	msp.ouIdentifiers = make(map[string][][]byte)
	for _, ou := range conf.OrganizationalUnitIdentifiers {

		// 1. check that certificate is registered in msp.rootCerts or msp.intermediateCerts
		cert, err := msp.getCertFromPem(ou.Certificate)
		if err != nil {
			return fmt.Errorf("Failed getting certificate for [%v]: [%s]", ou, err)
		}

		// 2. Sanitize it to ensure like for like comparison
		cert, err = msp.sanitizeCert(cert)
		if err != nil {
			return fmt.Errorf("sanitizeCert failed %s", err)
		}

		found := false
		root := false
		// Search among root certificates
		for _, v := range msp.rootCerts {
			if v.(*identity).cert.Equal(cert) {
				found = true
				root = true
				break
			}
		}
		if !found {
			// Search among root intermediate certificates
			for _, v := range msp.intermediateCerts {
				if v.(*identity).cert.Equal(cert) {
					found = true
					break
				}
			}
		}
		if !found {
			// Certificate not valid, reject configuration
			return fmt.Errorf("Failed adding OU. Certificate [%v] not in root or intermediate certs.", ou.Certificate)
		}

		// 3. get the certification path for it
		var certifiersIdentitifer []byte
		var chain []*x509.Certificate
		if root {
			chain = []*x509.Certificate{cert}
		} else {
			chain, err = msp.getValidationChain(cert, true)
			if err != nil {
				return fmt.Errorf("Failed computing validation chain for [%v]. [%s]", cert, err)
			}
		}

		// 4. compute the hash of the certification path
		certifiersIdentitifer, err = msp.getCertificationChainIdentifierFromChain(chain)
		if err != nil {
			return fmt.Errorf("Failed computing Certifiers Identifier for [%v]. [%s]", ou.Certificate, err)
		}

		// Check for duplicates
		found = false
		for _, id := range msp.ouIdentifiers[ou.OrganizationalUnitIdentifier] {
			if bytes.Equal(id, certifiersIdentitifer) {
				mspLogger.Warningf("Duplicate found in ou identifiers [%s, %v]", ou.OrganizationalUnitIdentifier, id)
				found = true
				break
			}
		}

		if !found {
			// No duplicates found, add it
			msp.ouIdentifiers[ou.OrganizationalUnitIdentifier] = append(
				msp.ouIdentifiers[ou.OrganizationalUnitIdentifier],
				certifiersIdentitifer,
			)
		}
	}

	return nil
}

// sanitizeCert ensures that x509 certificates signed using ECDSA
// do have signatures in Low-S. If this is not the case, the certificate
// is regenerated to have a Low-S signature.
func (msp *bccspmsp) sanitizeCert(cert *x509.Certificate) (*x509.Certificate, error) {
	if isECDSASignedCert(cert) {
		// Lookup for a parent certificate to perform the sanitization
		var parentCert *x509.Certificate
		if cert.IsCA {
			// at this point, cert might be a root CA certificate
			// or an intermediate CA certificate
			chain, err := msp.getUniqueValidationChain(cert)
			if err != nil {
				return nil, err
			}
			if len(chain) == 1 {
				// cert is a root CA certificate
				parentCert = cert
			} else {
				// cert is an intermediate CA certificate
				parentCert = chain[1]
			}
		} else {
			chain, err := msp.getUniqueValidationChain(cert)
			if err != nil {
				return nil, err
			}
			parentCert = chain[1]
		}

		// Sanitize
		var err error
		cert, err = sanitizeECDSASignedCert(cert, parentCert)
		if err != nil {
			return nil, err
		}
	}
	return cert, nil
}

func (msp *bccspmsp) validateIdentity(id *identity) error {
	validationChain, err := msp.getCertificationChainForBCCSPIdentity(id)
	if err != nil {
		return fmt.Errorf("Could not obtain certification chain, err %s", err)
	}

	err = msp.validateIdentityAgainstChain(id, validationChain)
	if err != nil {
		return fmt.Errorf("Could not validate identity against certification chain, err %s", err)
	}

	err = msp.validateIdentityOUs(id)
	if err != nil {
		return fmt.Errorf("Could not validate identity's OUs, err %s", err)
	}

	return nil
}

func (msp *bccspmsp) validateCAIdentity(id *identity) error {
	if !id.cert.IsCA {
		return errors.New("Only CA identities can be validated")
	}

	validationChain, err := msp.getUniqueValidationChain(id.cert)
	if err != nil {
		return fmt.Errorf("Could not obtain certification chain, err %s", err)
	}
	if len(validationChain) == 1 {
		// validationChain[0] is the root CA certificate
		return nil
	}

	return msp.validateIdentityAgainstChain(id, validationChain)
}

func (msp *bccspmsp) validateIdentityAgainstChain(id *identity, validationChain []*x509.Certificate) error {
	// here we know that the identity is valid; now we have to check whether it has been revoked

	// identify the SKI of the CA that signed this cert
	SKI, err := getSubjectKeyIdentifierFromCert(validationChain[1])
	if err != nil {
		return fmt.Errorf("Could not obtain Subject Key Identifier for signer cert, err %s", err)
	}

	// check whether one of the CRLs we have has this cert's
	// SKI as its AuthorityKeyIdentifier
	for _, crl := range msp.CRL {
		aki, err := getAuthorityKeyIdentifierFromCrl(crl)
		if err != nil {
			return fmt.Errorf("Could not obtain Authority Key Identifier for crl, err %s", err)
		}

		// check if the SKI of the cert that signed us matches the AKI of any of the CRLs
		if bytes.Equal(aki, SKI) {
			// we have a CRL, check whether the serial number is revoked
			for _, rc := range crl.TBSCertList.RevokedCertificates {
				if rc.SerialNumber.Cmp(id.cert.SerialNumber) == 0 {
					// We have found a CRL whose AKI matches the SKI of
					// the CA (root or intermediate) that signed the
					// certificate that is under validation. As a
					// precaution, we verify that said CA is also the
					// signer of this CRL.
					err = validationChain[1].CheckCRLSignature(crl)
					if err != nil {
						// the CA cert that signed the certificate
						// that is under validation did not sign the
						// candidate CRL - skip
						mspLogger.Warningf("Invalid signature over the identified CRL, error %s", err)
						continue
					}

					// A CRL also includes a time of revocation so that
					// the CA can say "this cert is to be revoked starting
					// from this time"; however here we just assume that
					// revocation applies instantaneously from the time
					// the MSP config is committed and used so we will not
					// make use of that field
					return errors.New("The certificate has been revoked")
				}
			}
		}
	}

	return nil
}

func (msp *bccspmsp) validateIdentityOUs(id *identity) error {
	// Check that the identity's OUs are compatible with those recognized by this MSP,
	// meaning that the intersection is not empty.
	if len(msp.ouIdentifiers) > 0 {
		found := false

		for _, OU := range id.GetOrganizationalUnits() {
			certificationIDs, exists := msp.ouIdentifiers[OU.OrganizationalUnitIdentifier]

			if exists {
				for _, certificationID := range certificationIDs {
					if bytes.Equal(certificationID, OU.CertifiersIdentifier) {
						found = true
						break
					}
				}
			}
		}

		if !found {
			if len(id.GetOrganizationalUnits()) == 0 {
				return fmt.Errorf("The identity certificate does not contain an Organizational Unit (OU)")
			}
			return fmt.Errorf("None of the identity's organizational units [%v] are in MSP %s", id.GetOrganizationalUnits(), msp.name)
		}
	}

	return nil
}

func (msp *bccspmsp) getValidityOptsForCert(cert *x509.Certificate) x509.VerifyOptions {
	// First copy the opts to override the CurrentTime field
	// in order to make the certificate passing the expiration test
	// independently from the real local current time.
	// This is a temporary workaround for FAB-3678

	var tempOpts x509.VerifyOptions
	tempOpts.Roots = msp.opts.Roots
	tempOpts.DNSName = msp.opts.DNSName
	tempOpts.Intermediates = msp.opts.Intermediates
	tempOpts.KeyUsages = msp.opts.KeyUsages
	tempOpts.CurrentTime = cert.NotBefore.Add(time.Second)

	return tempOpts
}
