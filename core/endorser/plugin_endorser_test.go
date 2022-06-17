/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	tspb "github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/endorser/fake"
	"github.com/hyperledger/fabric/core/endorser/mocks"
	endorsement "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	. "github.com/hyperledger/fabric/core/handlers/endorsement/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/privdata"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var mockTransientStoreRetriever = transientStoreRetriever()

//go:generate counterfeiter -o fake/query_executor.go -fake-name QueryExecutor . queryExecutor
type queryExecutor interface {
	ledger.QueryExecutor
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
	s.store, err = s.storeProvider.OpenStore("test")
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

func (s *testTransientStore) GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (privdata.RWSetScanner, error) {
	return s.store.GetTxPvtRWSetByTxid(txid, filter)
}

func TestPluginEndorserNotFound(t *testing.T) {
	pluginMapper := &mocks.PluginMapper{}
	pluginMapper.On("PluginFactoryByName", endorser.PluginName("notfound")).Return(nil)
	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		PluginMapper: pluginMapper,
	})
	endorsement, prpBytes, err := pluginEndorser.EndorseWithPlugin("notfound", "", nil, nil)
	require.Nil(t, endorsement)
	require.Nil(t, prpBytes)
	require.Contains(t, err.Error(), "plugin with name notfound wasn't found")
}

func TestPluginEndorserGreenPath(t *testing.T) {
	expectedSignature := []byte{5, 4, 3, 2, 1}
	expectedProposalResponsePayload := []byte{1, 2, 3}
	pluginMapper := &mocks.PluginMapper{}
	pluginFactory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	plugin.On("Endorse", mock.Anything, mock.Anything).Return(&peer.Endorsement{Signature: expectedSignature}, expectedProposalResponsePayload, nil)
	pluginMapper.On("PluginFactoryByName", endorser.PluginName("plugin")).Return(pluginFactory)
	// Expect that the plugin would be instantiated only once in this test, because we call the endorser with the same arguments
	plugin.On("Init", mock.Anything, mock.Anything).Return(nil).Once()
	pluginFactory.On("New").Return(plugin).Once()
	sif := &mocks.SigningIdentityFetcher{}
	cs := &mocks.ChannelStateRetriever{}
	queryCreator := &mocks.QueryCreator{}
	cs.On("NewQueryCreator", "mychannel").Return(queryCreator, nil)
	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		ChannelStateRetriever:   cs,
		SigningIdentityFetcher:  sif,
		PluginMapper:            pluginMapper,
		TransientStoreRetriever: mockTransientStoreRetriever,
	})

	// Scenario I: Call the endorsement for the first time
	endorsement, prpBytes, err := pluginEndorser.EndorseWithPlugin("plugin", "mychannel", nil, nil)
	require.NoError(t, err)
	require.Equal(t, expectedSignature, endorsement.Signature)
	require.Equal(t, expectedProposalResponsePayload, prpBytes)
	// Ensure both state and SigningIdentityFetcher were passed to Init()
	plugin.AssertCalled(t, "Init", &endorser.ChannelState{QueryCreator: queryCreator, Store: &transientstore.Store{}}, sif)

	// Scenario II: Call the endorsement again a second time.
	// Ensure the plugin wasn't instantiated again - which means the same instance
	// was used to service the request.
	// Also - check that the Init() wasn't called more than once on the plugin.
	endorsement, prpBytes, err = pluginEndorser.EndorseWithPlugin("plugin", "mychannel", nil, nil)
	require.NoError(t, err)
	require.Equal(t, expectedSignature, endorsement.Signature)
	require.Equal(t, expectedProposalResponsePayload, prpBytes)
	pluginFactory.AssertNumberOfCalls(t, "New", 1)
	plugin.AssertNumberOfCalls(t, "Init", 1)

	// Scenario III: Call the endorsement with a channel-less context.
	// The init method should be called again, but this time - a channel state object
	// should not be passed into the init.
	pluginFactory.On("New").Return(plugin).Once()
	plugin.On("Init", mock.Anything).Return(nil).Once()
	endorsement, prpBytes, err = pluginEndorser.EndorseWithPlugin("plugin", "", nil, nil)
	require.NoError(t, err)
	require.Equal(t, expectedSignature, endorsement.Signature)
	require.Equal(t, expectedProposalResponsePayload, prpBytes)
	plugin.AssertCalled(t, "Init", sif)
}

func TestPluginEndorserErrors(t *testing.T) {
	pluginMapper := &mocks.PluginMapper{}
	pluginFactory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	plugin.On("Endorse", mock.Anything, mock.Anything)
	pluginMapper.On("PluginFactoryByName", endorser.PluginName("plugin")).Return(pluginFactory)
	pluginFactory.On("New").Return(plugin)
	sif := &mocks.SigningIdentityFetcher{}
	cs := &mocks.ChannelStateRetriever{}
	queryCreator := &mocks.QueryCreator{}
	cs.On("NewQueryCreator", "mychannel").Return(queryCreator, nil)
	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		ChannelStateRetriever:   cs,
		SigningIdentityFetcher:  sif,
		PluginMapper:            pluginMapper,
		TransientStoreRetriever: mockTransientStoreRetriever,
	})

	// Failed initializing plugin
	t.Run("PluginInitializationFailure", func(t *testing.T) {
		plugin.On("Init", mock.Anything, mock.Anything).Return(errors.New("plugin initialization failed")).Once()
		endorsement, prpBytes, err := pluginEndorser.EndorseWithPlugin("plugin", "mychannel", nil, nil)
		require.Nil(t, endorsement)
		require.Nil(t, prpBytes)
		require.Contains(t, err.Error(), "plugin initialization failed")
	})
}

func transientStoreRetriever() *mocks.TransientStoreRetriever {
	storeRetriever := &mocks.TransientStoreRetriever{}
	storeRetriever.On("StoreForChannel", mock.Anything).Return(&transientstore.Store{})
	return storeRetriever
}

type fakeEndorsementPlugin struct {
	StateFetcher
}

func (fep *fakeEndorsementPlugin) Endorse(payload []byte, sp *peer.SignedProposal) (*peer.Endorsement, []byte, error) {
	state, _ := fep.StateFetcher.FetchState()
	txrws, _ := state.GetTransientByTXID("tx")
	b, _ := proto.Marshal(txrws[0])
	return nil, b, nil
}

func (fep *fakeEndorsementPlugin) Init(dependencies ...endorsement.Dependency) error {
	for _, dep := range dependencies {
		if state, isState := dep.(StateFetcher); isState {
			fep.StateFetcher = state
			return nil
		}
	}
	panic("could not find State dependency")
}

func TestTransientStore(t *testing.T) {
	plugin := &fakeEndorsementPlugin{}
	factory := &mocks.PluginFactory{}
	factory.On("New").Return(plugin)
	sif := &mocks.SigningIdentityFetcher{}
	cs := &mocks.ChannelStateRetriever{}
	queryCreator := &mocks.QueryCreator{}
	queryCreator.On("NewQueryExecutor").Return(&fake.QueryExecutor{}, nil)
	cs.On("NewQueryCreator", "mychannel").Return(queryCreator, nil)

	transientStore := newTransientStore(t)
	defer transientStore.tearDown()
	rws := &rwset.TxPvtReadWriteSet{
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "col",
					},
				},
			},
		},
	}

	transientStore.Persist("tx1", 1, &tspb.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset:          rws,
		CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
	})

	storeRetriever := &mocks.TransientStoreRetriever{}
	storeRetriever.On("StoreForChannel", mock.Anything).Return(transientStore.store)

	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		ChannelStateRetriever:  cs,
		SigningIdentityFetcher: sif,
		PluginMapper: endorser.MapBasedPluginMapper{
			"plugin": factory,
		},
		TransientStoreRetriever: storeRetriever,
	})

	_, prpBytes, err := pluginEndorser.EndorseWithPlugin("plugin", "mychannel", nil, nil)
	require.NoError(t, err)

	txrws := &rwset.TxPvtReadWriteSet{}
	err = proto.Unmarshal(prpBytes, txrws)
	require.NoError(t, err)
	require.True(t, proto.Equal(rws, txrws))
}
