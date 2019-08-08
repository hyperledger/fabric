/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package tx

import (
	"github.com/hyperledger/fabric/pkg/tx"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
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
	c, ok := f.ProcessorCreators[common.HeaderType(txEnv.Payload.Header.ChannelHeader.Type)]
	if !ok {
		return nil, nil, &tx.InvalidErr{
			ValidationCode: peer.TxValidationCode_UNKNOWN_TX_TYPE,
		}
	}
	return c.NewProcessor(txEnv)
}

// validateProtoAndConstructTxEnv attemps to unmarshal the bytes and prepare an instance of struct tx.Envelope
// It retruns an error of type `tx.InvalidErr` if the proto message is found to be invalid
func validateProtoAndConstructTxEnv(txEnvelopeBytes []byte) (*tx.Envelope, error) {
	// TODO implement
	return &tx.Envelope{}, nil
}
