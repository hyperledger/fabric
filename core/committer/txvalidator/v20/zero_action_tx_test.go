/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package txvalidator_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	txvalidatorv20 "github.com/hyperledger/fabric/core/committer/txvalidator/v20"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

// Blocks admitted by the orderer may contain transactions that are
// structurally invalid (no actions, or actions without a chaincode action).
// Validating such blocks must mark those txs invalid without panicking,
// including when a later valid tx's dependency scan walks over them.
func signedEnvWithTx(t *testing.T, tx *peer.Transaction, badTxID bool) *common.Envelope {
	shdr := protoutil.NewSignatureHeaderOrPanic(signer)
	chdr := protoutil.MakeChannelHeader(common.HeaderType_ENDORSER_TRANSACTION, 0, "testchannelid", 0)
	if badTxID {
		chdr.TxId = "bogus-txid"
	} else {
		chdr.TxId = protoutil.ComputeTxID(shdr.GetNonce(), shdr.GetCreator())
	}
	payl := &common.Payload{
		Header: &common.Header{
			ChannelHeader:   protoutil.MarshalOrPanic(chdr),
			SignatureHeader: protoutil.MarshalOrPanic(shdr),
		},
		Data: protoutil.MarshalOrPanic(tx),
	}
	paylBytes := protoutil.MarshalOrPanic(payl)
	sig, err := signer.Sign(paylBytes)
	require.NoError(t, err)
	return &common.Envelope{Payload: paylBytes, Signature: sig}
}

func setupCCValidator(t *testing.T) (*txvalidatorv20.TxValidator, []byte) {
	v, mockQE, _, _ := setupValidator()
	mockQE.On("GetState", "lscc", "mycc").Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    "mycc",
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", "mycc", "key").Return(nil, nil)
	return v, createRWset(t, "mycc")
}

func TestValidateBlockWithActionlessTx(t *testing.T) {
	v, rwsetBytes := setupCCValidator(t)

	actionless := signedEnvWithTx(t, &peer.Transaction{Actions: nil}, false)
	valid := getEnv("mycc", nil, rwsetBytes, t)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(actionless), protoutil.MarshalOrPanic(valid)}},
		Header: &common.BlockHeader{Number: 2},
	}

	require.NoError(t, v.Validate(b))
}

func TestValidateBlockWithNilChaincodeActionTx(t *testing.T) {
	v, rwsetBytes := setupCCValidator(t)

	capPayload := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: []byte("proposal"), Action: nil}
	act := &peer.TransactionAction{
		Header:  protoutil.MarshalOrPanic(protoutil.NewSignatureHeaderOrPanic(signer)),
		Payload: protoutil.MarshalOrPanic(capPayload),
	}
	// bad TxID: rejected before structural validation, so the dependency scan
	// of the following valid tx walks over it
	poison := signedEnvWithTx(t, &peer.Transaction{Actions: []*peer.TransactionAction{act}}, true)
	valid := getEnv("mycc", nil, rwsetBytes, t)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(poison), protoutil.MarshalOrPanic(valid)}},
		Header: &common.BlockHeader{Number: 2},
	}

	require.NoError(t, v.Validate(b))
}
