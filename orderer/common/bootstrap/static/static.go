/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package static

import (
	"math/rand"

	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/cauthdsl"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type bootstrapper struct {
	chainID []byte
}

// New returns a new static bootstrap helper
func New() bootstrap.Helper {
	b := make([]byte, 16)
	rand.Read(b)

	return &bootstrapper{
		chainID: b,
	}
}

// errorlessMarshal prevents poluting this code with many panics, if the genesis block cannot be created, the system cannot start so panic is correct
func errorlessMarshal(thing proto.Message) []byte {
	data, err := proto.Marshal(thing)
	if err != nil {
		panic(err)
	}
	return data
}

func (b *bootstrapper) makeSignedConfigurationItem(id string, ctype cb.ConfigurationItem_ConfigurationType, data []byte, modificationPolicyID string) *cb.SignedConfigurationItem {
	configurationBytes := errorlessMarshal(&cb.ConfigurationItem{
		Header: &cb.ChainHeader{
			ChainID: b.chainID,
		},
		LastModified:       0,
		Type:               ctype,
		ModificationPolicy: modificationPolicyID,
		Key:                id,
		Value:              data,
	})
	return &cb.SignedConfigurationItem{
		ConfigurationItem: configurationBytes,
	}
}

func sigPolicyToPolicy(sigPolicy *cb.SignaturePolicyEnvelope) []byte {
	policy := &cb.Policy{
		Type: &cb.Policy_SignaturePolicy{
			SignaturePolicy: sigPolicy,
		},
	}
	return errorlessMarshal(policy)
}

// GenesisBlock returns the genesis block to be used for bootstrapping
func (b *bootstrapper) GenesisBlock() (*cb.Block, error) {

	// Lock down the default modification policy to prevent any further policy modifications
	lockdownDefaultModificationPolicy := b.makeSignedConfigurationItem(configtx.DefaultModificationPolicyID, cb.ConfigurationItem_Policy, sigPolicyToPolicy(cauthdsl.RejectAllPolicy), configtx.DefaultModificationPolicyID)

	initialConfigTX := errorlessMarshal(&cb.ConfigurationEnvelope{
		Items: []*cb.SignedConfigurationItem{
			lockdownDefaultModificationPolicy,
		},
	})

	data := &cb.BlockData{
		Data: [][]byte{initialConfigTX},
	}

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number:       0,
			PreviousHash: []byte("GENESIS"),
			DataHash:     data.Hash(),
		},
		Data: data,
	}, nil

}
