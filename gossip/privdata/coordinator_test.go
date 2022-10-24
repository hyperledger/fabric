/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	tspb "github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/metrics"
	gmetricsmocks "github.com/hyperledger/fabric/gossip/metrics/mocks"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	privdatamocks "github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testConfig = CoordinatorConfig{
	PullRetryThreshold:             time.Second * 3,
	TransientBlockRetention:        1000,
	SkipPullingInvalidTransactions: false,
}

// CollectionCriteria aggregates criteria of
// a collection
type CollectionCriteria struct {
	Channel    string
	Collection string
	Namespace  string
}

func fromCollectionCriteria(criteria privdata.CollectionCriteria) CollectionCriteria {
	return CollectionCriteria{
		Collection: criteria.Collection,
		Namespace:  criteria.Namespace,
		Channel:    criteria.Channel,
	}
}

type validatorMock struct {
	err error
}

func (v *validatorMock) Validate(block *common.Block) error {
	if v.err != nil {
		return v.err
	}
	return nil
}

type digests []privdatacommon.DigKey

func (d digests) Equal(other digests) bool {
	flatten := func(d digests) map[privdatacommon.DigKey]struct{} {
		m := map[privdatacommon.DigKey]struct{}{}
		for _, dig := range d {
			m[dig] = struct{}{}
		}
		return m
	}
	return reflect.DeepEqual(flatten(d), flatten(other))
}

type fetchCall struct {
	fetcher *fetcherMock
	*mock.Call
}

func (fc *fetchCall) expectingEndorsers(orgs ...string) *fetchCall {
	if fc.fetcher.expectedEndorsers == nil {
		fc.fetcher.expectedEndorsers = make(map[string]struct{})
	}
	for _, org := range orgs {
		sID := &mspproto.SerializedIdentity{Mspid: org, IdBytes: []byte(fmt.Sprintf("p0%s", org))}
		b, _ := pb.Marshal(sID)
		fc.fetcher.expectedEndorsers[string(b)] = struct{}{}
	}

	return fc
}

func (fc *fetchCall) expectingDigests(digests []privdatacommon.DigKey) *fetchCall {
	fc.fetcher.expectedDigests = digests
	return fc
}

func (fc *fetchCall) Return(returnArguments ...interface{}) *mock.Call {
	return fc.Call.Return(returnArguments...)
}

type fetcherMock struct {
	t *testing.T
	mock.Mock
	expectedDigests   []privdatacommon.DigKey
	expectedEndorsers map[string]struct{}
}

func (f *fetcherMock) On(methodName string, arguments ...interface{}) *fetchCall {
	return &fetchCall{
		fetcher: f,
		Call:    f.Mock.On(methodName, arguments...),
	}
}

func (f *fetcherMock) fetch(dig2src dig2sources) (*privdatacommon.FetchedPvtDataContainer, error) {
	uniqueEndorsements := make(map[string]interface{})
	for _, endorsements := range dig2src {
		for _, endorsement := range endorsements {
			_, exists := f.expectedEndorsers[string(endorsement.Endorser)]
			if !exists {
				f.t.Fatalf("Encountered a non-expected endorser: %s", string(endorsement.Endorser))
			}
			uniqueEndorsements[string(endorsement.Endorser)] = struct{}{}
		}
	}
	require.True(f.t, digests(f.expectedDigests).Equal(digests(dig2src.keys())))
	require.Equal(f.t, len(f.expectedEndorsers), len(uniqueEndorsements))
	args := f.Called(dig2src)
	if args.Get(1) == nil {
		return args.Get(0).(*privdatacommon.FetchedPvtDataContainer), nil
	}
	return nil, args.Get(1).(error)
}

type testTransientStore struct {
	storeProvider transientstore.StoreProvider
	store         *transientstore.Store
	tempdir       string
}

func newTransientStore(t *testing.T) *testTransientStore {
	s := &testTransientStore{}
	var err error
	s.tempdir = t.TempDir()
	s.storeProvider, err = transientstore.NewStoreProvider(s.tempdir)
	if err != nil {
		t.Fatalf("Failed to open store, got err %s", err)
		return s
	}
	s.store, err = s.storeProvider.OpenStore("testchannelid")
	if err != nil {
		t.Fatalf("Failed to open store, got err %s", err)
		return s
	}
	return s
}

func (s *testTransientStore) tearDown() {
	s.storeProvider.Close()
}

func (s *testTransientStore) Persist(txid string, blockHeight uint64,
	privateSimulationResultsWithConfig *tspb.TxPvtReadWriteSetWithConfigInfo) error {
	return s.store.Persist(txid, blockHeight, privateSimulationResultsWithConfig)
}

func (s *testTransientStore) GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (RWSetScanner, error) {
	return s.store.GetTxPvtRWSetByTxid(txid, filter)
}

func createcollectionStore(expectedSignedData protoutil.SignedData) *collectionStore {
	return &collectionStore{
		expectedSignedData: expectedSignedData,
		policies:           make(map[collectionAccessPolicy]CollectionCriteria),
		store:              make(map[CollectionCriteria]collectionAccessPolicy),
	}
}

type collectionStore struct {
	expectedSignedData protoutil.SignedData
	acceptsAll         bool
	acceptsNone        bool
	lenient            bool
	mspIdentifier      string
	store              map[CollectionCriteria]collectionAccessPolicy
	policies           map[collectionAccessPolicy]CollectionCriteria
}

func (cs *collectionStore) thatAcceptsAll() *collectionStore {
	cs.acceptsAll = true
	return cs
}

func (cs *collectionStore) thatAcceptsNone() *collectionStore {
	cs.acceptsNone = true
	return cs
}

func (cs *collectionStore) thatAccepts(cc CollectionCriteria) *collectionStore {
	sp := collectionAccessPolicy{
		cs: cs,
		n:  util.RandomUInt64(),
	}
	cs.store[cc] = sp
	cs.policies[sp] = cc
	return cs
}

func (cs *collectionStore) withMSPIdentity(identifier string) *collectionStore {
	cs.mspIdentifier = identifier
	return cs
}

func (cs *collectionStore) RetrieveCollectionAccessPolicy(cc privdata.CollectionCriteria) (privdata.CollectionAccessPolicy, error) {
	if sp, exists := cs.store[fromCollectionCriteria(cc)]; exists {
		return &sp, nil
	}
	if cs.acceptsAll || cs.acceptsNone || cs.lenient {
		return &collectionAccessPolicy{
			cs: cs,
			n:  util.RandomUInt64(),
		}, nil
	}
	return nil, privdata.NoSuchCollectionError{}
}

func (cs *collectionStore) RetrieveCollection(privdata.CollectionCriteria) (privdata.Collection, error) {
	panic("implement me")
}

func (cs *collectionStore) RetrieveCollectionConfig(cc privdata.CollectionCriteria) (*peer.StaticCollectionConfig, error) {
	mspIdentifier := "different-org"
	if _, exists := cs.store[fromCollectionCriteria(cc)]; exists || cs.acceptsAll {
		mspIdentifier = cs.mspIdentifier
	}
	return &peer.StaticCollectionConfig{
		Name:           cc.Collection,
		MemberOnlyRead: true,
		MemberOrgsPolicy: &peer.CollectionPolicyConfig{
			Payload: &peer.CollectionPolicyConfig_SignaturePolicy{
				SignaturePolicy: &common.SignaturePolicyEnvelope{
					Rule: &common.SignaturePolicy{
						Type: &common.SignaturePolicy_SignedBy{
							SignedBy: 0,
						},
					},
					Identities: []*mspproto.MSPPrincipal{
						{
							PrincipalClassification: mspproto.MSPPrincipal_ROLE,
							Principal: protoutil.MarshalOrPanic(&mspproto.MSPRole{
								MspIdentifier: mspIdentifier,
								Role:          mspproto.MSPRole_MEMBER,
							}),
						},
					},
				},
			},
		},
	}, nil
}

func (cs *collectionStore) RetrieveReadWritePermission(cc privdata.CollectionCriteria, sp *peer.SignedProposal, qe ledger.QueryExecutor) (bool, bool, error) {
	panic("implement me")
}

func (cs *collectionStore) RetrieveCollectionConfigPackage(cc privdata.CollectionCriteria) (*peer.CollectionConfigPackage, error) {
	return &peer.CollectionConfigPackage{
		Config: []*peer.CollectionConfig{
			{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name:              cc.Collection,
						MaximumPeerCount:  1,
						RequiredPeerCount: 1,
					},
				},
			},
		},
	}, nil
}

func (cs *collectionStore) RetrieveCollectionPersistenceConfigs(cc privdata.CollectionCriteria) (privdata.CollectionPersistenceConfigs, error) {
	panic("implement me")
}

func (cs *collectionStore) AccessFilter(channelName string, collectionPolicyConfig *peer.CollectionPolicyConfig) (privdata.Filter, error) {
	panic("implement me")
}

type collectionAccessPolicy struct {
	cs *collectionStore
	n  uint64
}

func (cap *collectionAccessPolicy) MemberOrgs() map[string]struct{} {
	return map[string]struct{}{
		"org0": {},
		"org1": {},
	}
}

func (cap *collectionAccessPolicy) RequiredPeerCount() int {
	return 1
}

func (cap *collectionAccessPolicy) MaximumPeerCount() int {
	return 2
}

func (cap *collectionAccessPolicy) IsMemberOnlyRead() bool {
	return false
}

func (cap *collectionAccessPolicy) IsMemberOnlyWrite() bool {
	return false
}

func (cap *collectionAccessPolicy) AccessFilter() privdata.Filter {
	return func(sd protoutil.SignedData) bool {
		that, _ := asn1.Marshal(sd)
		this, _ := asn1.Marshal(cap.cs.expectedSignedData)
		if hex.EncodeToString(that) != hex.EncodeToString(this) {
			panic(fmt.Errorf("self signed data passed isn't equal to expected:%v, %v", sd, cap.cs.expectedSignedData))
		}

		if cap.cs.acceptsNone {
			return false
		} else if cap.cs.acceptsAll {
			return true
		}

		_, exists := cap.cs.policies[*cap]
		return exists
	}
}

func TestPvtDataCollections_FailOnEmptyPayload(t *testing.T) {
	collection := &util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7},
							},
						},
					},
				},
			},
		},

		nil,
	}

	_, err := collection.Marshal()
	assertion := require.New(t)
	assertion.Error(err, "Expected to fail since second item has nil payload")
	assertion.Equal("Mallformed private data payload, rwset index 1 is nil", fmt.Sprintf("%s", err))
}

func TestPvtDataCollections_FailMarshalingWriteSet(t *testing.T) {
	collection := &util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet:   nil,
		},
	}

	_, err := collection.Marshal()
	assertion := require.New(t)
	assertion.Error(err, "Expected to fail since first item has nil writeset")
	assertion.Contains(fmt.Sprintf("%s", err), "Could not marshal private rwset index 0")
}

func TestPvtDataCollections_Marshal(t *testing.T) {
	collection := &util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7},
							},
						},
					},
				},
			},
		},

		&ledger.TxPvtData{
			SeqInBlock: uint64(2),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{42, 42, 42, 42, 42, 42, 42},
							},
						},
					},
					{
						Namespace: "ns2",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "otherCollection",
								Rwset:          []byte{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
							},
						},
					},
				},
			},
		},
	}

	bytes, err := collection.Marshal()

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotNil(bytes)
	assertion.Equal(2, len(bytes))
}

func TestPvtDataCollections_Unmarshal(t *testing.T) {
	collection := util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7},
							},
						},
					},
				},
			},
		},
	}

	bytes, err := collection.Marshal()

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotNil(bytes)
	assertion.Equal(1, len(bytes))

	var newCol util.PvtDataCollections

	err = newCol.Unmarshal(bytes)
	assertion.NoError(err)
	assertion.Equal(1, len(newCol))
	assertion.Equal(newCol[0].SeqInBlock, collection[0].SeqInBlock)
	assertion.True(pb.Equal(newCol[0].WriteSet, collection[0].WriteSet))
}

type rwsTriplet struct {
	namespace  string
	collection string
	rwset      string
}

func flattenTxPvtDataMap(pd ledger.TxPvtDataMap) map[uint64]map[rwsTriplet]struct{} {
	m := make(map[uint64]map[rwsTriplet]struct{})
	for seqInBlock, namespaces := range pd {
		triplets := make(map[rwsTriplet]struct{})
		for _, namespace := range namespaces.WriteSet.NsPvtRwset {
			for _, col := range namespace.CollectionPvtRwset {
				triplets[rwsTriplet{
					namespace:  namespace.Namespace,
					collection: col.CollectionName,
					rwset:      hex.EncodeToString(col.Rwset),
				}] = struct{}{}
			}
		}
		m[seqInBlock] = triplets
	}
	return m
}

var expectedCommittedPrivateData1 = map[uint64]*ledger.TxPvtData{
	0: {SeqInBlock: 0, WriteSet: &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns1",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "c1",
						Rwset:          []byte("rws-pre-image"),
					},
					{
						CollectionName: "c2",
						Rwset:          []byte("rws-pre-image"),
					},
				},
			},
		},
	}},
	1: {SeqInBlock: 1, WriteSet: &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns2",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "c1",
						Rwset:          []byte("rws-pre-image"),
					},
				},
			},
		},
	}},
}

var expectedCommittedPrivateData2 = map[uint64]*ledger.TxPvtData{
	0: {SeqInBlock: 0, WriteSet: &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns3",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "c3",
						Rwset:          []byte("rws-pre-image"),
					},
				},
			},
		},
	}},
}

var expectedCommittedPrivateData3 = map[uint64]*ledger.TxPvtData{}

func TestCoordinatorStoreInvalidBlock(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		t.Fatal("Shouldn't have committed")
	}).Return(nil)
	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll().withMSPIdentity(identity.GetMSPIdentifier())

	store := newTransientStore(t)
	defer store.tearDown()

	assertPurged := func(txns ...string) {
		for _, txn := range txns {
			iterator, err := store.GetTxPvtRWSetByTxid(txn, nil)
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			res, err := iterator.Next()
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			require.Nil(t, res)
			iterator.Close()
		}
	}
	fetcher := &fetcherMock{t: t}
	pdFactory := &pvtDataFactory{}
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})
	block := bf.withoutMetadata().create()
	// Scenario I: Block we got doesn't have any metadata with it
	pvtData := pdFactory.create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Block.Metadata is nil or Block.Metadata lacks a Tx filter bitmap")

	// Scenario II: Validator has an error while validating the block
	block = bf.create()
	pvtData = pdFactory.create()
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{fmt.Errorf("failed validating block")},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed validating block")

	// Scenario III: Block we got contains an inadequate length of Tx filter in the metadata
	block = bf.withMetadataSize(100).create()
	pvtData = pdFactory.create()
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "block data size")
	require.Contains(t, err.Error(), "is different from Tx filter size")

	// Scenario IV: The second transaction in the block we got is invalid, and we have no private data for that.
	// As the StorePvtDataOfInvalidTx is set of false, if the coordinator would try to fetch private data, the
	// test would fall because we haven't defined the mock operations for the transientstore (or for gossip)
	// in this test.
	var commitHappened bool
	assertCommitHappened := func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	digKeys := []privdatacommon.DigKey{
		{
			TxId:       "tx2",
			Namespace:  "ns2",
			Collection: "c1",
			BlockSeq:   1,
			SeqInBlock: 1,
		},
	}
	fetcher = &fetcherMock{t: t}
	fetcher.On("fetch", mock.Anything).expectingDigests(digKeys).expectingEndorsers(identity.GetMSPIdentifier()).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: nil,
	}, nil)
	committer = &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		privateDataPassed2Ledger := args.Get(0).(*ledger.BlockAndPvtData).PvtData
		commitHappened = true
		// Only the first transaction's private data is passed to the ledger
		require.Len(t, privateDataPassed2Ledger, 1)
		require.Equal(t, 0, int(privateDataPassed2Ledger[0].SeqInBlock))
		// The private data passed to the ledger contains "ns1" and has 2 collections in it
		require.Len(t, privateDataPassed2Ledger[0].WriteSet.NsPvtRwset, 1)
		require.Equal(t, "ns1", privateDataPassed2Ledger[0].WriteSet.NsPvtRwset[0].Namespace)
		require.Len(t, privateDataPassed2Ledger[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset, 2)
	}).Return(nil)
	block = bf.withInvalidTxns(1).AddTxn("tx1", "ns1", hash, "c1", "c2").AddTxn("tx2", "ns2", hash, "c1").create()
	pvtData = pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	capabilityProvider = &privdatamocks.CapabilityProvider{}
	appCapability = &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(false)
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	// Ensure the 2nd transaction which is invalid and wasn't committed - is still purged.
	// This is so that if we get a transaction via dissemination from an endorser, we purge it
	// when its block comes.
	assertPurged("tx1", "tx2")

	// Scenario V: The second transaction in the block we got is invalid, and we have no private
	// data for that in the transient store. As we have set StorePvtDataOfInvalidTx to true and
	// configured the coordinator to skip pulling pvtData of invalid transactions from other peers,
	// it should not store the pvtData of invalid transaction in the ledger instead a missing entry.
	testConfig.SkipPullingInvalidTransactions = true
	assertCommitHappened = func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	committer = &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		blockAndPvtData := args.Get(0).(*ledger.BlockAndPvtData)
		commitHappened = true
		// Only the first transaction's private data is passed to the ledger
		privateDataPassed2Ledger := blockAndPvtData.PvtData
		require.Len(t, privateDataPassed2Ledger, 1)
		require.Equal(t, 0, int(privateDataPassed2Ledger[0].SeqInBlock))
		// The private data passed to the ledger contains "ns1" and has 2 collections in it
		require.Len(t, privateDataPassed2Ledger[0].WriteSet.NsPvtRwset, 1)
		require.Equal(t, "ns1", privateDataPassed2Ledger[0].WriteSet.NsPvtRwset[0].Namespace)
		require.Len(t, privateDataPassed2Ledger[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset, 2)

		missingPrivateDataPassed2Ledger := blockAndPvtData.MissingPvtData
		require.Len(t, missingPrivateDataPassed2Ledger, 1)
		require.Len(t, missingPrivateDataPassed2Ledger[1], 1)
		require.Equal(t, missingPrivateDataPassed2Ledger[1][0].Namespace, "ns2")
		require.Equal(t, missingPrivateDataPassed2Ledger[1][0].Collection, "c1")
		require.Equal(t, missingPrivateDataPassed2Ledger[1][0].IsEligible, true)

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	block = bf.withInvalidTxns(1).AddTxn("tx1", "ns1", hash, "c1", "c2").AddTxn("tx2", "ns2", hash, "c1").create()
	pvtData = pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	capabilityProvider = &privdatamocks.CapabilityProvider{}
	appCapability = &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	digKeys = []privdatacommon.DigKey{}
	fetcher = &fetcherMock{t: t}
	fetcher.On("fetch", mock.Anything).expectingDigests(digKeys).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: nil,
	}, nil)
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurged("tx1", "tx2")

	// Scenario VI: The second transaction in the block we got is invalid. As we have set the
	// StorePvtDataOfInvalidTx to true and configured the coordinator to pull pvtData of invalid
	// transactions, it should store the pvtData of invalid transactions in the ledger.
	testConfig.SkipPullingInvalidTransactions = false
	committer = &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		blockAndPvtData := args.Get(0).(*ledger.BlockAndPvtData)
		commitHappened = true
		// pvtData of both transactions must be present though the second transaction
		// is invalid.
		privateDataPassed2Ledger := blockAndPvtData.PvtData
		require.Len(t, privateDataPassed2Ledger, 2)
		require.Equal(t, 0, int(privateDataPassed2Ledger[0].SeqInBlock))
		require.Equal(t, 1, int(privateDataPassed2Ledger[1].SeqInBlock))
		// The private data passed to the ledger for tx1 contains "ns1" and has 2 collections in it
		require.Len(t, privateDataPassed2Ledger[0].WriteSet.NsPvtRwset, 1)
		require.Equal(t, "ns1", privateDataPassed2Ledger[0].WriteSet.NsPvtRwset[0].Namespace)
		require.Len(t, privateDataPassed2Ledger[0].WriteSet.NsPvtRwset[0].CollectionPvtRwset, 2)
		// The private data passed to the ledger for tx2 contains "ns2" and has 1 collection in it
		require.Len(t, privateDataPassed2Ledger[1].WriteSet.NsPvtRwset, 1)
		require.Equal(t, "ns2", privateDataPassed2Ledger[1].WriteSet.NsPvtRwset[0].Namespace)
		require.Len(t, privateDataPassed2Ledger[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset, 1)

		missingPrivateDataPassed2Ledger := blockAndPvtData.MissingPvtData
		require.Len(t, missingPrivateDataPassed2Ledger, 0)

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	fetcher = &fetcherMock{t: t}
	fetcher.On("fetch", mock.Anything).expectingDigests([]privdatacommon.DigKey{
		{
			TxId: "tx2", Namespace: "ns2", Collection: "c1", BlockSeq: 1, SeqInBlock: 1,
		},
	}).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: []*proto.PvtDataElement{
			{
				Digest: &proto.PvtDataDigest{
					SeqInBlock: 1,
					BlockSeq:   1,
					Collection: "c1",
					Namespace:  "ns2",
					TxId:       "tx2",
				},
				Payload: [][]byte{[]byte("rws-pre-image")},
			},
		},
	}, nil)

	block = bf.withInvalidTxns(1).AddTxnWithEndorsement("tx1", "ns1", hash, "org1", true, "c1", "c2").
		AddTxnWithEndorsement("tx2", "ns2", hash, "org2", true, "c1").create()
	pvtData = pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurged("tx1", "tx2")

	// Scenario VII: Block doesn't contain a header
	block.Header = nil
	err = coordinator.StoreBlock(block, pvtData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Block header is nil")

	// Scenario VIII: Block doesn't contain Data
	block.Data = nil
	err = coordinator.StoreBlock(block, pvtData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Block data is empty")
}

func TestCoordinatorToFilterOutPvtRWSetsWithWrongHash(t *testing.T) {
	/*
		Test case, where peer receives new block for commit
		it has ns1:c1 in transient store, while it has wrong
		hash, hence it will fetch ns1:c1 from other peers
	*/
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	expectedPvtData := map[uint64]*ledger.TxPvtData{
		0: {SeqInBlock: 0, WriteSet: &rwset.TxPvtReadWriteSet{
			DataModel: rwset.TxReadWriteSet_KV,
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				{
					Namespace: "ns1",
					CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
						{
							CollectionName: "c1",
							Rwset:          []byte("rws-original"),
						},
					},
				},
			},
		}},
	}

	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll().withMSPIdentity(identity.GetMSPIdentifier())
	committer := &privdatamocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	assertPurged := func(txns ...string) {
		for _, txn := range txns {
			iterator, err := store.GetTxPvtRWSetByTxid(txn, nil)
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			res, err := iterator.Next()
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			require.Nil(t, res)
			iterator.Close()
		}
	}

	fetcher := &fetcherMock{t: t}

	var commitHappened bool

	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		privateDataPassed2Ledger := args.Get(0).(*ledger.BlockAndPvtData).PvtData
		require.True(t, reflect.DeepEqual(flattenTxPvtDataMap(privateDataPassed2Ledger),
			flattenTxPvtDataMap(expectedPvtData)))
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	hash := util2.ComputeSHA256([]byte("rws-original"))
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	block := bf.AddTxnWithEndorsement("tx1", "ns1", hash, "org1", true, "c1").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)

	fetcher.On("fetch", mock.Anything).expectingDigests([]privdatacommon.DigKey{
		{
			TxId: "tx1", Namespace: "ns1", Collection: "c1", BlockSeq: 1,
		},
	}).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: []*proto.PvtDataElement{
			{
				Digest: &proto.PvtDataDigest{
					BlockSeq:   1,
					Collection: "c1",
					Namespace:  "ns1",
					TxId:       "tx1",
				},
				Payload: [][]byte{[]byte("rws-original")},
			},
		},
	}, nil)

	coordinator.StoreBlock(block, nil)
	// Assert blocks was eventually committed
	require.True(t, commitHappened)

	// Assert transaction has been purged
	assertPurged("tx1")
}

func TestCoordinatorStoreBlock(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}
	// Green path test, all private data should be obtained successfully

	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll().withMSPIdentity(identity.GetMSPIdentifier())

	var commitHappened bool
	assertCommitHappened := func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		privateDataPassed2Ledger := args.Get(0).(*ledger.BlockAndPvtData).PvtData
		require.True(t, reflect.DeepEqual(flattenTxPvtDataMap(privateDataPassed2Ledger),
			flattenTxPvtDataMap(expectedCommittedPrivateData1)))
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	store := newTransientStore(t)
	defer store.tearDown()

	assertPurged := func(txns ...string) bool {
		for _, txn := range txns {
			iterator, err := store.GetTxPvtRWSetByTxid(txn, nil)
			if err != nil {
				iterator.Close()
				t.Fatalf("Failed iterating, got err %s", err)
			}
			res, err := iterator.Next()
			iterator.Close()
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			if res != nil {
				return false
			}
		}
		return true
	}

	fetcher := &fetcherMock{t: t}

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	pdFactory := &pvtDataFactory{}
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	block := bf.AddTxnWithEndorsement("tx1", "ns1", hash, "org1", true, "c1", "c2").
		AddTxnWithEndorsement("tx2", "ns2", hash, "org2", true, "c1").create()

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	fmt.Println("Scenario I")
	// Scenario I: Block we got has sufficient private data alongside it.
	// If the coordinator tries fetching from the transientstore, or peers it would result in panic,
	// because we didn't define yet the "On(...)" invocation of the transient store or other peers.
	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").addRWSet().addNSRWSet("ns2", "c1").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurgeTxs := func() bool {
		return assertPurged("tx1", "tx2")
	}
	require.Eventually(t, assertPurgeTxs, 2*time.Second, 100*time.Millisecond)

	fmt.Println("Scenario II")
	// Scenario II: Block we got doesn't have sufficient private data alongside it,
	// it is missing ns1: c2, but the data exists in the transient store
	store.Persist("tx1", 1, &tspb.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: &rwset.TxPvtReadWriteSet{
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				{
					Namespace: "ns1",
					CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
						{
							CollectionName: "c2",
							Rwset:          []byte("rws-pre-image"),
						},
					},
				},
			},
		},
		CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
	})
	pvtData = pdFactory.addRWSet().addNSRWSet("ns1", "c1").addRWSet().addNSRWSet("ns2", "c1").create()
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurgeTxs = func() bool {
		return assertPurged("tx1", "tx2")
	}
	require.Eventually(t, assertPurgeTxs, 2*time.Second, 100*time.Millisecond)

	fmt.Println("Scenario III")
	// Scenario III: Block doesn't have sufficient private data alongside it,
	// it is missing ns1: c2, and the data exists in the transient store,
	// but it is also missing ns2: c1, and that data doesn't exist in the transient store - but in a peer.
	// Additionally, the coordinator should pass an endorser identity of org1, but not of org2, since
	// the MemberOrgs() call doesn't return org2 but only org0 and org1.
	fetcher.On("fetch", mock.Anything).expectingDigests([]privdatacommon.DigKey{
		{
			TxId: "tx1", Namespace: "ns1", Collection: "c2", BlockSeq: 1,
		},
		{
			TxId: "tx2", Namespace: "ns2", Collection: "c1", BlockSeq: 1, SeqInBlock: 1,
		},
	}).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: []*proto.PvtDataElement{
			{
				Digest: &proto.PvtDataDigest{
					BlockSeq:   1,
					Collection: "c2",
					Namespace:  "ns1",
					TxId:       "tx1",
				},
				Payload: [][]byte{[]byte("rws-pre-image")},
			},
			{
				Digest: &proto.PvtDataDigest{
					SeqInBlock: 1,
					BlockSeq:   1,
					Collection: "c1",
					Namespace:  "ns2",
					TxId:       "tx2",
				},
				Payload: [][]byte{[]byte("rws-pre-image")},
			},
		},
	}, nil)
	pvtData = pdFactory.addRWSet().addNSRWSet("ns1", "c1").create()
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurgeTxs = func() bool {
		return assertPurged("tx1", "tx2")
	}
	require.Eventually(t, assertPurgeTxs, 2*time.Second, 100*time.Millisecond)

	fmt.Println("Scenario IV")
	// Scenario IV: Block came with more than sufficient private data alongside it, some of it is redundant.
	pvtData = pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2", "c3").
		addRWSet().addNSRWSet("ns2", "c1", "c3").addRWSet().addNSRWSet("ns1", "c4").create()
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurgeTxs = func() bool {
		return assertPurged("tx1", "tx2")
	}
	require.Eventually(t, assertPurgeTxs, 2*time.Second, 100*time.Millisecond)

	fmt.Println("Scenario V")
	// Scenario V: Block we got has private data alongside it but coordinator cannot retrieve collection access
	// policy of collections due to databse unavailability error.
	// we verify that the error propagates properly.
	mockCs := &privdatamocks.CollectionStore{}
	mockCs.On("RetrieveCollectionConfig", mock.Anything).Return(nil, errors.New("test error"))
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    mockCs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, nil)
	require.Error(t, err)
	require.Equal(t, "test error", err.Error())

	fmt.Println("Scenario VI")
	// Scenario VI: Block didn't get with any private data alongside it, and the transient store
	// has some problem.
	// In this case, we should try to fetch data from peers.
	block = bf.AddTxn("tx3", "ns3", hash, "c3").create()
	fetcher = &fetcherMock{t: t}
	fetcher.On("fetch", mock.Anything).expectingDigests([]privdatacommon.DigKey{
		{
			TxId: "tx3", Namespace: "ns3", Collection: "c3", BlockSeq: 1,
		},
	}).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: []*proto.PvtDataElement{
			{
				Digest: &proto.PvtDataDigest{
					BlockSeq:   1,
					Collection: "c3",
					Namespace:  "ns3",
					TxId:       "tx3",
				},
				Payload: [][]byte{[]byte("rws-pre-image")},
			},
		},
	}, nil)
	committer = &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		privateDataPassed2Ledger := args.Get(0).(*ledger.BlockAndPvtData).PvtData
		require.True(t, reflect.DeepEqual(flattenTxPvtDataMap(privateDataPassed2Ledger),
			flattenTxPvtDataMap(expectedCommittedPrivateData2)))
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, nil)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurgeTxs = func() bool {
		return assertPurged("tx3")
	}
	require.Eventually(t, assertPurgeTxs, 2*time.Second, 100*time.Millisecond)

	fmt.Println("Scenario VII")
	// Scenario VII: Block contains 2 transactions, and the peer is eligible for only tx3-ns3-c3.
	// Also, the blocks comes with a private data for tx3-ns3-c3 so that the peer won't have to fetch the
	// private data from the transient store or peers, and in fact- if it attempts to fetch the data it's not eligible
	// for from the transient store or from peers - the test would fail because the Mock wasn't initialized.
	block = bf.AddTxn("tx3", "ns3", hash, "c3", "c2", "c1").AddTxn("tx1", "ns1", hash, "c1").create()
	cs = createcollectionStore(peerSelfSignedData).thatAccepts(CollectionCriteria{
		Collection: "c3",
		Namespace:  "ns3",
		Channel:    "testchannelid",
	}).withMSPIdentity(identity.GetMSPIdentifier())
	fetcher = &fetcherMock{t: t}
	committer = &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		privateDataPassed2Ledger := args.Get(0).(*ledger.BlockAndPvtData).PvtData
		require.True(t, reflect.DeepEqual(flattenTxPvtDataMap(privateDataPassed2Ledger),
			flattenTxPvtDataMap(expectedCommittedPrivateData2)))
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	coordinator = NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)

	pvtData = pdFactory.addRWSet().addNSRWSet("ns3", "c3").create()
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	// In any case, all transactions in the block are purged from the transient store
	assertPurgeTxs = func() bool {
		return assertPurged("tx3", "tx1")
	}
	require.Eventually(t, assertPurgeTxs, 2*time.Second, 100*time.Millisecond)
}

func TestCoordinatorStoreBlockWhenPvtDataExistInLedger(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	var commitHappened bool
	assertCommitHappened := func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		privateDataPassed2Ledger := args.Get(0).(*ledger.BlockAndPvtData).PvtData
		require.Equal(t, ledger.TxPvtDataMap{}, privateDataPassed2Ledger)
		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: true}
		require.Equal(t, expectedCommitOpts, commitOpts)
		commitHappened = true
	}).Return(nil)

	fetcher := &fetcherMock{t: t}

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	pdFactory := &pvtDataFactory{}
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	block := bf.AddTxnWithEndorsement("tx1", "ns1", hash, "org1", true, "c1", "c2").
		AddTxnWithEndorsement("tx2", "ns2", hash, "org2", true, "c1").create()

	// Scenario: Block we got has been reprocessed and hence the sufficient pvtData is present
	// in the local pvtdataStore itself. The pvtData would be fetched from the local pvtdataStore.
	// If the coordinator tries fetching from the transientstore, or peers it would result in panic,
	// because we didn't define yet the "On(...)" invocation of the transient store or other peers.
	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").addRWSet().addNSRWSet("ns2", "c1").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(true, nil)

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    nil,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, nil, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
}

func TestProceedWithoutPrivateData(t *testing.T) {
	// Scenario: we are missing private data (c2 in ns3) and it cannot be obtained from any peer.
	// Block needs to be committed with missing private data.
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}
	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll().withMSPIdentity(identity.GetMSPIdentifier())
	var commitHappened bool
	assertCommitHappened := func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		blockAndPrivateData := args.Get(0).(*ledger.BlockAndPvtData)
		privateDataPassed2Ledger := blockAndPrivateData.PvtData
		require.True(t, reflect.DeepEqual(flattenTxPvtDataMap(privateDataPassed2Ledger),
			flattenTxPvtDataMap(expectedCommittedPrivateData2)))
		missingPrivateData := blockAndPrivateData.MissingPvtData
		expectedMissingPvtData := make(ledger.TxMissingPvtData)
		expectedMissingPvtData.Add(0, "ns3", "c2", true)
		require.Equal(t, expectedMissingPvtData, missingPrivateData)
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	store := newTransientStore(t)
	defer store.tearDown()

	assertPurged := func(txns ...string) {
		for _, txn := range txns {
			iterator, err := store.GetTxPvtRWSetByTxid(txn, nil)
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			res, err := iterator.Next()
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			require.Nil(t, res)
			iterator.Close()
		}
	}

	fetcher := &fetcherMock{t: t}
	// Have the peer return in response to the pull, a private data with a non matching hash
	fetcher.On("fetch", mock.Anything).expectingDigests([]privdatacommon.DigKey{
		{
			TxId: "tx1", Namespace: "ns3", Collection: "c2", BlockSeq: 1,
		},
	}).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: []*proto.PvtDataElement{
			{
				Digest: &proto.PvtDataDigest{
					BlockSeq:   1,
					Collection: "c2",
					Namespace:  "ns3",
					TxId:       "tx1",
				},
				Payload: [][]byte{[]byte("wrong pre-image")},
			},
		},
	}, nil)

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	pdFactory := &pvtDataFactory{}
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	block := bf.AddTxn("tx1", "ns3", hash, "c3", "c2").create()
	pvtData := pdFactory.addRWSet().addNSRWSet("ns3", "c3").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	assertCommitHappened()
	assertPurged("tx1")
}

func TestProceedWithInEligiblePrivateData(t *testing.T) {
	// Scenario: we are missing private data (c2 in ns3) and it cannot be obtained from any peer.
	// Block needs to be committed with missing private data.
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	cs := createcollectionStore(peerSelfSignedData).thatAcceptsNone().withMSPIdentity(identity.GetMSPIdentifier())

	var commitHappened bool
	assertCommitHappened := func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		blockAndPrivateData := args.Get(0).(*ledger.BlockAndPvtData)
		privateDataPassed2Ledger := blockAndPrivateData.PvtData
		require.True(t, reflect.DeepEqual(flattenTxPvtDataMap(privateDataPassed2Ledger),
			flattenTxPvtDataMap(expectedCommittedPrivateData3)))
		missingPrivateData := blockAndPrivateData.MissingPvtData
		expectedMissingPvtData := make(ledger.TxMissingPvtData)
		expectedMissingPvtData.Add(0, "ns3", "c2", false)
		require.Equal(t, expectedMissingPvtData, missingPrivateData)
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	block := bf.AddTxn("tx1", "ns3", hash, "c2").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            nil,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, nil, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, nil)
	require.NoError(t, err)
	assertCommitHappened()
}

func TestCoordinatorGetBlocks(t *testing.T) {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	store := newTransientStore(t)
	defer store.tearDown()

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	fetcher := &fetcherMock{t: t}

	committer := &privdatamocks.Committer{}
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	bf := &blockFactory{
		channelID: "testchannelid",
	}
	block := bf.AddTxn("tx1", "ns1", hash, "c1", "c2").AddTxn("tx2", "ns2", hash, "c1").create()

	// Green path - block and private data is returned, but the requester isn't eligible for all the private data,
	// but only to a subset of it.
	cs := createcollectionStore(peerSelfSignedData).thatAccepts(CollectionCriteria{
		Namespace:  "ns1",
		Collection: "c2",
		Channel:    "testchannelid",
	}).withMSPIdentity(identity.GetMSPIdentifier())
	committer.Mock = mock.Mock{}
	committer.On("GetPvtDataAndBlockByNum", mock.Anything).Return(&ledger.BlockAndPvtData{
		Block:   block,
		PvtData: expectedCommittedPrivateData1,
	}, nil)
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	expectedPrivData := (&pvtDataFactory{}).addRWSet().addNSRWSet("ns1", "c2").create()
	block2, returnedPrivateData, err := coordinator.GetPvtDataAndBlockByNum(1, peerSelfSignedData)
	require.NoError(t, err)
	require.Equal(t, block, block2)
	require.Equal(t, expectedPrivData, []*ledger.TxPvtData(returnedPrivateData))

	// Bad path - error occurs when trying to retrieve the block and private data
	committer.Mock = mock.Mock{}
	committer.On("GetPvtDataAndBlockByNum", mock.Anything).Return(nil, errors.New("uh oh"))
	block2, returnedPrivateData, err = coordinator.GetPvtDataAndBlockByNum(1, peerSelfSignedData)
	require.Nil(t, block2)
	require.Empty(t, returnedPrivateData)
	require.Error(t, err)
}

func TestPurgeBelowHeight(t *testing.T) {
	conf := testConfig
	conf.TransientBlockRetention = 5
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{}
	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll()

	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Return(nil)

	store := newTransientStore(t)
	defer store.tearDown()

	// store 9 data sets initially
	for i := 0; i < 9; i++ {
		txID := fmt.Sprintf("tx%d", i+1)
		store.Persist(txID, uint64(i), &tspb.TxPvtReadWriteSetWithConfigInfo{
			PvtRwset: &rwset.TxPvtReadWriteSet{
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "c1",
								Rwset:          []byte("rws-pre-image"),
							},
						},
					},
				},
			},
			CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
		})
	}
	assertPurged := func(purged bool) bool {
		numTx := 9
		if purged {
			numTx = 10
		}
		for i := 1; i <= numTx; i++ {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			if err != nil {
				iterator.Close()
				t.Fatalf("Failed iterating, got err %s", err)
			}
			res, err := iterator.Next()
			iterator.Close()
			if err != nil {
				t.Fatalf("Failed iterating, got err %s", err)
			}
			if (i < 6 || i == numTx) && purged {
				if res != nil {
					return false
				}
				continue
			}
			if res == nil {
				return false
			}
		}
		return true
	}

	fetcher := &fetcherMock{t: t}

	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	pdFactory := &pvtDataFactory{}

	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, conf, idDeserializerFactory)

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	block := bf.AddTxn("tx10", "ns1", hash, "c1").create()
	block.Header.Number = 10
	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1").create()
	// test no blocks purged yet
	assertPurgedBlocks := func() bool {
		return assertPurged(false)
	}
	require.Eventually(t, assertPurgedBlocks, 2*time.Second, 100*time.Millisecond)
	err := coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)
	// test first 6 blocks were purged
	assertPurgedBlocks = func() bool {
		return assertPurged(true)
	}
	require.Eventually(t, assertPurgedBlocks, 2*time.Second, 100*time.Millisecond)
}

func TestCoordinatorStorePvtData(t *testing.T) {
	mspID := "Org1MSP"
	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics
	cs := createcollectionStore(protoutil.SignedData{}).thatAcceptsAll()
	committer := &privdatamocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	fetcher := &fetcherMock{t: t}
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, protoutil.SignedData{}, metrics, testConfig, idDeserializerFactory)
	pvtData := (&pvtDataFactory{}).addRWSet().addNSRWSet("ns1", "c1").create()
	// Green path: ledger height can be retrieved from ledger/committer
	err := coordinator.StorePvtData("tx1", &tspb.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset:          pvtData[0].WriteSet,
		CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
	}, uint64(5))
	require.NoError(t, err)
}

func TestContainsWrites(t *testing.T) {
	// Scenario I: Nil HashedRwSet in collection
	col := &rwsetutil.CollHashedRwSet{
		CollectionName: "col1",
	}
	require.False(t, containsWrites("tx", "ns", col))

	// Scenario II: No writes in collection
	col.HashedRwSet = &kvrwset.HashedRWSet{}
	require.False(t, containsWrites("tx", "ns", col))

	// Scenario III: Some writes in collection
	col.HashedRwSet.HashedWrites = append(col.HashedRwSet.HashedWrites, &kvrwset.KVWriteHash{})
	require.True(t, containsWrites("tx", "ns", col))
}

func TestIgnoreReadOnlyColRWSets(t *testing.T) {
	// Scenario: The transaction has some ColRWSets that have only reads and no writes,
	// These should be ignored and not considered as missing private data that needs to be retrieved
	// from the transient store or other peers.
	// The gossip and transient store mocks in this test aren't initialized with
	// actions, so if the coordinator attempts to fetch private data from the
	// transient store or other peers, the test would fail.
	// Also - we check that at commit time - the coordinator concluded that
	// no missing private data was found.
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}
	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll().withMSPIdentity(identity.GetMSPIdentifier())
	var commitHappened bool
	assertCommitHappened := func() {
		require.True(t, commitHappened)
		commitHappened = false
	}
	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		blockAndPrivateData := args.Get(0).(*ledger.BlockAndPvtData)
		// Ensure there is no private data to commit
		require.Empty(t, blockAndPrivateData.PvtData)
		// Ensure there is no missing private data
		require.Empty(t, blockAndPrivateData.MissingPvtData)
		commitHappened = true

		commitOpts := args.Get(1).(*ledger.CommitOptions)
		expectedCommitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: false}
		require.Equal(t, expectedCommitOpts, commitOpts)
	}).Return(nil)

	store := newTransientStore(t)
	defer store.tearDown()

	fetcher := &fetcherMock{t: t}
	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	// The block contains a read only private data transaction
	block := bf.AddReadOnlyTxn("tx1", "ns3", hash, "c3", "c2").create()
	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	// We pass a nil private data slice to indicate no pre-images though the block contains
	// private data reads.
	err = coordinator.StoreBlock(block, nil)
	require.NoError(t, err)
	assertCommitHappened()
}

func TestCoordinatorMetrics(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))
	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))
	mspID := "Org1MSP"
	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	cs := createcollectionStore(peerSelfSignedData).thatAcceptsAll().withMSPIdentity(identity.GetMSPIdentifier())

	committer := &privdatamocks.Committer{}
	committer.On("CommitLegacy", mock.Anything, mock.Anything).Return(nil)

	store := newTransientStore(t)
	defer store.tearDown()

	hash := util2.ComputeSHA256([]byte("rws-pre-image"))
	pdFactory := &pvtDataFactory{}
	bf := &blockFactory{
		channelID: "testchannelid",
	}

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain("testchannelid")
	})

	block := bf.AddTxnWithEndorsement("tx1", "ns1", hash, "org1", true, "c1", "c2").
		AddTxnWithEndorsement("tx2", "ns2", hash, "org2", true, "c1").
		AddTxnWithEndorsement("tx3", "ns3", hash, "org3", true, "c1").create()

	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").addRWSet().addNSRWSet("ns2", "c1").create()
	// fetch duration metric only reported when fetching from remote peer
	fetcher := &fetcherMock{t: t}
	fetcher.On("fetch", mock.Anything).expectingDigests([]privdatacommon.DigKey{
		{
			TxId: "tx3", Namespace: "ns3", Collection: "c1", BlockSeq: 1, SeqInBlock: 2,
		},
	}).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: []*proto.PvtDataElement{
			{
				Digest: &proto.PvtDataDigest{
					SeqInBlock: 2,
					BlockSeq:   1,
					Collection: "c1",
					Namespace:  "ns3",
					TxId:       "tx3",
				},
				Payload: [][]byte{[]byte("rws-pre-image")},
			},
		},
	}, nil)

	testMetricProvider := gmetricsmocks.TestUtilConstructMetricProvider()
	metrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).PrivdataMetrics

	committer.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)

	capabilityProvider := &privdatamocks.CapabilityProvider{}
	appCapability := &privdatamocks.ApplicationCapabilities{}
	capabilityProvider.On("Capabilities").Return(appCapability)
	appCapability.On("StorePvtDataOfInvalidTx").Return(true)
	coordinator := NewCoordinator(mspID, Support{
		ChainID:            "testchannelid",
		CollectionStore:    cs,
		Committer:          committer,
		Fetcher:            fetcher,
		Validator:          &validatorMock{},
		CapabilityProvider: capabilityProvider,
	}, store.store, peerSelfSignedData, metrics, testConfig, idDeserializerFactory)
	err = coordinator.StoreBlock(block, pvtData)
	require.NoError(t, err)

	// make sure all coordinator metrics were reported

	require.Equal(t,
		[]string{"channel", "testchannelid"},
		testMetricProvider.FakeValidationDuration.WithArgsForCall(0),
	)
	require.True(t, testMetricProvider.FakeValidationDuration.ObserveArgsForCall(0) > 0)
	require.Equal(t,
		[]string{"channel", "testchannelid"},
		testMetricProvider.FakeListMissingPrivateDataDuration.WithArgsForCall(0),
	)
	require.True(t, testMetricProvider.FakeListMissingPrivateDataDuration.ObserveArgsForCall(0) > 0)
	require.Equal(t,
		[]string{"channel", "testchannelid"},
		testMetricProvider.FakeFetchDuration.WithArgsForCall(0),
	)
	// fetch duration metric only reported when fetching from remote peer
	require.True(t, testMetricProvider.FakeFetchDuration.ObserveArgsForCall(0) > 0)
	require.Equal(t,
		[]string{"channel", "testchannelid"},
		testMetricProvider.FakeCommitPrivateDataDuration.WithArgsForCall(0),
	)
	require.True(t, testMetricProvider.FakeCommitPrivateDataDuration.ObserveArgsForCall(0) > 0)
	require.Equal(t,
		[]string{"channel", "testchannelid"},
		testMetricProvider.FakePurgeDuration.WithArgsForCall(0),
	)

	purgeDuration := func() bool {
		return testMetricProvider.FakePurgeDuration.ObserveArgsForCall(0) > 0
	}
	require.Eventually(t, purgeDuration, 2*time.Second, 100*time.Millisecond)
}
