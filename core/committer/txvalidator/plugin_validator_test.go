/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator_test

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/testdata"
	"github.com/hyperledger/fabric/core/handlers/validation/api"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	"github.com/hyperledger/fabric/msp"
	. "github.com/hyperledger/fabric/msp/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestValidateWithPlugin(t *testing.T) {
	pm := make(txvalidator.MapBasedPluginMapper)
	qec := &mocks.QueryExecutorCreator{}
	deserializer := &mocks.IdentityDeserializer{}
	capabilites := &mocks.Capabilities{}
	v := txvalidator.NewPluginValidator(pm, qec, deserializer, capabilites)
	ctx := &txvalidator.Context{
		Namespace: "mycc",
		VSCCName:  "vscc",
	}

	// Scenario I: The plugin isn't found because the map wasn't populated with anything yet
	err := v.ValidateWithPlugin(ctx)
	assert.Contains(t, err.Error(), "plugin with name vscc wasn't found")

	// Scenario II: The plugin initialization fails
	factory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("foo")).Once()
	factory.On("New").Return(plugin)
	pm["vscc"] = factory
	err = v.ValidateWithPlugin(ctx)
	assert.Contains(t, err.(*validation.ExecutionFailureError).Error(), "failed initializing plugin: foo")

	// Scenario III: The plugin initialization succeeds but an execution error occurs.
	// The plugin should pass the error as is.
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	validationErr := &validation.ExecutionFailureError{
		Reason: "bar",
	}
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(validationErr).Once()
	err = v.ValidateWithPlugin(ctx)
	assert.Equal(t, validationErr, err)

	// Scenario IV: The plugin initialization succeeds and the validation passes
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	err = v.ValidateWithPlugin(ctx)
	assert.NoError(t, err)
}

func TestSamplePlugin(t *testing.T) {
	pm := make(txvalidator.MapBasedPluginMapper)
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

	transaction := testdata.MarshaledSignedData{
		Data:      []byte{1, 2, 3},
		Signature: []byte{4, 5, 6},
		Identity:  []byte{7, 8, 9},
	}

	txnData, _ := proto.Marshal(&transaction)

	v := txvalidator.NewPluginValidator(pm, qec, deserializer, capabilites)
	acceptAllPolicyBytes, _ := proto.Marshal(cauthdsl.AcceptAllPolicy)
	ctx := &txvalidator.Context{
		Namespace: "mycc",
		VSCCName:  "vscc",
		Policy:    acceptAllPolicyBytes,
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

func TestCapabilitiesInterface(t *testing.T) {
	// Make sure that the application capabilities are all implemented by the validation capabilities
	// Obtain all methods of the ApplicationCapabilities and ensure
	// that every method in ApplicationCapabilities exists in the validation capabilities
	var appCapabilities *channelconfig.ApplicationCapabilities
	appMeta := reflect.TypeOf(appCapabilities).Elem()

	var validationCapabilities *Capabilities
	validationMeta := reflect.TypeOf(validationCapabilities).Elem()
	for i := 0; i < appMeta.NumMethod(); i++ {
		method := appMeta.Method(i).Name
		_, exists := validationMeta.MethodByName(method)
		assert.True(t, exists, "method %s doesn't exist", method)
	}
}
