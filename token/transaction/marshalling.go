/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/pkg/errors"
)

func UnmarshalTokenTransaction(raw []byte) (*cb.ChannelHeader, *token.TokenTransaction, identity.PublicInfo, error) {
	// the payload...
	payload := &common.Payload{}
	err := proto.Unmarshal(raw, payload)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "error unmarshaling Payload")
	}

	// the creator from the signature header
	sh, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	if err != nil {
		return nil, nil, nil, err
	}
	creatorInfo := &TxCreatorInfo{public: sh.Creator}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, nil, nil, err
	}

	// validate the payload type
	if common.HeaderType(chdr.Type) != common.HeaderType_TOKEN_TRANSACTION {
		return nil, nil, nil, errors.Errorf("only token transactions are supported, provided type: %d", chdr.Type)
	}

	ttx := &token.TokenTransaction{}
	err = proto.Unmarshal(payload.Data, ttx)
	if err != nil {
		return nil, nil, nil, errors.Errorf("failed getting token token transaction, %s", err)
	}

	return chdr, ttx, creatorInfo, nil
}
