/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"testing"

	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDispatchConsensus(t *testing.T) {
	expectedRequest := &protos.Message{
		Content: &protos.Message_Prepare{
			Prepare: &protos.Prepare{
				Seq:  1,
				View: 1,
			},
		},
	}

	t.Run("Channel exists", func(t *testing.T) {
		receivedMessage := make(chan *protos.Message, 1)
		mr := &mocks.MessageReceiver{}
		mr.On("HandleMessage", uint64(1), mock.Anything).Return(nil).Run(func(arguments mock.Arguments) {
			receivedMessage <- arguments.Get(1).(*protos.Message)
		}).Once()

		rg := &mocks.ReceiverGetter{}
		rg.On("ReceiverByChain", "mychannel").Return(mr).Twice()

		ingress := &smartbft.Ingress{ChainSelector: rg, Logger: flogging.MustGetLogger("test")}

		// Good message
		err := ingress.OnConsensus("mychannel", 1, &ab.ConsensusRequest{
			Channel: "mychannel",
			Payload: protoutil.MarshalOrPanic(expectedRequest),
		})
		assert.NoError(t, err)

		receivedMsg := <-receivedMessage
		assert.True(t, proto.Equal(receivedMsg, expectedRequest))
		mr.AssertNumberOfCalls(t, "HandleMessage", 1)

		// Bad message
		err = ingress.OnConsensus("mychannel", 1, &ab.ConsensusRequest{
			Channel: "mychannel",
			Payload: []byte{1, 2, 3},
		})

		assert.Contains(t, err.Error(), "malformed message")
	})

	t.Run("Channel does not exist", func(t *testing.T) {
		rg := &mocks.ReceiverGetter{}
		rg.On("ReceiverByChain", "notmychannel").Return(nil).Once()

		ingress := &smartbft.Ingress{ChainSelector: rg, Logger: flogging.MustGetLogger("test")}

		err := ingress.OnConsensus("notmychannel", 1, nil)
		assert.EqualError(t, err, "channel notmychannel doesn't exist")
	})
}

func TestDispatchSubmit(t *testing.T) {
	expectedRequest := &ab.SubmitRequest{
		Channel: "ignored value - success",
		Payload: &cb.Envelope{Payload: []byte{1, 2, 3}},
	}

	mr := &mocks.MessageReceiver{}
	mr.On("Submit", expectedRequest, uint64(1)).Return(nil).Once()

	rg := &mocks.ReceiverGetter{}
	rg.On("ReceiverByChain", "mychannel").Return(mr).Once()
	rg.On("ReceiverByChain", "notmychannel").Return(nil).Once()

	t.Run("Channel exists", func(t *testing.T) {
		mr := &mocks.MessageReceiver{}
		mr.On("HandleRequest", uint64(1), protoutil.MarshalOrPanic(expectedRequest.Payload)).Return(nil).Once()

		rg := &mocks.ReceiverGetter{}
		rg.On("ReceiverByChain", "mychannel").Return(mr).Once()

		ingress := &smartbft.Ingress{ChainSelector: rg, Logger: flogging.MustGetLogger("test")}

		err := ingress.OnSubmit("mychannel", 1, expectedRequest)
		assert.NoError(t, err)

		mr.AssertCalled(t, "HandleRequest", uint64(1), protoutil.MarshalOrPanic(expectedRequest.Payload))
	})

	t.Run("Channel does not exist", func(t *testing.T) {
		rg := &mocks.ReceiverGetter{}
		rg.On("ReceiverByChain", "notmychannel").Return(nil).Once()

		ingress := &smartbft.Ingress{ChainSelector: rg, Logger: flogging.MustGetLogger("test")}

		err := ingress.OnSubmit("notmychannel", 1, expectedRequest)
		assert.EqualError(t, err, "channel notmychannel doesn't exist")
	})
}
