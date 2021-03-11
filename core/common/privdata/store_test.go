/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/common/privdata/mock"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// local interfaces to avoid import cycles
//go:generate counterfeiter -o mock/query_executor_factory.go -fake-name QueryExecutorFactory . queryExecutorFactory
//go:generate counterfeiter -o mock/chaincode_info_provider.go -fake-name ChaincodeInfoProvider . chaincodeInfoProvider
//go:generate counterfeiter -o mock/identity_deserializer_factory.go -fake-name IdentityDeserializerFactory . identityDeserializerFactory
//go:generate counterfeiter -o mock/query_executor.go -fake-name QueryExecutor . queryExecutor

type (
	queryExecutorFactory        interface{ QueryExecutorFactory }
	chaincodeInfoProvider       interface{ ChaincodeInfoProvider }
	identityDeserializerFactory interface{ IdentityDeserializerFactory }
	queryExecutor               interface{ ledger.QueryExecutor }
)

func TestNewSimpleCollectionStore(t *testing.T) {
	mockQueryExecutorFactory := &mock.QueryExecutorFactory{}
	mockCCInfoProvider := &mock.ChaincodeInfoProvider{}
	mockIDDeserializerFactory := &mock.IdentityDeserializerFactory{}

	cs := NewSimpleCollectionStore(mockQueryExecutorFactory, mockCCInfoProvider, mockIDDeserializerFactory)
	require.NotNil(t, cs)
	require.Exactly(t, mockQueryExecutorFactory, cs.qeFactory)
	require.Exactly(t, mockCCInfoProvider, cs.ccInfoProvider)
	require.Exactly(t, mockIDDeserializerFactory, cs.idDeserializerFactory)
	require.Equal(t,
		&SimpleCollectionStore{
			qeFactory:             mockQueryExecutorFactory,
			ccInfoProvider:        mockCCInfoProvider,
			idDeserializerFactory: mockIDDeserializerFactory,
		},
		cs,
	)
}

func TestCollectionStore(t *testing.T) {
	mockQueryExecutorFactory := &mock.QueryExecutorFactory{}
	mockCCInfoProvider := &mock.ChaincodeInfoProvider{}
	mockCCInfoProvider.AllCollectionsConfigPkgReturns(nil, errors.New("Chaincode [non-existing-chaincode] does not exist"))
	mockIDDeserializerFactory := &mock.IdentityDeserializerFactory{}
	mockIDDeserializerFactory.GetIdentityDeserializerReturns(&mockDeserializer{})

	cs := &SimpleCollectionStore{
		qeFactory:             mockQueryExecutorFactory,
		ccInfoProvider:        mockCCInfoProvider,
		idDeserializerFactory: mockIDDeserializerFactory,
	}

	mockQueryExecutorFactory.NewQueryExecutorReturns(nil, errors.New("new-query-executor-failed"))
	_, err := cs.RetrieveCollection(CollectionCriteria{})
	require.Contains(t, err.Error(), "could not retrieve query executor for collection criteria")

	mockQueryExecutorFactory.NewQueryExecutorReturns(&mock.QueryExecutor{}, nil)
	_, err = cs.retrieveCollectionConfigPackage(CollectionCriteria{Namespace: "non-existing-chaincode"}, nil)
	require.EqualError(t, err, "Chaincode [non-existing-chaincode] does not exist")

	_, err = cs.RetrieveCollection(CollectionCriteria{})
	require.Contains(t, err.Error(), "could not be found")

	ccr := CollectionCriteria{Channel: "ch", Namespace: "cc", Collection: "mycollection"}
	mockCCInfoProvider.CollectionInfoReturns(nil, errors.New("collection-info-error"))
	_, err = cs.RetrieveCollection(ccr)
	require.EqualError(t, err, "collection-info-error")

	scc := &peer.StaticCollectionConfig{Name: "mycollection"}
	mockCCInfoProvider.CollectionInfoReturns(scc, nil)
	_, err = cs.RetrieveCollection(ccr)
	require.Contains(t, err.Error(), "error setting up collection for collection criteria")

	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
	accessPolicy := createCollectionPolicyConfig(policyEnvelope)

	scc = &peer.StaticCollectionConfig{
		Name:             "mycollection",
		MemberOrgsPolicy: accessPolicy,
		MemberOnlyRead:   false,
		MemberOnlyWrite:  false,
	}

	mockCCInfoProvider.CollectionInfoReturns(scc, nil)
	c, err := cs.RetrieveCollection(ccr)
	require.NoError(t, err)
	require.NotNil(t, c)

	ca, err := cs.RetrieveCollectionAccessPolicy(ccr)
	require.NoError(t, err)
	require.NotNil(t, ca)

	scc = &peer.StaticCollectionConfig{
		Name:             "mycollection",
		MemberOrgsPolicy: accessPolicy,
		MemberOnlyRead:   true,
		MemberOnlyWrite:  true,
	}
	cc := &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: scc},
	}
	ccp := &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{cc}}

	mockCCInfoProvider.CollectionInfoReturns(scc, nil)
	mockCCInfoProvider.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{ExplicitCollectionConfigPkg: ccp},
		nil,
	)
	mockCCInfoProvider.AllCollectionsConfigPkgReturns(ccp, nil)

	ccc, err := cs.RetrieveCollectionConfigPackage(ccr)
	require.NoError(t, err)
	require.NotNil(t, ccc)

	signedProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("signer0"), []byte("msg1"))
	readP, writeP, err := cs.RetrieveReadWritePermission(ccr, signedProp, &mock.QueryExecutor{})
	require.NoError(t, err)
	require.True(t, readP)
	require.True(t, writeP)

	// only signer0 and signer1 are the members
	signedProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("signer2"), []byte("msg1"))
	readP, writeP, err = cs.RetrieveReadWritePermission(ccr, signedProp, &mock.QueryExecutor{})
	require.NoError(t, err)
	require.False(t, readP)
	require.False(t, writeP)

	scc = &peer.StaticCollectionConfig{
		Name:             "mycollection",
		MemberOrgsPolicy: accessPolicy,
		MemberOnlyRead:   false,
		MemberOnlyWrite:  false,
	}
	mockCCInfoProvider.CollectionInfoReturns(scc, nil)

	// only signer0 and signer1 are the members
	signedProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("signer2"), []byte("msg1"))
	readP, writeP, err = cs.RetrieveReadWritePermission(ccr, signedProp, &mock.QueryExecutor{})
	require.NoError(t, err)
	require.True(t, readP)
	require.True(t, writeP)
}
