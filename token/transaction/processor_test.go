/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package transaction_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/transaction"
	"github.com/hyperledger/fabric/token/transaction/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestProcessor_GenerateSimulationResults_FailedUnmarshalTokenTransaction(t *testing.T) {
	txp := transaction.Processor{}
	tx := invalidTransaction()

	err := txp.GenerateSimulationResults(tx, nil, false)
	assert.EqualError(t, err, "failed unmarshalling token transaction: error unmarshaling Payload: proto: can't skip unknown wire type 7")
}

func TestProcessor_GenerateSimulationResults_FailedGetCommitter(t *testing.T) {
	tmsManager := &mocks.TMSManager{}
	tmsManager.On("GetTxProcessor", "wild_channel").Return(
		nil, errors.New("wild_GetCommitter_error"),
	)

	txp := transaction.Processor{TMSManager: tmsManager}
	tx := validTransaction(t)

	tmsManager.On("GetTxProcessor", "wild_channel").Return(nil, errors.New("wild_GetCommitter_error"))

	err := txp.GenerateSimulationResults(tx, nil, false)
	assert.EqualError(t, err, "failed getting committer for channel wild_channel: wild_GetCommitter_error")
	tmsManager.AssertCalled(t, "GetTxProcessor", "wild_channel")
}

func TestProcessor_GenerateSimulationResults_FailedCommit(t *testing.T) {
	committer := &mocks.TMSTxProcessor{}
	committer.On("ProcessTx", mock.Anything, nil, false).Return(errors.New("wild_Commit_error"))

	tmsManager := &mocks.TMSManager{}
	tmsManager.On("GetTxProcessor", "wild_channel").Return(committer, nil)

	txp := transaction.Processor{TMSManager: tmsManager}
	tx := validTransaction(t)

	err := txp.GenerateSimulationResults(tx, nil, false)
	assert.EqualError(t, err, "failed committing transaction for channel wild_channel: wild_Commit_error")
	tmsManager.AssertCalled(t, "GetTxProcessor", "wild_channel")
	committer.AssertCalled(t, "ProcessTx", mock.Anything, nil, false)
}

func TestProcessor_GenerateSimulationResults_Success(t *testing.T) {
	committer := &mocks.TMSTxProcessor{}
	committer.On("ProcessTx", mock.Anything, nil, false).Return(nil)

	tmsManager := &mocks.TMSManager{}
	tmsManager.On("GetTxProcessor", "wild_channel").Return(committer, nil)

	txp := transaction.Processor{TMSManager: tmsManager}
	tx := validTransaction(t)

	err := txp.GenerateSimulationResults(tx, nil, false)
	assert.NoError(t, err)
	tmsManager.AssertCalled(t, "GetTxProcessor", "wild_channel")
	committer.AssertCalled(t, "ProcessTx", mock.Anything, nil, false)
}

func invalidTransaction() *common.Envelope {
	return &common.Envelope{
		Payload:   []byte("wild_payload"),
		Signature: nil,
	}
}

func validTransaction(t *testing.T) *common.Envelope {
	ttx := &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{},
		},
	}

	ch := &common.ChannelHeader{
		Type: int32(common.HeaderType_TOKEN_TRANSACTION), ChannelId: "wild_channel",
	}
	hdr := &common.Header{
		ChannelHeader: marshal(t, ch),
	}
	payload := &common.Payload{
		Header: hdr,
		Data:   marshal(t, ttx),
	}
	payloadBytes, err := proto.Marshal(payload)
	assert.NoError(t, err)
	return &common.Envelope{
		Payload:   payloadBytes,
		Signature: nil,
	}
}

func marshal(t *testing.T, pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	assert.NoError(t, err)
	return data
}
