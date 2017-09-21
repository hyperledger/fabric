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

func setup(configPath string) (MSP, error) {
	msp, err := newIdemixMsp()
	if err != nil {
		return nil, err
	}

	conf, err := GetIdemixMspConfig(configPath)
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
	msp, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)

	assert.Equal(t, IDEMIX, msp.GetType())
}

func TestSetupBad(t *testing.T) {
	_, err := setup("testdata/idemix/badpath")
	assert.Error(t, err)

	msp1, err := newIdemixMsp()
	assert.NoError(t, err)

	// Setup with nil config
	err = msp1.Setup(nil)
	assert.Error(t, err)

	// Setup with incorrect MSP type
	conf := &msp.MSPConfig{1234, nil}
	err = msp1.Setup(conf)
	assert.Error(t, err)

	// Setup with bad idemix config bytes
	conf = &msp.MSPConfig{int32(IDEMIX), []byte("barf")}
	err = msp1.Setup(conf)
	assert.Error(t, err)

	conf, err = GetIdemixMspConfig("testdata/idemix/MSP1OU1")
	idemixconfig := &msp.IdemixMSPConfig{}
	err = proto.Unmarshal(conf.Config, idemixconfig)
	assert.NoError(t, err)

	// Create MSP config with IPK with incorrect attribute names
	rng, err := idemix.GetRand()
	assert.NoError(t, err)
	key, err := idemix.NewIssuerKey([]string{}, rng)
	assert.NoError(t, err)
	ipkBytes, err := proto.Marshal(key.IPk)
	assert.NoError(t, err)
	idemixconfig.IPk = ipkBytes

	idemixConfigBytes, err := proto.Marshal(idemixconfig)
	assert.NoError(t, err)
	conf.Config = idemixConfigBytes

	err = msp1.Setup(conf)
	assert.Error(t, err)

	// Create MSP config with bad IPK bytes
	ipkBytes = []byte("barf")
	idemixconfig.IPk = ipkBytes

	idemixConfigBytes, err = proto.Marshal(idemixconfig)
	assert.NoError(t, err)
	conf.Config = idemixConfigBytes

	err = msp1.Setup(conf)
	assert.Error(t, err)
}

func TestSigning(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1")
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

	verMsp, err := setup("testdata/idemix/MSP1Verifier")
	assert.NoError(t, err)
	err = verMsp.Validate(id)
	assert.NoError(t, err)
	_, err = verMsp.GetDefaultSigningIdentity()
	assert.Error(t, err)
}

func TestSigningBad(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)

	id, err := getDefaultSigner(msp)
	assert.NoError(t, err)

	msg := []byte("TestMessage")
	sig := []byte("barf")

	err = id.Verify(msg, sig)
	assert.Error(t, err)
}

func TestIdentitySerialization(t *testing.T) {
	msp, err := setup("testdata/idemix/MSP1OU1")
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
	msp, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)

	_, err = msp.DeserializeIdentity([]byte("barf"))
	assert.Error(t, err, "DeserializeIdentity should have failed for bad input")
}

func TestIdentitySerializationWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)
	msp2, err := setup("testdata/idemix/MSP2OU1")
	assert.NoError(t, err)
	id2, err := getDefaultSigner(msp2)
	assert.NoError(t, err)

	idBytes, err := id2.Serialize()
	assert.NoError(t, err)

	_, err = msp1.DeserializeIdentity(idBytes)
	assert.Error(t, err, "DeserializeIdentity should have failed for ID of other MSP")
}

func TestPrincipalIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
	msp1, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	msp2, err := setup("testdata/idemix/MSP1OU2")
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
}

func TestPrincipalIdentityBadIdentity(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	idBytes := []byte("barf")

	principal := &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               idBytes}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Identity MSP principal for a bad principal should fail")
}

func TestIdemixIsWellFormed(t *testing.T) {
	idemixMSP, err := setup("testdata/idemix/MSP1OU1")
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
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalOUWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalOUBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalRoleMember(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalRoleAdmin(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1Admin")
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

func TestPrincipalRoleNotAdmin(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalRoleWrongMSP(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalRoleBadRole(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
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
}

func TestPrincipalBad(t *testing.T) {
	msp1, err := setup("testdata/idemix/MSP1OU1")
	assert.NoError(t, err)

	id1, err := getDefaultSigner(msp1)
	assert.NoError(t, err)

	principal := &msp.MSPPrincipal{
		PrincipalClassification: 1234,
		Principal:               nil}

	err = id1.SatisfiesPrincipal(principal)
	assert.Error(t, err, "Principal with bad Classification should fail")
}
