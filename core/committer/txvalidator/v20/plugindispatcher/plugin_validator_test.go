/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plugindispatcher_test

import (
	errors2 "errors"
	"sync"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	tmocks "github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/testdata"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/msp"
	. "github.com/hyperledger/fabric/msp/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestValidateWithPlugin(t *testing.T) {
	pm := make(plugin.MapBasedMapper)
	qec := &mocks.QueryExecutorCreator{}
	deserializer := &mocks.IdentityDeserializer{}
	capabilities := &mocks.Capabilities{}

	mcpmg := &mocks.ChannelPolicyManagerGetter{}
	mcpmg.On("Manager", "").Return(&mocks.PolicyManager{})

	v := plugindispatcher.NewPluginValidator(pm, qec, deserializer, capabilities, mcpmg, nil)
	ctx := &plugindispatcher.Context{
		Namespace:  "mycc",
		PluginName: "vscc",
	}

	// Scenario I: The plugin isn't found because the map wasn't populated with anything yet
	err := v.ValidateWithPlugin(ctx)
	require.Contains(t, err.Error(), "plugin with name vscc wasn't found")

	// Scenario II: The plugin initialization fails
	factory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("foo")).Once()
	factory.On("New").Return(plugin)
	pm["vscc"] = factory
	err = v.ValidateWithPlugin(ctx)
	cerr, _ := errors2.AsType[*validation.ExecutionFailureError](err)
	require.Contains(t, cerr.Error(), "failed initializing plugin: foo")

	// Scenario III: The plugin initialization succeeds but an execution error occurs.
	// The plugin should pass the error as is.
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	validationErr := &validation.ExecutionFailureError{
		Reason: "bar",
	}
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(validationErr).Once()
	err = v.ValidateWithPlugin(ctx)
	require.Equal(t, validationErr, err)

	// Scenario IV: The plugin initialization succeeds and the validation passes
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	err = v.ValidateWithPlugin(ctx)
	require.NoError(t, err)
}

func TestSamplePlugin(t *testing.T) {
	pm := make(plugin.MapBasedMapper)

	qe := &tmocks.QueryExecutor{}
	state := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	qe.On("GetState", "lscc", "mycc").Return(state, nil)
	qe.On("GetStateMultipleKeys", "lscc", []string{"mycc"}).Return([][]byte{state}, nil)
	qe.On("Done").Return(nil, nil)

	qec := &mocks.QueryExecutorCreator{}
	qec.On("NewQueryExecutor").Return(qe, nil)

	identity := &MockIdentity{}
	identity.On("GetIdentifier").Return(&msp.IdentityIdentifier{
		Mspid: "SampleOrg",
		Id:    "foo",
	})
	deserializer := &mocks.IdentityDeserializer{}
	deserializer.On("DeserializeIdentity", []byte{7, 8, 9}).Return(identity, nil)
	capabilities := &mocks.Capabilities{}
	capabilities.On("PrivateChannelData").Return(true)
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

	v := plugindispatcher.NewPluginValidator(pm, qec, deserializer, capabilities, mcpmg, nil)
	acceptAllPolicyBytes, _ := proto.Marshal(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: policydsl.AcceptAllPolicy}})
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
	require.NoError(t, v.ValidateWithPlugin(ctx))
}

// recordedInvalidTxSignal is an invalid-transaction signal delivered to a plugin
type recordedInvalidTxSignal struct {
	blockNum, txNum uint64
	err             error
}

// signalingPlugin is a validation plugin that records the invalid-transaction
// signals delivered to it
type signalingPlugin struct {
	mutex   sync.Mutex
	signals []recordedInvalidTxSignal
}

func (p *signalingPlugin) Validate(block *common.Block, namespace string, txPosition int, actionPosition int, contextData ...validation.ContextDatum) error {
	return nil
}

func (p *signalingPlugin) Init(dependencies ...validation.Dependency) error {
	return nil
}

func (p *signalingPlugin) SignalInvalidTx(blockNum, txNum uint64, err error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.signals = append(p.signals, recordedInvalidTxSignal{blockNum: blockNum, txNum: txNum, err: err})
}

func (p *signalingPlugin) recordedSignals() []recordedInvalidTxSignal {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return append([]recordedInvalidTxSignal(nil), p.signals...)
}

func newSignalingPluginValidator() (*plugindispatcher.PluginValidator, *signalingPlugin) {
	pm := make(plugin.MapBasedMapper)
	rec := &signalingPlugin{}
	factory := &mocks.PluginFactory{}
	factory.On("New").Return(rec)
	pm["vscc"] = factory
	mcpmg := &mocks.ChannelPolicyManagerGetter{}
	mcpmg.On("Manager", mock.Anything).Return(&mocks.PolicyManager{})
	v := plugindispatcher.NewPluginValidator(pm, &mocks.QueryExecutorCreator{}, &mocks.IdentityDeserializer{}, &mocks.Capabilities{}, mcpmg, nil)
	return v, rec
}

func blockContext(blockNum uint64) *plugindispatcher.Context {
	return &plugindispatcher.Context{
		Namespace:  "mycc",
		PluginName: "vscc",
		Channel:    "mychannel",
		Block: &common.Block{
			Header: &common.BlockHeader{Number: blockNum},
			Data:   &common.BlockData{},
		},
	}
}

func TestSignalInvalidTxReplayedToLazilyCreatedPlugin(t *testing.T) {
	v, rec := newSignalingPluginValidator()
	errRejected := errors.New("rejected")

	// the signal arrives before any plugin instance exists for the channel
	v.SignalInvalidTx("mychannel", 2, 0, errRejected)
	require.Empty(t, rec.recordedSignals())

	// the plugin instance is created lazily during validation of block 2 and
	// is caught up on the pending signal
	require.NoError(t, v.ValidateWithPlugin(blockContext(2)))
	require.Equal(t, []recordedInvalidTxSignal{{blockNum: 2, txNum: 0, err: errRejected}}, rec.recordedSignals())
}

func TestStaleSignalNotReplayedToPluginCreatedForLaterBlock(t *testing.T) {
	v, rec := newSignalingPluginValidator()
	errRejected := errors.New("rejected")

	// a signal for block 2 is pending while no plugin instance exists yet
	v.SignalInvalidTx("mychannel", 2, 0, errRejected)

	// the plugin instance is only created during validation of block 3; the
	// pending signal of block 2 is stale and must not be replayed to an
	// instance whose validation context may already have moved to block 3
	require.NoError(t, v.ValidateWithPlugin(blockContext(3)))
	require.Empty(t, rec.recordedSignals())

	// signals of the block under validation are still delivered
	v.SignalInvalidTx("mychannel", 3, 1, errRejected)
	require.Equal(t, []recordedInvalidTxSignal{{blockNum: 3, txNum: 1, err: errRejected}}, rec.recordedSignals())
}

func TestSignalInvalidTxDeliveredRepeatedly(t *testing.T) {
	v, rec := newSignalingPluginValidator()
	errRejected := errors.New("rejected")

	require.NoError(t, v.ValidateWithPlugin(blockContext(2)))

	// duplicate signals for the same transaction are all delivered;
	// idempotent handling of them is the plugin's documented contract
	v.SignalInvalidTx("mychannel", 2, 0, errRejected)
	v.SignalInvalidTx("mychannel", 2, 0, errRejected)
	require.Equal(t, []recordedInvalidTxSignal{
		{blockNum: 2, txNum: 0, err: errRejected},
		{blockNum: 2, txNum: 0, err: errRejected},
	}, rec.recordedSignals())
}
