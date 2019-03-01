/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"testing"

	"github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
)

func TestValidateConfigTx(t *testing.T) {
	chainID := util.GetTestChainID()
	profile := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
	chCrtEnv, err := encoder.MakeChannelCreationTransaction(genesisconfig.SampleConsortiumName, nil, profile)
	if err != nil {
		t.Fatalf("MakeChannelCreationTransaction failed, err %s", err)
		return
	}

	updateResult := &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{Header: &cb.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
				Type:      int32(cb.HeaderType_CONFIG),
				ChannelId: chainID,
			}),
			SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{
				Creator: signerSerialized,
				Nonce:   protoutil.CreateNonceOrPanic(),
			}),
		},
			Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
				LastUpdate: chCrtEnv,
			}),
		}),
	}
	updateResult.Signature, _ = signer.Sign(updateResult.Payload)
	_, txResult := ValidateTransaction(updateResult, &config.MockApplicationCapabilities{})
	if txResult != peer.TxValidationCode_VALID {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}
}
