// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
)

func newChainRequest(
	consensusType,
	creationPolicy,
	newChannelID string,
	signer identity.SignerSerializer,
) *cb.Envelope {
	env, err := encoder.MakeChannelCreationTransaction(
		newChannelID,
		signer,
		genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile),
	)
	if err != nil {
		panic(err)
	}
	return env
}
