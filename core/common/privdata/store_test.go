/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	lm "github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type mockStoreSupport struct {
	Qe                     *lm.MockQueryExecutor
	QErr                   error
	CollectionInfoProvider *mock.DeployedChaincodeInfoProvider
}

func (c *mockStoreSupport) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	return c.Qe, c.QErr
}

func (c *mockStoreSupport) GetIdentityDeserializer(chainID string) msp.IdentityDeserializer {
	return &mockDeserializer{}
}

func (c *mockStoreSupport) GetCollectionInfoProvider() ledger.DeployedChaincodeInfoProvider {
	return c.CollectionInfoProvider
}

func TestCollectionStore(t *testing.T) {
	support := &mockStoreSupport{
		CollectionInfoProvider: &mock.DeployedChaincodeInfoProvider{},
	}
	cs := NewSimpleCollectionStore(support)
	assert.NotNil(t, cs)

	support.QErr = errors.New("")
	_, err := cs.RetrieveCollection(common.CollectionCriteria{})
	assert.Contains(t, err.Error(), "could not retrieve query executor for collection criteria")
	support.QErr = nil

	_, err = cs.retrieveCollectionConfigPackage(common.CollectionCriteria{Namespace: "non-existing-chaincode"}, nil)
	assert.EqualError(t, err, "Chaincode [non-existing-chaincode] does not exist")

	_, err = cs.RetrieveCollection(common.CollectionCriteria{})
	assert.Contains(t, err.Error(), "could not be found")

	ccr := common.CollectionCriteria{Channel: "ch", Namespace: "cc", Collection: "mycollection"}
	support.CollectionInfoProvider.CollectionInfoReturns(nil, errors.New("dummy error"))
	_, err = cs.RetrieveCollection(ccr)
	assert.EqualError(t, err, "dummy error")

	scc := &common.StaticCollectionConfig{
		Name: "mycollection",
	}
	support.CollectionInfoProvider.CollectionInfoReturns(scc, nil)
	_, err = cs.RetrieveCollection(ccr)
	assert.Contains(t, err.Error(), "error setting up collection for collection criteria")

	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	accessPolicy := createCollectionPolicyConfig(policyEnvelope)

	scc = &common.StaticCollectionConfig{
		Name:             "mycollection",
		MemberOrgsPolicy: accessPolicy,
		MemberOnlyRead:   false,
		MemberOnlyWrite:  false,
	}

	support.CollectionInfoProvider.CollectionInfoReturns(scc, nil)
	c, err := cs.RetrieveCollection(ccr)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	ca, err := cs.RetrieveCollectionAccessPolicy(ccr)
	assert.NoError(t, err)
	assert.NotNil(t, ca)

	scc = &common.StaticCollectionConfig{
		Name:             "mycollection",
		MemberOrgsPolicy: accessPolicy,
		MemberOnlyRead:   true,
		MemberOnlyWrite:  true,
	}
	cc := &common.CollectionConfig{Payload: &common.CollectionConfig_StaticCollectionConfig{
		StaticCollectionConfig: scc,
	}}
	ccp := &common.CollectionConfigPackage{Config: []*common.CollectionConfig{cc}}

	support.CollectionInfoProvider.CollectionInfoReturns(scc, nil)
	support.CollectionInfoProvider.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{
			ExplicitCollectionConfigPkg: ccp,
		}, nil)

	ccc, err := cs.RetrieveCollectionConfigPackage(ccr)
	assert.NoError(t, err)
	assert.NotNil(t, ccc)

	signedProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("signer0"), []byte("msg1"))
	readP, writeP, err := cs.RetrieveReadWritePermission(ccr, signedProp, &lm.MockQueryExecutor{})
	assert.NoError(t, err)
	assert.True(t, readP)
	assert.True(t, writeP)

	// only signer0 and signer1 are the members
	signedProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("signer2"), []byte("msg1"))
	readP, writeP, err = cs.RetrieveReadWritePermission(ccr, signedProp, &lm.MockQueryExecutor{})
	assert.NoError(t, err)
	assert.False(t, readP)
	assert.False(t, writeP)

	scc = &common.StaticCollectionConfig{
		Name:             "mycollection",
		MemberOrgsPolicy: accessPolicy,
		MemberOnlyRead:   false,
		MemberOnlyWrite:  false,
	}
	support.CollectionInfoProvider.CollectionInfoReturns(scc, nil)

	// only signer0 and signer1 are the members
	signedProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("signer2"), []byte("msg1"))
	readP, writeP, err = cs.RetrieveReadWritePermission(ccr, signedProp, &lm.MockQueryExecutor{})
	assert.NoError(t, err)
	assert.True(t, readP)
	assert.True(t, writeP)
}
