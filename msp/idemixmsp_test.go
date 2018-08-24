/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/idemix"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func setup(configPath string, ID string) (MSP, error) {
	return setupWithVersion(configPath, ID, MSPv1_3)
}

func setupWithVersion(configPath string, ID string, version MSPVersion) (MSP, error) {
	msp, err := newIdemixMsp(version)
	if err != nil {
		return nil, err
	}

	conf, err := GetIdemixMspConfig(configPath, ID)
	if err != nil {
		return nil, errors.Wrap(err, "Getting MSP config failed")
	}

	err = msp.Setup(conf)
	if err != nil {
		return nil, errors.Wrap(err, "Setting up MSP failed")
	}
	return msp, nil
}

func getDefaultSigner(msp MSP) (SigningIdentity, error) {
	id, err := msp.GetDefaultSigningIdentity()
	if err != nil {
		return nil, errors.Wrap(err, "Getting default signing identity failed")
	}

	err = id.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "Default signing identity invalid")
	}

	err = msp.Validate(id)
	if err != nil {
		return nil, errors.Wrap(err, "Default signing identity invalid")
	}

	return id, nil
}

func TestSetup(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	assert.Equal(t, IDEMIX, msp.GetType())
}

func TestSetupBad(t *testing.T) {
	_, err := setup("testdata/idemix/badpath", "MSPID")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Getting MSP config failed")

	msp1, err := newIdemixMsp(MSPv1_3)
	assert.NoError(t, err)

	// Setup with nil config
	err = msp1.Setup(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "setup error: nil conf reference")

	// Setup with incorrect MSP type
	conf := &msp.MSPConfig{Type: 1234, Config: nil}
	err = msp1.Setup(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "setup error: config is not of type IDEMIX")

	// Setup with bad idemix config bytes
	conf = &msp.MSPConfig{Type: int32(IDEMIX), Config: []byte("barf")}
	err = msp1.Setup(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed unmarshalling idemix msp config")

	conf, err = GetIdemixMspConfig("testdata/idemix/MSP1OU1", "IdemixMSP1")
	idemixconfig := &msp.IdemixMSPConfig{}
	err = proto.Unmarshal(conf.Config, idemixconfig)
	assert.NoError(t, err)

	// Create MSP config with IPK with incorrect attribute names
	rng, err := idemix.GetRand()
	assert.NoError(t, err)
	key, err := idemix.NewIssuerKey([]string{}, rng)
	assert.NoError(t, err)
	ipkBytes, err := proto.Marshal(key.Ipk)
	assert.NoError(t, err)
	idemixconfig.Ipk = ipkBytes

	idemixConfigBytes, err := proto.Marshal(idemixconfig)
	assert.NoError(t, err)
	conf.Config = idemixConfigBytes

	err = msp1.Setup(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issuer public key must have have attributes OU, Role, EnrollmentId, and RevocationHandle")

	// Create MSP config with bad IPK bytes
	ipkBytes = []byte("barf")
	idemixconfig.Ipk = ipkBytes

	idemixConfigBytes, err = proto.Marshal(idemixconfig)
	assert.NoError(t, err)
	conf.Config = idemixConfigBytes

	err = msp1.Setup(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal ipk from idemix msp config")
}

func TestSigning(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1")
	assert.NoError(t, err)

	id, err := getDefaultSigner(msp)
	assert.NoError(t, err)

	msg := []byte("TestMessage")
	sig, err := id.Sign(msg)
	assert.NoError(t, err)

	err = id.Verify(msg, sig)
	assert.NoError(t, err)

	err = id.Verify([]byte("OtherMessage"), sig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pseudonym signature invalid: zero-knowledge proof is invalid")

	verMsp, err := setup("testdata/idemix/MSP1Verifier", "MSP1")
	assert.NoError(t, err)
	err = verMsp.Validate(id)
	assert.NoError(t, err)
	_, err = verMsp.GetDefaultSigningIdentity()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no default signer setup")
}

func TestSigningBad(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id, err := getDefaultSigner(msp)
	assert.NoError(t, err)

	msg := []byte("TestMessage")
	sig := []byte("barf")

	err = id.Verify(msg, sig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshalling signature")
}

func TestIdentitySerialization(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id, err := getDefaultSigner(msp)
	assert.NoError(t, err)

	// Test serialization of identities
	serializedID, err := id.Serialize()
	assert.NoError(t, err)

	verID, err := msp.DeserializeIdentity(serializedID)

	err = verID.Validate()
	assert.NoError(t, err)

	err = msp.Validate(verID)
	assert.NoError(t, err)
}

func TestIdentitySerializationBad(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	_, err = msp.DeserializeIdentity([]byte("barf"))
	assert.Error(t, err, "DeserializeIdentity should have failed for bad input")
	assert.Contains(t, err.Error(), "could not deserialize a SerializedIdentity")
}

func TestIdentitySerializationWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)
	msp2, err := setup("testdata/idemix/MSP2OU1", "MSP2OU1")
	assert.NoError(t, err)
	id2, err := getDefaultSigner(msp2)
	assert.NoError(t, err)

	idBytes, err := id2.Serialize()
	assert.NoError(t, err)

	_, err = msp1.DeserializeIdentity(idBytes)
	assert.Error(t, err, "DeserializeIdentity should have failed for ID of other MSP")
	assert.Contains(t, err.Error(), "expected MSP ID MSP1OU1, received MSP2OU1")
}

func TestPrincipalIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	idBytes, err := id1.Serialize()
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)
}

func TestPrincipalIdentityWrongIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	msp2, err := setup("testdata/idemix/MSP1OU2", "MSP1OU2")
	assert.NoError(t, err)

	id2, err := getDefaultSigner(msp2)
	assert.NoError(t, err)

	idBytes, err := id1.Serialize()
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes}

	err = id2.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Identity MSP principal for different user should fail")
	assert.Contains(t, err.Error(), "the identities do not match")

}

func TestPrincipalIdentityBadIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	idBytes := []byte("barf")

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Identity MSP principal for a bad principal should fail")
	assert.Contains(t, err.Error(), "the identities do not match")
}

func TestAnonymityPrincipal(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_ANONYMOUS})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ANONYMITY,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)
}

func TestAnonymityPrincipalBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_NOMINAL})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ANONYMITY,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Idemix identity is anonymous and should not pass NOMINAL anonymity principal")
	assert.Contains(t, err.Error(), "principal is nominal, but idemix MSP is anonymous")
}

func TestAnonymityPrincipalV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_NOMINAL})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ANONYMITY,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Anonymity MSP Principals are unsupported in MSPv1_1")
}

func TestIdemixIsWellFormed(t *testing.T) {
	idemixMSP, err := setup("testdata/idemix/MSP1OU1", "TestName")
	assert.NoError(t, err)

	id, err := getDefaultSigner(idemixMSP)
	assert.NoError(t, err)
	rawId, err := id.Serialize()
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{}
	err = proto.Unmarshal(rawId, sId)
	assert.NoError(t, err)
	err = idemixMSP.IsWellFormed(sId)
	assert.NoError(t, err)
	// Corrupt the identity bytes
	sId.IdBytes = append(sId.IdBytes, 1)
	err = idemixMSP.IsWellFormed(sId)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not an idemix identity")
}

func TestPrincipalOU(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	bytes, err := proto.Marshal(ou)
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)
}

func TestPrincipalOUWrongOU(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: "DifferentOU",
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	bytes, err := proto.Marshal(ou)
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "OU MSP principal should have failed for user of different OU")
	assert.Contains(t, err.Error(), "user is not part of the desired organizational unit")

}

func TestPrincipalOUWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: "OU1",
		MspIdentifier:                "OtherMSP",
		CertifiersIdentifier:         nil,
	}
	bytes, err := proto.Marshal(ou)
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "OU MSP principal should have failed for user of different MSP")
	assert.Contains(t, err.Error(), "the identity is a member of a different MSP")

}

func TestPrincipalOUBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	bytes := []byte("barf")
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "OU MSP principal should have failed for a bad OU principal")
	assert.Contains(t, err.Error(), "could not unmarshal OU from principal")
}

func TestPrincipalRoleMember(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)

	// Member should also satisfy client
	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal = &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)
}

func TestPrincipalRoleAdmin(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1Admin", "MSP1OU1Admin")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	// Admin should also satisfy member
	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal = &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.NoError(t, err)
}

func TestPrincipalRoleNotPeer(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1Admin", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Admin should not satisfy PEER principal")
	assert.Contains(t, err.Error(), "idemixmsp only supports client use, so it cannot satisfy an MSPRole PEER principal")
}

func TestPrincipalRoleNotAdmin(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Member should not satisfy Admin principal")
	assert.Contains(t, err.Error(), "user is not an admin")
}

func TestPrincipalRoleWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "OtherMSP"})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Role MSP principal should have failed for user of different MSP")
	assert.Contains(t, err.Error(), "the identity is a member of a different MSP")
}

func TestPrincipalRoleBadRole(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	// Make principal for nonexisting role 1234
	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: 1234, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Role MSP principal should have failed for a bad Role")
	assert.Contains(t, err.Error(), "invalid MSP role type")
}

func TestPrincipalBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: 1234,
		Principal:               nil}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Principal with bad Classification should fail")
	assert.Contains(t, err.Error(), "invalid principal type")
}

func TestPrincipalCombined(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	principalBytes, err := proto.Marshal(ou)
	assert.NoError(t, err)

	principalOU := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               principalBytes}

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	principals := []*msp.MSPPrincipal{principalOU, principalRole}

	combinedPrincipal := &msp.CombinedPrincipal{Principals: principals}
	combinedPrincipalBytes, err := proto.Marshal(combinedPrincipal)

	assert.NoError(t, err)

	principalsCombined := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED, Principal: combinedPrincipalBytes}

	err = id1.SatisfiesPrincipal(principalsCombined)
	assert.NoError(t, err)
}

func TestPrincipalCombinedBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	// create combined principal requiring membership of OU1 in MSP1 and requiring admin role
	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	principalBytes, err := proto.Marshal(ou)
	assert.NoError(t, err)

	principalOU := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               principalBytes}

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	principals := []*msp.MSPPrincipal{principalOU, principalRole}

	combinedPrincipal := &msp.CombinedPrincipal{Principals: principals}
	combinedPrincipalBytes, err := proto.Marshal(combinedPrincipal)

	assert.NoError(t, err)

	principalsCombined := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED, Principal: combinedPrincipalBytes}

	err = id1.SatisfiesPrincipal(principalsCombined)
	assert.Error(t, err, "non-admin member of OU1 in MSP1 should not satisfy principal admin and OU1 in MSP1")
	assert.Contains(t, err.Error(), "user is not an admin")
}

func TestPrincipalCombinedV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	principalBytes, err := proto.Marshal(ou)
	assert.NoError(t, err)

	principalOU := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               principalBytes}

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)

	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}

	principals := []*msp.MSPPrincipal{principalOU, principalRole}

	combinedPrincipal := &msp.CombinedPrincipal{Principals: principals}
	combinedPrincipalBytes, err := proto.Marshal(combinedPrincipal)

	assert.NoError(t, err)

	principalsCombined := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED, Principal: combinedPrincipalBytes}

	err = id1.SatisfiesPrincipal(principalsCombined)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Combined MSP Principals are unsupported in MSPv1_1")
}

func TestRoleClientV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	assert.NoError(t, err)
	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)
	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}
	err = id1.SatisfiesPrincipal(principalRole)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid MSP role type")
}

func TestRolePeerV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	assert.NoError(t, err)
	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: id1.GetMSPIdentifier()})
	assert.NoError(t, err)
	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes}
	err = id1.SatisfiesPrincipal(principalRole)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid MSP role type")
}
