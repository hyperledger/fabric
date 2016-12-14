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

package provisional

import (
	"fmt"

	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

const (
	msgVersion = int32(1)

	// ConsensusTypeSolo identifies the solo consensus implementation.
	ConsensusTypeSolo = "solo"
	// ConsensusTypeKafka identifies the Kafka-based consensus implementation.
	ConsensusTypeKafka = "kafka"
	// ConsensusTypeSbft identifies the SBFT consensus implementation.
	ConsensusTypeSbft = "sbft"

	// TestChainID is the default value of ChainID. It is used by all testing
	// networks. It it necessary to set and export this variable so that test
	// clients can connect without being rejected for targetting a chain which
	// does not exist.
	TestChainID = "**TEST_CHAINID**"

	// AcceptAllPolicyKey is the key of the AcceptAllPolicy.
	AcceptAllPolicyKey = "AcceptAllPolicy"

	// These values are fixed for the genesis block.
	lastModified = 0
	epoch        = 0
)

// DefaultChainCreators is the default value of ChainCreatorsKey.
var DefaultChainCreators = []string{AcceptAllPolicyKey}

type commonBootstrapper struct {
	chainID       string
	consensusType string
	batchSize     *ab.BatchSize
}

type soloBootstrapper struct {
	commonBootstrapper
}

type kafkaBootstrapper struct {
	commonBootstrapper
	kafkaBrokers []string
}

// New returns a new provisional bootstrap helper.
func New(conf *config.TopLevel) bootstrap.Helper {
	cbs := &commonBootstrapper{
		chainID:       TestChainID,
		consensusType: conf.General.OrdererType,
		batchSize: &ab.BatchSize{
			MaxMessageCount: conf.General.BatchSize.MaxMessageCount,
		},
	}

	switch conf.General.OrdererType {
	case ConsensusTypeSolo, ConsensusTypeSbft:
		return &soloBootstrapper{
			commonBootstrapper: *cbs,
		}
	case ConsensusTypeKafka:
		return &kafkaBootstrapper{
			commonBootstrapper: *cbs,
			kafkaBrokers:       conf.Kafka.Brokers,
		}
	default:
		panic(fmt.Errorf("Wrong consenter type value given: %s", conf.General.OrdererType))
	}
}

// GenesisBlock returns the genesis block to be used for bootstrapping.
func (cbs *commonBootstrapper) GenesisBlock() *cb.Block {
	return cbs.makeGenesisBlock(cbs.makeGenesisConfigEnvelope())
}

// GenesisBlock returns the genesis block to be used for bootstrapping.
func (kbs *kafkaBootstrapper) GenesisBlock() *cb.Block {
	return kbs.makeGenesisBlock(kbs.makeGenesisConfigEnvelope())
}

func (cbs *commonBootstrapper) makeGenesisBlock(configEnvelope *cb.ConfigurationEnvelope) *cb.Block {
	configItemChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM, msgVersion, cbs.chainID, epoch)
	payloadChainHeader := utils.MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION, configItemChainHeader.Version, cbs.chainID, epoch)
	payloadSignatureHeader := utils.MakeSignatureHeader(nil, utils.CreateNonceOrPanic())
	payloadHeader := utils.MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(configEnvelope)}
	envelope := &cb.Envelope{Payload: utils.MarshalOrPanic(payload), Signature: nil}

	blockData := &cb.BlockData{Data: [][]byte{utils.MarshalOrPanic(envelope)}}

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number:       0,
			PreviousHash: nil,
			DataHash:     blockData.Hash(),
		},
		Data:     blockData,
		Metadata: nil,
	}
}
