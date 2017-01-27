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

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Generator can either create an orderer genesis block or configuration template
type Generator interface {
	bootstrap.Helper

	// TemplateItems returns a set of configuration items which can be used to initialize a template
	TemplateItems() []*cb.ConfigurationItem
}

const (
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
	TestChainID = "testchainid"

	// AcceptAllPolicyKey is the key of the AcceptAllPolicy.
	AcceptAllPolicyKey = "AcceptAllPolicy"
)

// DefaultChainCreationPolicyNames is the default value of ChainCreatorsKey.
var DefaultChainCreationPolicyNames = []string{AcceptAllPolicyKey}

type bootstrapper struct {
	minimalItems     []*cb.ConfigurationItem
	systemChainItems []*cb.ConfigurationItem
}

// New returns a new provisional bootstrap helper.
func New(conf *config.TopLevel) Generator {
	bs := &bootstrapper{
		minimalItems: []*cb.ConfigurationItem{
			// Chain Config Types
			chainconfig.DefaultHashingAlgorithm(),
			chainconfig.DefaultBlockDataHashingStructure(),
			chainconfig.TemplateOrdererAddresses([]string{fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort)}),

			// Orderer Config Types
			sharedconfig.TemplateConsensusType(conf.Genesis.OrdererType),
			sharedconfig.TemplateBatchSize(&ab.BatchSize{
				MaxMessageCount:   conf.Genesis.BatchSize.MaxMessageCount,
				AbsoluteMaxBytes:  conf.Genesis.BatchSize.AbsoluteMaxBytes,
				PreferredMaxBytes: conf.Genesis.BatchSize.PreferredMaxBytes,
			}),
			sharedconfig.TemplateBatchTimeout(conf.Genesis.BatchTimeout.String()),
			sharedconfig.TemplateIngressPolicyNames([]string{AcceptAllPolicyKey}),
			sharedconfig.TemplateEgressPolicyNames([]string{AcceptAllPolicyKey}),

			// Policies
			cauthdsl.TemplatePolicy(configtx.NewConfigurationItemPolicyKey, cauthdsl.RejectAllPolicy),
			cauthdsl.TemplatePolicy(AcceptAllPolicyKey, cauthdsl.AcceptAllPolicy),
		},

		systemChainItems: []*cb.ConfigurationItem{
			sharedconfig.TemplateChainCreationPolicyNames(DefaultChainCreationPolicyNames),
		},
	}

	switch conf.Genesis.OrdererType {
	case ConsensusTypeSolo, ConsensusTypeSbft:
	case ConsensusTypeKafka:
		bs.minimalItems = append(bs.minimalItems, sharedconfig.TemplateKafkaBrokers(conf.Kafka.Brokers))
	default:
		panic(fmt.Errorf("Wrong consenter type value given: %s", conf.Genesis.OrdererType))
	}

	return bs
}

func (bs *bootstrapper) TemplateItems() []*cb.ConfigurationItem {
	return bs.minimalItems
}

func (bs *bootstrapper) GenesisBlock() *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewCompositeTemplate(
			configtx.NewSimpleTemplate(bs.minimalItems...),
			configtx.NewSimpleTemplate(bs.systemChainItems...),
		),
	).Block(TestChainID)

	if err != nil {
		panic(err)
	}
	return block
}
