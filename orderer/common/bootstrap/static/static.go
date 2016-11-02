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
	"fmt"
	"time"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/cauthdsl"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
)

type bootstrapper struct {
	chainID []byte
}

// New returns a new static bootstrap helper.
func New() bootstrap.Helper {
	chainID, err := primitives.GetRandomBytes(16)
	if err != nil {
		panic(fmt.Errorf("Cannot generate random chain ID: %s", err))
	}
	return &bootstrapper{chainID}
}

// errorlessMarshal prevents poluting this code with panics
// If the genesis block cannot be created, the system cannot start so panic is correct
func errorlessMarshal(thing proto.Message) []byte {
	data, err := proto.Marshal(thing)
	if err != nil {
		panic(err)
	}
	return data
}

func makeChainHeader(headerType cb.HeaderType, version int32, chainID []byte, epoch uint64) *cb.ChainHeader {
	return &cb.ChainHeader{
		Type:    int32(headerType),
		Version: version,
		Timestamp: &timestamp.Timestamp{
			Seconds: time.Now().Unix(),
			Nanos:   0,
		},
		ChainID: chainID,
		Epoch:   epoch,
	}
}

func makeSignatureHeader(serializedCreatorCertChain []byte, nonce []byte) *cb.SignatureHeader {
	return &cb.SignatureHeader{
		Creator: serializedCreatorCertChain,
		Nonce:   nonce,
	}
}

func (b *bootstrapper) makeSignedConfigurationItem(configurationItemType cb.ConfigurationItem_ConfigurationType, modificationPolicyID string, key string, value []byte) *cb.SignedConfigurationItem {
	marshaledConfigurationItem := errorlessMarshal(&cb.ConfigurationItem{
		Header:             makeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, 1, b.chainID, 0),
		Type:               configurationItemType,
		LastModified:       0,
		ModificationPolicy: modificationPolicyID,
		Key:                key,
		Value:              value,
	})

	return &cb.SignedConfigurationItem{
		ConfigurationItem: marshaledConfigurationItem,
		Signatures:        nil,
	}
}

func (b *bootstrapper) makeConfigurationEnvelope(items ...*cb.SignedConfigurationItem) *cb.ConfigurationEnvelope {
	return &cb.ConfigurationEnvelope{
		Items: items,
	}
}

func (b *bootstrapper) makeEnvelope(configurationEnvelope *cb.ConfigurationEnvelope) *cb.Envelope {
	nonce, err := primitives.GetRandomNonce()
	if err != nil {
		panic(fmt.Errorf("Cannot generate random nonce: %s", err))
	}
	marshaledPayload := errorlessMarshal(&cb.Payload{
		Header: &cb.Header{
			ChainHeader:     makeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, 1, b.chainID, 0),
			SignatureHeader: makeSignatureHeader(nil, nonce),
		},
		Data: errorlessMarshal(configurationEnvelope),
	})
	return &cb.Envelope{
		Payload:   marshaledPayload,
		Signature: nil,
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
	lockdownDefaultModificationPolicy := b.makeSignedConfigurationItem(cb.ConfigurationItem_Policy, configtx.DefaultModificationPolicyID, configtx.DefaultModificationPolicyID, sigPolicyToPolicy(cauthdsl.RejectAllPolicy))

	blockData := &cb.BlockData{
		Data: [][]byte{errorlessMarshal(b.makeEnvelope(b.makeConfigurationEnvelope(lockdownDefaultModificationPolicy)))},
	}

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number:       0,
			PreviousHash: nil,
			DataHash:     blockData.Hash(),
		},
		Data:     blockData,
		Metadata: nil,
	}, nil
}
