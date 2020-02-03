/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/msp"
	pb "github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func createCollectionPolicyConfig(accessPolicy *pb.SignaturePolicyEnvelope) *pb.CollectionPolicyConfig {
	cpcSp := &pb.CollectionPolicyConfig_SignaturePolicy{
		SignaturePolicy: accessPolicy,
	}
	cpc := &pb.CollectionPolicyConfig{
		Payload: cpcSp,
	}
	return cpc
}

type mockIdentity struct {
	idBytes []byte
}

func (id *mockIdentity) Anonymous() bool {
	panic("implement me")
}

func (id *mockIdentity) ExpiresAt() time.Time {
	return time.Time{}
}

func (id *mockIdentity) SatisfiesPrincipal(p *mb.MSPPrincipal) error {
	if bytes.Compare(id.idBytes, p.Principal) == 0 {
		return nil
	}
	return errors.New("Principals do not match")
}

func (id *mockIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "Mock", Id: string(id.idBytes)}
}

func (id *mockIdentity) GetMSPIdentifier() string {
	return string(id.idBytes)
}

func (id *mockIdentity) Validate() error {
	return nil
}

func (id *mockIdentity) GetOrganizationalUnits() []*msp.OUIdentifier {
	return nil
}

func (id *mockIdentity) Verify(msg []byte, sig []byte) error {
	if bytes.Compare(sig, []byte("badsigned")) == 0 {
		return errors.New("Invalid signature")
	}
	return nil
}

func (id *mockIdentity) Serialize() ([]byte, error) {
	return id.idBytes, nil
}

type mockDeserializer struct {
	fail error
}

func (md *mockDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	if md.fail != nil {
		return nil, md.fail
	}
	return &mockIdentity{idBytes: serializedIdentity}, nil
}

func (md *mockDeserializer) IsWellFormed(_ *mb.SerializedIdentity) error {
	return nil
}

func TestSetupWithBadConfig(t *testing.T) {
	// set up simple collection with invalid data
	var sc SimpleCollection
	err := sc.Setup(&pb.StaticCollectionConfig{}, &mockDeserializer{})
	assert.Error(t, err)

	// create static collection config with faulty policy
	collectionConfig := &pb.StaticCollectionConfig{
		Name:              "test collection",
		RequiredPeerCount: 1,
		MemberOrgsPolicy:  getBadAccessPolicy([]string{"peer0", "peer1"}, 3),
	}
	err = sc.Setup(collectionConfig, &mockDeserializer{})
	assert.Error(t, err)
	assert.EqualError(t, err, "failed constructing policy object out of collection policy config: identity index out of range, requested 3, but identities length is 2")
}

func TestSetupGoodConfigCollection(t *testing.T) {
	// create member access policy
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	accessPolicy := createCollectionPolicyConfig(policyEnvelope)

	// create static collection config
	collectionConfig := &pb.StaticCollectionConfig{
		Name:              "test collection",
		RequiredPeerCount: 1,
		MemberOrgsPolicy:  accessPolicy,
	}

	// set up simple collection with valid data
	var sc SimpleCollection
	err := sc.Setup(collectionConfig, &mockDeserializer{})
	assert.NoError(t, err)

	// check name
	assert.True(t, sc.CollectionID() == "test collection")

	// check members
	members := sc.MemberOrgs()
	assert.Contains(t, members, "signer0")
	assert.Contains(t, members, "signer1")

	// check required peer count
	assert.True(t, sc.RequiredPeerCount() == 1)
}

func TestSimpleCollectionFilter(t *testing.T) {
	// create member access policy
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	accessPolicy := createCollectionPolicyConfig(policyEnvelope)

	// create static collection config
	collectionConfig := &pb.StaticCollectionConfig{
		Name:              "test collection",
		RequiredPeerCount: 1,
		MemberOrgsPolicy:  accessPolicy,
	}

	// set up simple collection
	var sc SimpleCollection
	err := sc.Setup(collectionConfig, &mockDeserializer{})
	assert.NoError(t, err)

	// get the collection access filter
	var cap CollectionAccessPolicy
	cap = &sc
	accessFilter := cap.AccessFilter()

	// check filter: not a member of the collection
	notMember := pb.SignedData{
		Identity:  []byte{1, 2, 3},
		Signature: []byte{},
		Data:      []byte{},
	}
	assert.False(t, accessFilter(notMember))

	// check filter: member of the collection
	member := pb.SignedData{
		Identity:  signers[0],
		Signature: []byte{},
		Data:      []byte{},
	}
	assert.True(t, accessFilter(member))
}
