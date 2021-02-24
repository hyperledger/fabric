/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"errors"

	"github.com/golang/protobuf/proto"
	commonProto "github.com/hyperledger/fabric-protos-go/common"
	peerProto "github.com/hyperledger/fabric-protos-go/peer"
	ghErrors "github.com/pkg/errors"
)

type blockParser struct {
	Block *commonProto.Block
}

func (parser *blockParser) TransactionValidationCodes() (map[string]peerProto.TxValidationCode, error) {
	results := make(map[string]peerProto.TxValidationCode)

	envelopes := parser.Block.Data.Data
	for i, envelopeBytes := range envelopes {
		channelHeader, err := unmarshalChannelHeader(envelopeBytes)
		if err != nil {
			return nil, ghErrors.WithMessagef(err, "failed to unmarshal channel header from envelope at index %v in block number %v",
				i, parser.Block.Header.Number)
		}

		validationCode := parser.validationCode(i)
		if validationCode == peerProto.TxValidationCode_BAD_PROPOSAL_TXID {
			continue
		}

		if _, exists := results[channelHeader.TxId]; !exists {
			results[channelHeader.TxId] = parser.validationCode(i)
		}
	}

	return results, nil
}

func (parser *blockParser) validationCode(envelopeIndex int) peerProto.TxValidationCode {
	validationCodes := parser.Block.Metadata.Metadata[int(commonProto.BlockMetadataIndex_TRANSACTIONS_FILTER)]
	return peerProto.TxValidationCode(validationCodes[envelopeIndex])
}

func unmarshalChannelHeader(envelopeBytes []byte) (*commonProto.ChannelHeader, error) {
	envelope := &commonProto.Envelope{}
	if err := proto.Unmarshal(envelopeBytes, envelope); err != nil {
		return nil, err
	}

	payload := &commonProto.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return nil, err
	}

	if payload.Header == nil {
		return nil, errors.New("missing payload header")
	}

	channelHeader := &commonProto.ChannelHeader{}
	if err := proto.Unmarshal(payload.Header.ChannelHeader, channelHeader); err != nil {
		return nil, err
	}

	return channelHeader, nil
}
