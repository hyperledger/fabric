/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plugindispatcher_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/testdata"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/msp"
	. "github.com/hyperledger/fabric/msp/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestValidateWithPlugin(t *testing.T) {
	pm := make(plugin.MapBasedMapper)
	qec := &mocks.QueryExecutorCreator{}
	deserializer := &mocks.IdentityDeserializer{}
	capabilites := &mocks.Capabilities{}

	mcpmg := &mocks.ChannelPolicyManagerGetter{}
	mcpmg.On("Manager", "").Return(&mocks.PolicyManager{})

	v := plugindispatcher.NewPluginValidator(pm, qec, deserializer, capabilites, mcpmg, nil)
	ctx := &plugindispatcher.Context{
		Namespace:  "mycc",
		PluginName: "vscc",
	}

	// Scenario I: The plugin isn't found because the map wasn't populated with anything yet
	err := v.ValidateWithPlugin(ctx)
	assert.Contains(t, err.Error(), "plugin with name vscc wasn't found")

	// Scenario II: The plugin initialization fails
	factory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("foo")).Once()
	factory.On("New").Return(plugin)
	pm["vscc"] = factory
	err = v.ValidateWithPlugin(ctx)
	assert.Contains(t, err.(*validation.ExecutionFailureError).Error(), "failed initializing plugin: foo")

	// Scenario III: The plugin initialization succeeds but an execution error occurs.
	// The plugin should pass the error as is.
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	validationErr := &validation.ExecutionFailureError{
		Reason: "bar",
	}
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(validationErr).Once()
	err = v.ValidateWithPlugin(ctx)
	assert.Equal(t, validationErr, err)

	// Scenario IV: The plugin initialization succeeds and the validation passes
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	err = v.ValidateWithPlugin(ctx)
	assert.NoError(t, err)
}

func TestSamplePlugin(t *testing.T) {
	pm := make(plugin.MapBasedMapper)
	qec := &mocks.QueryExecutorCreator{}

	qec.On("NewQueryExecutor").Return(&ledger.MockQueryExecutor{
		State: map[string]map[string][]byte{
			"lscc": {
				"mycc": []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
		},
	}, nil)

	deserializer := &mocks.IdentityDeserializer{}
	identity := &MockIdentity{}
	identity.On("GetIdentifier").Return(&msp.IdentityIdentifier{
		Mspid: "SampleOrg",
		Id:    "foo",
	})
	deserializer.On("DeserializeIdentity", []byte{7, 8, 9}).Return(identity, nil)
	capabilites := &mocks.Capabilities{}
	capabilites.On("PrivateChannelData").Return(true)
	factory := &mocks.PluginFactory{}
	factory.On("New").Return(testdata.NewSampleValidationPlugin(t))
	pm["vscc"] = factory

	mcpmg := &mocks.ChannelPolicyManagerGetter{}
	mcpmg.On("Manager", "mychannel").Return(&mocks.PolicyManager{})

	transaction := testdata.MarshaledSignedData{
		Data:      []byte{1, 2, 3},
		Signature: []byte{4, 5, 6},
		Identity:  []byte{7, 8, 9},
	}

	txnData, _ := proto.Marshal(&transaction)

	v := plugindispatcher.NewPluginValidator(pm, qec, deserializer, capabilites, mcpmg, nil)
	acceptAllPolicyBytes, _ := proto.Marshal(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: cauthdsl.AcceptAllPolicy}})
	ctx := &plugindispatcher.Context{
		Namespace:  "mycc",
		PluginName: "vscc",
		Policy:     acceptAllPolicyBytes,
		Block: &common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{txnData},
			},
		},
		Channel: "mychannel",
	}
	assert.NoError(t, v.ValidateWithPlugin(ctx))
}
