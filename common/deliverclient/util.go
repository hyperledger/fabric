/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var ErrNotAConfig = errors.New("not a config block")

// ConfigFromBlock returns a ConfigEnvelope if exists, or a *ErrNotAConfig error.
// It may also return some other error in case parsing failed.
func ConfigFromBlock(block *common.Block) (*common.ConfigEnvelope, error) {
	if block == nil || block.GetData() == nil || len(block.GetData().GetData()) == 0 {
		return nil, errors.New("empty block")
	}
	txn := block.GetData().GetData()[0]
	env, err := protoutil.GetEnvelopeFromBlock(txn)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	payload, err := protoutil.UnmarshalPayload(env.GetPayload())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if block.GetHeader().GetNumber() == 0 {
		configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.GetData())
		if err != nil {
			return nil, errors.Wrap(err, "invalid config envelope")
		}
		return configEnvelope, nil
	}
	if payload.GetHeader() == nil {
		return nil, errors.New("nil header in payload")
	}
	chdr, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if common.HeaderType(chdr.GetType()) != common.HeaderType_CONFIG {
		return nil, ErrNotAConfig
	}
	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.GetData())
	if err != nil {
		return nil, errors.Wrap(err, "invalid config envelope")
	}
	return configEnvelope, nil
}
