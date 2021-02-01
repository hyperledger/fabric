/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/idemix"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	require.Equal(t, IDEMIX, msp.GetType())
}

func TestSetupBad(t *testing.T) {
	_, err := setup("testdata/idemix/badpath", "MSPID")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Getting MSP config failed")

	msp1, err := newIdemixMsp(MSPv1_3)
	require.NoError(t, err)

	// Setup with nil config
	err = msp1.Setup(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "setup error: nil conf reference")

	// Setup with incorrect MSP type
	conf := &msp.MSPConfig{Type: 1234, Config: nil}
	err = msp1.Setup(conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "setup error: config is not of type IDEMIX")

	// Setup with bad idemix config bytes
	conf = &msp.MSPConfig{Type: int32(IDEMIX), Config: []byte("barf")}
	err = msp1.Setup(conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmarshalling idemix msp config")

	conf, err = GetIdemixMspConfig("testdata/idemix/MSP1OU1", "IdemixMSP1")
	require.NoError(t, err)
	idemixconfig := &msp.IdemixMSPConfig{}
	err = proto.Unmarshal(conf.Config, idemixconfig)
	require.NoError(t, err)

	// Create MSP config with IPK with incorrect attribute names
	rng, err := idemix.GetRand()
	require.NoError(t, err)
	key, err := idemix.NewIssuerKey([]string{}, rng)
	require.NoError(t, err)
	ipkBytes, err := proto.Marshal(key.Ipk)
	require.NoError(t, err)
	idemixconfig.Ipk = ipkBytes

	idemixConfigBytes, err := proto.Marshal(idemixconfig)
	require.NoError(t, err)
	conf.Config = idemixConfigBytes

	err = msp1.Setup(conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "issuer public key must have have attributes OU, Role, EnrollmentId, and RevocationHandle")

	// Create MSP config with bad IPK bytes
	ipkBytes = []byte("barf")
	idemixconfig.Ipk = ipkBytes

	idemixConfigBytes, err = proto.Marshal(idemixconfig)
	require.NoError(t, err)
	conf.Config = idemixConfigBytes

	err = msp1.Setup(conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal ipk from idemix msp config")
}

func TestSigning(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1")
	require.NoError(t, err)

	id, err := getDefaultSigner(msp)
	require.NoError(t, err)

	msg := []byte("TestMessage")
	sig, err := id.Sign(msg)
	require.NoError(t, err)

	err = id.Verify(msg, sig)
	require.NoError(t, err)

	err = id.Verify([]byte("OtherMessage"), sig)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pseudonym signature invalid: zero-knowledge proof is invalid")

	verMsp, err := setup("testdata/idemix/MSP1Verifier", "MSP1")
	require.NoError(t, err)
	err = verMsp.Validate(id)
	require.NoError(t, err)
	_, err = verMsp.GetDefaultSigningIdentity()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no default signer setup")
}

func TestSigningBad(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id, err := getDefaultSigner(msp)
	require.NoError(t, err)

	msg := []byte("TestMessage")
	sig := []byte("barf")

	err = id.Verify(msg, sig)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error unmarshalling signature")
}

func TestIdentitySerialization(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id, err := getDefaultSigner(msp)
	require.NoError(t, err)

	// Test serialization of identities
	serializedID, err := id.Serialize()
	require.NoError(t, err)

	verID, err := msp.DeserializeIdentity(serializedID)
	require.NoError(t, err)

	err = verID.Validate()
	require.NoError(t, err)

	err = msp.Validate(verID)
	require.NoError(t, err)
}

func TestIdentitySerializationBad(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	_, err = msp.DeserializeIdentity([]byte("barf"))
	require.Error(t, err, "DeserializeIdentity should have failed for bad input")
	require.Contains(t, err.Error(), "could not deserialize a SerializedIdentity")
}

func TestIdentitySerializationWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)
	msp2, err := setup("testdata/idemix/MSP2OU1", "MSP2OU1")
	require.NoError(t, err)
	id2, err := getDefaultSigner(msp2)
	require.NoError(t, err)

	idBytes, err := id2.Serialize()
	require.NoError(t, err)

	_, err = msp1.DeserializeIdentity(idBytes)
	require.Error(t, err, "DeserializeIdentity should have failed for ID of other MSP")
	require.Contains(t, err.Error(), "expected MSP ID MSP1OU1, received MSP2OU1")
}

func TestPrincipalIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	idBytes, err := id1.Serialize()
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestPrincipalIdentityWrongIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	msp2, err := setup("testdata/idemix/MSP1OU2", "MSP1OU2")
	require.NoError(t, err)

	id2, err := getDefaultSigner(msp2)
	require.NoError(t, err)

	idBytes, err := id1.Serialize()
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes,
	}

	err = id2.SatisfiesPrincipal(principal)
	require.Error(t, err, "Identity MSP principal for different user should fail")
	require.Contains(t, err.Error(), "the identities do not match")
}

func TestPrincipalIdentityBadIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	idBytes := []byte("barf")

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Identity MSP principal for a bad principal should fail")
	require.Contains(t, err.Error(), "the identities do not match")
}

func TestAnonymityPrincipal(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_ANONYMOUS})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ANONYMITY,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestAnonymityPrincipalBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_NOMINAL})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ANONYMITY,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Idemix identity is anonymous and should not pass NOMINAL anonymity principal")
	require.Contains(t, err.Error(), "principal is nominal, but idemix MSP is anonymous")
}

func TestAnonymityPrincipalV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_NOMINAL})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ANONYMITY,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Anonymity MSP Principals are unsupported in MSPv1_1")
}

func TestIdemixIsWellFormed(t *testing.T) {
	idemixMSP, err := setup("testdata/idemix/MSP1OU1", "TestName")
	require.NoError(t, err)

	id, err := getDefaultSigner(idemixMSP)
	require.NoError(t, err)
	rawId, err := id.Serialize()
	require.NoError(t, err)
	sId := &msp.SerializedIdentity{}
	err = proto.Unmarshal(rawId, sId)
	require.NoError(t, err)
	err = idemixMSP.IsWellFormed(sId)
	require.NoError(t, err)
	// Corrupt the identity bytes
	sId.IdBytes = append(sId.IdBytes, 1)
	err = idemixMSP.IsWellFormed(sId)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an idemix identity")
}

func TestPrincipalOU(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	bytes, err := proto.Marshal(ou)
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestPrincipalOUWrongOU(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: "DifferentOU",
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	bytes, err := proto.Marshal(ou)
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "OU MSP principal should have failed for user of different OU")
	require.Contains(t, err.Error(), "user is not part of the desired organizational unit")
}

func TestPrincipalOUWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: "OU1",
		MspIdentifier:                "OtherMSP",
		CertifiersIdentifier:         nil,
	}
	bytes, err := proto.Marshal(ou)
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "OU MSP principal should have failed for user of different MSP")
	require.Contains(t, err.Error(), "the identity is a member of a different MSP")
}

func TestPrincipalOUBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	bytes := []byte("barf")
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               bytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "OU MSP principal should have failed for a bad OU principal")
	require.Contains(t, err.Error(), "could not unmarshal OU from principal")
}

func TestPrincipalRoleMember(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)

	// Member should also satisfy client
	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal = &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestPrincipalRoleAdmin(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1Admin", "MSP1OU1Admin")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	// Admin should also satisfy member
	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal = &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.NoError(t, err)
}

func TestPrincipalRoleNotPeer(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1Admin", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Admin should not satisfy PEER principal")
	require.Contains(t, err.Error(), "idemixmsp only supports client use, so it cannot satisfy an MSPRole PEER principal")
}

func TestPrincipalRoleNotAdmin(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Member should not satisfy Admin principal")
	require.Contains(t, err.Error(), "user is not an admin")
}

func TestPrincipalRoleWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "OtherMSP"})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Role MSP principal should have failed for user of different MSP")
	require.Contains(t, err.Error(), "the identity is a member of a different MSP")
}

func TestPrincipalRoleBadRole(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	// Make principal for nonexisting role 1234
	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: 1234, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Role MSP principal should have failed for a bad Role")
	require.Contains(t, err.Error(), "invalid MSP role type")
}

func TestPrincipalBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: 1234,
		Principal:               nil,
	}

	err = id1.SatisfiesPrincipal(principal)
	require.Error(t, err, "Principal with bad Classification should fail")
	require.Contains(t, err.Error(), "invalid principal type")
}

func TestPrincipalCombined(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	principalBytes, err := proto.Marshal(ou)
	require.NoError(t, err)

	principalOU := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               principalBytes,
	}

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	principals := []*msp.MSPPrincipal{principalOU, principalRole}

	combinedPrincipal := &msp.CombinedPrincipal{Principals: principals}
	combinedPrincipalBytes, err := proto.Marshal(combinedPrincipal)

	require.NoError(t, err)

	principalsCombined := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED, Principal: combinedPrincipalBytes}

	err = id1.SatisfiesPrincipal(principalsCombined)
	require.NoError(t, err)
}

func TestPrincipalCombinedBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1", "MSP1OU1")
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	// create combined principal requiring membership of OU1 in MSP1 and requiring admin role
	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	principalBytes, err := proto.Marshal(ou)
	require.NoError(t, err)

	principalOU := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               principalBytes,
	}

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_ADMIN, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	principals := []*msp.MSPPrincipal{principalOU, principalRole}

	combinedPrincipal := &msp.CombinedPrincipal{Principals: principals}
	combinedPrincipalBytes, err := proto.Marshal(combinedPrincipal)

	require.NoError(t, err)

	principalsCombined := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED, Principal: combinedPrincipalBytes}

	err = id1.SatisfiesPrincipal(principalsCombined)
	require.Error(t, err, "non-admin member of OU1 in MSP1 should not satisfy principal admin and OU1 in MSP1")
	require.Contains(t, err.Error(), "user is not an admin")
}

func TestPrincipalCombinedV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	require.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	ou := &msp.OrganizationUnit{
		OrganizationalUnitIdentifier: id1.GetOrganizationalUnits()[0].OrganizationalUnitIdentifier,
		MspIdentifier:                id1.GetMSPIdentifier(),
		CertifiersIdentifier:         nil,
	}
	principalBytes, err := proto.Marshal(ou)
	require.NoError(t, err)

	principalOU := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               principalBytes,
	}

	principalBytes, err = proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)

	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}

	principals := []*msp.MSPPrincipal{principalOU, principalRole}

	combinedPrincipal := &msp.CombinedPrincipal{Principals: principals}
	combinedPrincipalBytes, err := proto.Marshal(combinedPrincipal)

	require.NoError(t, err)

	principalsCombined := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED, Principal: combinedPrincipalBytes}

	err = id1.SatisfiesPrincipal(principalsCombined)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Combined MSP Principals are unsupported in MSPv1_1")
}

func TestRoleClientV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	require.NoError(t, err)
	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_CLIENT, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)
	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	err = id1.SatisfiesPrincipal(principalRole)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid MSP role type")
}

func TestRolePeerV11(t *testing.T) {
	msp1, err := setupWithVersion("testdata/idemix/MSP1OU1", "MSP1OU1", MSPv1_1)
	require.NoError(t, err)
	id1, err := getDefaultSigner(msp1)
	require.NoError(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: id1.GetMSPIdentifier()})
	require.NoError(t, err)
	principalRole := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	err = id1.SatisfiesPrincipal(principalRole)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid MSP role type")
}
