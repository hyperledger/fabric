/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestDispatchStep(t *testing.T) {
	expectedRequest := &orderer.StepRequest{
		Channel: "ignored value",
	}

	mr := &mocks.MessageReceiver{}
	mr.On("Step", expectedRequest, uint64(1)).Return(nil).Once()

	rg := &mocks.ReceiverGetter{}
	rg.On("ReceiverByChain", "mychannel").Return(mr).Once()
	rg.On("ReceiverByChain", "notmychannel").Return(nil).Once()

	disp := &etcdraft.Dispatcher{ChainSelector: rg, Logger: flogging.MustGetLogger("test")}

	t.Run("Channel exists", func(t *testing.T) {
		_, err := disp.OnStep("mychannel", 1, expectedRequest)
		assert.NoError(t, err)
	})

	t.Run("Channel does not exist", func(t *testing.T) {
		_, err := disp.OnStep("notmychannel", 1, expectedRequest)
		assert.EqualError(t, err, "channel notmychannel doesn't exist")
	})
}

func TestDispatchSubmit(t *testing.T) {
	expectedRequest := &orderer.SubmitRequest{
		Channel: "ignored value - success",
	}

	expectedRequestForBackendError := &orderer.SubmitRequest{
		Channel: "ignored value - backend error",
	}

	expectedWrongChannelError := &orderer.SubmitResponse{
		Info:   "channel notmychannel doesn't exist",
		Status: common.Status_NOT_FOUND,
	}

	expectedWrongBackendError := &orderer.SubmitResponse{
		Info:   "backend error",
		Status: common.Status_INTERNAL_SERVER_ERROR,
	}

	mr := &mocks.MessageReceiver{}
	mr.On("Submit", expectedRequest, uint64(1)).Return(nil).Once()
	mr.On("Submit", expectedRequestForBackendError, uint64(1)).Return(errors.New("backend error")).Once()

	rg := &mocks.ReceiverGetter{}
	rg.On("ReceiverByChain", "mychannel").Return(mr).Twice()
	rg.On("ReceiverByChain", "notmychannel").Return(nil).Once()

	disp := &etcdraft.Dispatcher{ChainSelector: rg, Logger: flogging.MustGetLogger("test")}

	t.Run("Channel exists", func(t *testing.T) {
		_, err := disp.OnSubmit("mychannel", 1, expectedRequest)
		assert.NoError(t, err)
	})

	t.Run("Channel does not exist", func(t *testing.T) {
		res, err := disp.OnSubmit("notmychannel", 1, expectedRequest)
		assert.NoError(t, err)
		assert.Equal(t, expectedWrongChannelError, res)
	})

	t.Run("Backend error", func(t *testing.T) {
		res, err := disp.OnSubmit("mychannel", 1, expectedRequestForBackendError)
		assert.NoError(t, err)
		assert.Equal(t, expectedWrongBackendError, res)
	})
}
