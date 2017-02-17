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
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	configtxchannel "github.com/hyperledger/fabric/common/configvalues/channel"
	configtxapplication "github.com/hyperledger/fabric/common/configvalues/channel/application"
	configtxorderer "github.com/hyperledger/fabric/common/configvalues/channel/orderer"
	configvaluesmsp "github.com/hyperledger/fabric/common/configvalues/msp"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Generator can either create an orderer genesis block or config template
type Generator interface {
	bootstrap.Helper

	// ChannelTemplate returns a template which can be used to help initialize a channel
	ChannelTemplate() configtx.Template
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
	minimalGroups     []*cb.ConfigGroup
	systemChainGroups []*cb.ConfigGroup
}

// New returns a new provisional bootstrap helper.
func New(conf *genesisconfig.TopLevel) Generator {
	bs := &bootstrapper{
		minimalGroups: []*cb.ConfigGroup{
			// Chain Config Types
			configtxchannel.DefaultHashingAlgorithm(),
			configtxchannel.DefaultBlockDataHashingStructure(),
			configtxchannel.TemplateOrdererAddresses(conf.Orderer.Addresses),

			// Orderer Config Types
			configtxorderer.TemplateConsensusType(conf.Orderer.OrdererType),
			configtxorderer.TemplateBatchSize(&ab.BatchSize{
				MaxMessageCount:   conf.Orderer.BatchSize.MaxMessageCount,
				AbsoluteMaxBytes:  conf.Orderer.BatchSize.AbsoluteMaxBytes,
				PreferredMaxBytes: conf.Orderer.BatchSize.PreferredMaxBytes,
			}),
			configtxorderer.TemplateBatchTimeout(conf.Orderer.BatchTimeout.String()),
			configtxorderer.TemplateIngressPolicyNames([]string{AcceptAllPolicyKey}),
			configtxorderer.TemplateEgressPolicyNames([]string{AcceptAllPolicyKey}),

			// Policies
			cauthdsl.TemplatePolicy(AcceptAllPolicyKey, cauthdsl.AcceptAllPolicy),

			// Initialize the default Reader/Writer/Admins channel policies
			policies.TemplateImplicitMetaAnyPolicy([]string{}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{}, configvaluesmsp.AdminsPolicyKey),

			// Initialize the default Reader/Writer/Admins orderer policies
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxorderer.GroupKey}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxorderer.GroupKey}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{configtxorderer.GroupKey}, configvaluesmsp.AdminsPolicyKey),

			// Initialize the default Reader/Writer/Admins application policies
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxapplication.GroupKey}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxapplication.GroupKey}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{configtxapplication.GroupKey}, configvaluesmsp.AdminsPolicyKey),
		},

		systemChainGroups: []*cb.ConfigGroup{
			configtxorderer.TemplateChainCreationPolicyNames(DefaultChainCreationPolicyNames),
		},
	}

	switch conf.Orderer.OrdererType {
	case ConsensusTypeSolo, ConsensusTypeSbft:
	case ConsensusTypeKafka:
		bs.minimalGroups = append(bs.minimalGroups, configtxorderer.TemplateKafkaBrokers(conf.Orderer.Kafka.Brokers))
	default:
		panic(fmt.Errorf("Wrong consenter type value given: %s", conf.Orderer.OrdererType))
	}

	return bs
}

func (bs *bootstrapper) ChannelTemplate() configtx.Template {
	return configtx.NewSimpleTemplate(bs.minimalGroups...)
}

func (bs *bootstrapper) GenesisBlock() *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewCompositeTemplate(
			configtx.NewSimpleTemplate(bs.systemChainGroups...),
			bs.ChannelTemplate(),
		),
	).Block(TestChainID)

	if err != nil {
		panic(err)
	}
	return block
}
