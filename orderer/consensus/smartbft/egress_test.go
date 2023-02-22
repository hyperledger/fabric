/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"testing"

	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestEgressSendConsensus(t *testing.T) {
	logger := flogging.MustGetLogger("test")
	rpc := &mocks.RPC{}
	rpc.On("SendConsensus", mock.Anything, mock.Anything).Return(nil)
	egress := &smartbft.Egress{
		Logger:  logger,
		Channel: "test",
		RPC:     rpc,
	}

	viewData := &protos.Message{
		Content: &protos.Message_NewView{
			NewView: &protos.NewView{SignedViewData: []*protos.SignedViewData{
				{RawViewData: []byte{1, 2, 3}},
			}},
		},
	}

	egress.SendConsensus(42, viewData)

	rpc.AssertCalled(t, "SendConsensus", uint64(42), &ab.ConsensusRequest{
		Payload: protoutil.MarshalOrPanic(viewData),
		Channel: "test",
	})
}

func TestEgressSendTransaction(t *testing.T) {
	logger := flogging.MustGetLogger("test")
	rpc := &mocks.RPC{}
	rpc.On("SendSubmit", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	egress := &smartbft.Egress{
		Logger:  logger,
		Channel: "test",
		RPC:     rpc,
	}

	t.Run("malformed transaction", func(t *testing.T) {
		badTransactionAttempt := func() {
			egress.SendTransaction(42, []byte{1, 2, 3})
		}
		assert.Panics(t, badTransactionAttempt)
	})

	t.Run("valid transaction", func(t *testing.T) {
		egress.SendTransaction(42, protoutil.MarshalOrPanic(&cb.Envelope{
			Payload: []byte{1, 2, 3},
		}))
	})

	rpc.AssertCalled(t, "SendSubmit", uint64(42), &ab.SubmitRequest{
		Channel: "test",
		Payload: &cb.Envelope{
			Payload: []byte{1, 2, 3},
		},
	}, mock.Anything)
}
