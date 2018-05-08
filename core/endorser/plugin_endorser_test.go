/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/endorser/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPluginEndorserNotFound(t *testing.T) {
	pluginMapper := &mocks.PluginMapper{}
	pluginMapper.On("PluginFactoryByName", endorser.PluginName("notfound")).Return(nil)
	pluginEndorser := endorser.NewPluginEndorser(nil, nil, pluginMapper)
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
	pluginEndorser := endorser.NewPluginEndorser(cs, sif, pluginMapper)
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
	plugin.AssertCalled(t, "Init", &endorser.ChannelState{queryCreator}, sif)

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
	pluginEndorser := endorser.NewPluginEndorser(cs, sif, pluginMapper)

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
		assert.Contains(t, err.Error(), "Response is nil")

	})
}
