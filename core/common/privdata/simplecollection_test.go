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

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func createCollectionPolicyConfig(accessPolicy *cb.SignaturePolicyEnvelope) *pb.CollectionPolicyConfig {
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
	if bytes.Equal(id.idBytes, p.Principal) {
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
	if bytes.Equal(sig, []byte("badsigned")) {
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

func TestNewSimpleCollectionWithBadConfig(t *testing.T) {
	// set up simple collection with nil collection config
	_, err := NewSimpleCollection(nil, &mockDeserializer{})
	require.Error(t, err)

	// create static collection config with faulty policy
	collectionConfig := &pb.StaticCollectionConfig{
		Name:              "test collection",
		RequiredPeerCount: 1,
		MemberOrgsPolicy:  getBadAccessPolicy([]string{"peer0", "peer1"}, 3),
	}
	_, err = NewSimpleCollection(collectionConfig, &mockDeserializer{})
	require.Error(t, err)
	require.EqualError(t, err, "failed constructing policy object out of collection policy config: identity index out of range, requested 3, but identities length is 2")
}

func TestNewSimpleCollectionWithGoodConfig(t *testing.T) {
	// create member access policy
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
	accessPolicy := createCollectionPolicyConfig(policyEnvelope)

	// create static collection config
	collectionConfig := &pb.StaticCollectionConfig{
		Name:              "test collection",
		RequiredPeerCount: 1,
		MemberOrgsPolicy:  accessPolicy,
	}

	// set up simple collection with valid data
	sc, err := NewSimpleCollection(collectionConfig, &mockDeserializer{})
	require.NoError(t, err)

	// check name
	require.True(t, sc.CollectionID() == "test collection")

	// check members
	members := sc.MemberOrgs()
	require.Contains(t, members, "signer0")
	require.Contains(t, members, "signer1")

	// check required peer count
	require.True(t, sc.RequiredPeerCount() == 1)
}

func TestSetupWithBadConfig(t *testing.T) {
	// set up simple collection with invalid data
	var sc SimpleCollection
	err := sc.Setup(&pb.StaticCollectionConfig{}, &mockDeserializer{})
	require.Error(t, err)

	// create static collection config with faulty policy
	collectionConfig := &pb.StaticCollectionConfig{
		Name:              "test collection",
		RequiredPeerCount: 1,
		MemberOrgsPolicy:  getBadAccessPolicy([]string{"peer0", "peer1"}, 3),
	}
	err = sc.Setup(collectionConfig, &mockDeserializer{})
	require.Error(t, err)
	require.EqualError(t, err, "failed constructing policy object out of collection policy config: identity index out of range, requested 3, but identities length is 2")
}

func TestSetupGoodConfigCollection(t *testing.T) {
	// create member access policy
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
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
	require.NoError(t, err)

	// check name
	require.True(t, sc.CollectionID() == "test collection")

	// check members
	members := sc.MemberOrgs()
	require.Contains(t, members, "signer0")
	require.Contains(t, members, "signer1")

	// check required peer count
	require.True(t, sc.RequiredPeerCount() == 1)
}

func TestSimpleCollectionFilter(t *testing.T) {
	// create member access policy
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
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
	require.NoError(t, err)

	// get the collection access filter
	accessFilter := (&sc).AccessFilter()

	// check filter: not a member of the collection
	notMember := protoutil.SignedData{
		Identity:  []byte{1, 2, 3},
		Signature: []byte{},
		Data:      []byte{},
	}
	require.False(t, accessFilter(notMember))

	// check filter: member of the collection
	member := protoutil.SignedData{
		Identity:  signers[0],
		Signature: []byte{},
		Data:      []byte{},
	}
	require.True(t, accessFilter(member))
}
