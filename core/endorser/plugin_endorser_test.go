/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/endorser/mocks"
	"github.com/hyperledger/fabric/core/handlers/endorsement/api"
	. "github.com/hyperledger/fabric/core/handlers/endorsement/api/state"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/peer"
	transientstore2 "github.com/hyperledger/fabric/protos/transientstore"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockTransientStoreRetriever = transientStoreRetriever()
	mockTransientStore          = &mocks.Store{}
)

func TestPluginEndorserNotFound(t *testing.T) {
	pluginMapper := &mocks.PluginMapper{}
	pluginMapper.On("PluginFactoryByName", endorser.PluginName("notfound")).Return(nil)
	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		PluginMapper: pluginMapper,
	})
	resp, err := pluginEndorser.EndorseWithPlugin(endorser.Context{
		Response:   &peer.Response{},
		PluginName: "notfound",
	})
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "plugin with name notfound wasn't found")
}

func TestPluginEndorserGreenPath(t *testing.T) {
	proposal, _, err := utils.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, "mychannel", &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: "mycc"},
		},
	}, []byte{1, 2, 3})
	assert.NoError(t, err)
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
	ctx := endorser.Context{
		Response:   &peer.Response{},
		PluginName: "plugin",
		Proposal:   proposal,
		ChaincodeID: &peer.ChaincodeID{
			Name: "mycc",
		},
		Channel: "mychannel",
	}

	// Scenario I: Call the endorsement for the first time
	resp, err := pluginEndorser.EndorseWithPlugin(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, expectedSignature, resp.Endorsement.Signature)
	assert.Equal(t, expectedProposalResponsePayload, resp.Payload)
	// Ensure both state and SigningIdentityFetcher were passed to Init()
	plugin.AssertCalled(t, "Init", &endorser.ChannelState{QueryCreator: queryCreator, Store: mockTransientStore}, sif)

	// Scenario II: Call the endorsement again a second time.
	// Ensure the plugin wasn't instantiated again - which means the same instance
	// was used to service the request.
	// Also - check that the Init() wasn't called more than once on the plugin.
	resp, err = pluginEndorser.EndorseWithPlugin(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, expectedSignature, resp.Endorsement.Signature)
	assert.Equal(t, expectedProposalResponsePayload, resp.Payload)
	pluginFactory.AssertNumberOfCalls(t, "New", 1)
	plugin.AssertNumberOfCalls(t, "Init", 1)

	// Scenario III: Call the endorsement with a channel-less context.
	// The init method should be called again, but this time - a channel state object
	// should not be passed into the init.
	ctx.Channel = ""
	pluginFactory.On("New").Return(plugin).Once()
	plugin.On("Init", mock.Anything).Return(nil).Once()
	resp, err = pluginEndorser.EndorseWithPlugin(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, expectedSignature, resp.Endorsement.Signature)
	assert.Equal(t, expectedProposalResponsePayload, resp.Payload)
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

	// Scenario I: Failed initializing plugin
	t.Run("PluginInitializationFailure", func(t *testing.T) {
		plugin.On("Init", mock.Anything, mock.Anything).Return(errors.New("plugin initialization failed")).Once()
		resp, err := pluginEndorser.EndorseWithPlugin(endorser.Context{
			PluginName: "plugin",
			Channel:    "mychannel",
			Response:   &peer.Response{},
		})
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "plugin initialization failed")
	})

	// Scenario II: an empty proposal is passed in the context, and parsing fails
	t.Run("EmptyProposal", func(t *testing.T) {
		plugin.On("Init", mock.Anything, mock.Anything).Return(nil).Once()
		ctx := endorser.Context{
			Response:   &peer.Response{},
			PluginName: "plugin",
			ChaincodeID: &peer.ChaincodeID{
				Name: "mycc",
			},
			Proposal: &peer.Proposal{},
			Channel:  "mychannel",
		}
		resp, err := pluginEndorser.EndorseWithPlugin(ctx)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "could not compute proposal hash")
	})

	// Scenario III: The proposal's header is invalid
	t.Run("InvalidHeader in the proposal", func(t *testing.T) {
		ctx := endorser.Context{
			Response:   &peer.Response{},
			PluginName: "plugin",
			ChaincodeID: &peer.ChaincodeID{
				Name: "mycc",
			},
			Proposal: &peer.Proposal{
				Header: []byte{1, 2, 3},
			},
			Channel: "mychannel",
		}
		resp, err := pluginEndorser.EndorseWithPlugin(ctx)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed parsing header")
	})

	// Scenario IV: The proposal's response status code indicates an error
	t.Run("ResponseStatusContainsError", func(t *testing.T) {
		r := &peer.Response{
			Status:  shim.ERRORTHRESHOLD,
			Payload: []byte{1, 2, 3},
			Message: "bla bla",
		}
		resp, err := pluginEndorser.EndorseWithPlugin(endorser.Context{
			Response: r,
		})
		assert.Equal(t, &peer.ProposalResponse{Response: r}, resp)
		assert.NoError(t, err)
	})

	// Scenario V: The proposal's response is nil
	t.Run("ResponseIsNil", func(t *testing.T) {
		resp, err := pluginEndorser.EndorseWithPlugin(endorser.Context{})
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "response is nil")
	})
}

func transientStoreRetriever() *mocks.TransientStoreRetriever {
	storeRetriever := &mocks.TransientStoreRetriever{}
	storeRetriever.On("StoreForChannel", mock.Anything).Return(mockTransientStore)
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

type rwsetScanner struct {
	mock.Mock
	data []*rwset.TxPvtReadWriteSet
}

func (*rwsetScanner) Next() (*transientstore.EndorserPvtSimulationResults, error) {
	panic("implement me")
}

func (rws *rwsetScanner) NextWithConfig() (*transientstore.EndorserPvtSimulationResultsWithConfig, error) {
	if len(rws.data) == 0 {
		return nil, nil
	}
	res := rws.data[0]
	rws.data = rws.data[1:]
	return &transientstore.EndorserPvtSimulationResultsWithConfig{
		PvtSimulationResultsWithConfig: &transientstore2.TxPvtReadWriteSetWithConfigInfo{
			PvtRwset: res,
		},
	}, nil
}

func (rws *rwsetScanner) Close() {
	rws.Called()
}

func TestTransientStore(t *testing.T) {
	plugin := &fakeEndorsementPlugin{}
	factory := &mocks.PluginFactory{}
	factory.On("New").Return(plugin)
	sif := &mocks.SigningIdentityFetcher{}
	cs := &mocks.ChannelStateRetriever{}
	queryCreator := &mocks.QueryCreator{}
	queryCreator.On("NewQueryExecutor").Return(&ledger.MockQueryExecutor{}, nil)
	cs.On("NewQueryCreator", "mychannel").Return(queryCreator, nil)

	transientStore := &mocks.Store{}
	storeRetriever := &mocks.TransientStoreRetriever{}
	storeRetriever.On("StoreForChannel", mock.Anything).Return(transientStore)

	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		ChannelStateRetriever:  cs,
		SigningIdentityFetcher: sif,
		PluginMapper: endorser.MapBasedPluginMapper{
			"plugin": factory,
		},
		TransientStoreRetriever: storeRetriever,
	})

	proposal, _, err := utils.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, "mychannel", &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: "mycc"},
		},
	}, []byte{1, 2, 3})
	assert.NoError(t, err)
	ctx := endorser.Context{
		Response:   &peer.Response{},
		PluginName: "plugin",
		Proposal:   proposal,
		ChaincodeID: &peer.ChaincodeID{
			Name: "mycc",
		},
		Channel: "mychannel",
	}

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
	scanner := &rwsetScanner{
		data: []*rwset.TxPvtReadWriteSet{rws},
	}
	scanner.On("Close")

	transientStore.On("GetTxPvtRWSetByTxid", mock.Anything, mock.Anything).Return(scanner, nil)

	resp, err := pluginEndorser.EndorseWithPlugin(ctx)
	assert.NoError(t, err)

	txrws := &rwset.TxPvtReadWriteSet{}
	err = proto.Unmarshal(resp.Payload, txrws)
	assert.NoError(t, err)
	assert.True(t, proto.Equal(rws, txrws))
	scanner.AssertCalled(t, "Close")
}
