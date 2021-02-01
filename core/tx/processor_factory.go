/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tx

import (
	"github.com/pkg/errors"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/pkg/tx"
	"github.com/hyperledger/fabric/protoutil"
)

// ProcessorFactory maintains a mapping between transaction type and associate `ProcessorCreator`
type ProcessorFactory struct {
	ProcessorCreators map[common.HeaderType]tx.ProcessorCreator
}

// CreateProcessor unmarshals bytes into an Envelope and invokes a ProcessorCreators corresponding to the transaction type
// present in the ChannelHeader. If successful, this function returns the Processor and simulatedRWSet created by the ProcessorCreators.
// However, if this function encounters and error in detecting the transaction type or a
// ProcessorCreators has not been registered for the transaction type, the error returned would be of type
// `tx.InvalidErr` which implies that the transaction is found to be invalid at the very beginning stage
func (f *ProcessorFactory) CreateProcessor(txEnvelopeBytes []byte) (processor tx.Processor, simulatedRWSet [][]byte, err error) {
	txEnv, err := validateProtoAndConstructTxEnv(txEnvelopeBytes)
	if err != nil {
		return nil, nil, err
	}
	c, ok := f.ProcessorCreators[common.HeaderType(txEnv.ChannelHeader.Type)]
	if !ok {
		return nil, nil, &tx.InvalidErr{
			ActualErr:      errors.Errorf("invalid transaction type %d", txEnv.ChannelHeader.Type),
			ValidationCode: peer.TxValidationCode_UNKNOWN_TX_TYPE,
		}
	}
	return c.NewProcessor(txEnv)
}

// validateProtoAndConstructTxEnv attempts to unmarshal the bytes and prepare an instance of struct tx.Envelope
// It returns an error of type `tx.InvalidErr` if the proto message is found to be invalid
func validateProtoAndConstructTxEnv(txEnvelopeBytes []byte) (*tx.Envelope, error) {
	txenv, err := protoutil.UnmarshalEnvelope(txEnvelopeBytes)
	if err != nil {
		return nil, &tx.InvalidErr{
			ActualErr:      err,
			ValidationCode: peer.TxValidationCode_INVALID_OTHER_REASON,
		}
	}

	if len(txenv.Payload) == 0 {
		return nil, &tx.InvalidErr{
			ActualErr:      errors.New("nil envelope payload"),
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	payload, err := protoutil.UnmarshalPayload(txenv.Payload)
	if err != nil {
		return nil, &tx.InvalidErr{
			ActualErr:      err,
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	if payload.Header == nil {
		return nil, &tx.InvalidErr{
			ActualErr:      errors.New("nil payload header"),
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	if len(payload.Header.ChannelHeader) == 0 {
		return nil, &tx.InvalidErr{
			ActualErr:      errors.New("nil payload channel header"),
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, &tx.InvalidErr{
			ActualErr:      err,
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	if len(payload.Header.SignatureHeader) == 0 {
		return nil, &tx.InvalidErr{
			ActualErr:      errors.New("nil payload signature header"),
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(payload.Header.SignatureHeader)
	if err != nil {
		return nil, &tx.InvalidErr{
			ActualErr:      err,
			ValidationCode: peer.TxValidationCode_BAD_PAYLOAD,
		}
	}

	// other checks over shdr.Nonce, shdr.Creator can be added if universally applicable

	// what TODO in legacy validation:
	//   validate cHdr.ChannelId ?
	//   validate epoch in cHdr.Epoch?

	return &tx.Envelope{
			SignedBytes:          txenv.Payload,
			Signature:            txenv.Signature,
			Data:                 payload.Data,
			ChannelHeaderBytes:   payload.Header.ChannelHeader,
			SignatureHeaderBytes: payload.Header.SignatureHeader,
			ChannelHeader:        chdr,
			SignatureHeader:      shdr,
		},
		nil
}
