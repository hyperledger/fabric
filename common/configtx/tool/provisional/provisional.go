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
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/configtx/tool/provisional")

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
	channelGroups              []*cb.ConfigGroup
	ordererGroups              []*cb.ConfigGroup
	applicationGroups          []*cb.ConfigGroup
	ordererSystemChannelGroups []*cb.ConfigGroup
}

// New returns a new provisional bootstrap helper.
func New(conf *genesisconfig.Profile) Generator {
	bs := &bootstrapper{
		channelGroups: []*cb.ConfigGroup{
			// Chain Config Types
			configtxchannel.DefaultHashingAlgorithm(),
			configtxchannel.DefaultBlockDataHashingStructure(),
			configtxchannel.TemplateOrdererAddresses(conf.Orderer.Addresses), // TODO, move to conf.Channel when it exists

			// Default policies
			policies.TemplateImplicitMetaAnyPolicy([]string{}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{}, configvaluesmsp.AdminsPolicyKey),

			// Temporary AcceptAllPolicy XXX, remove
			cauthdsl.TemplatePolicy(AcceptAllPolicyKey, cauthdsl.AcceptAllPolicy),
		},
	}

	if conf.Orderer != nil {
		bs.ordererGroups = []*cb.ConfigGroup{
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

			// Initialize the default Reader/Writer/Admins orderer policies
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxorderer.GroupKey}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxorderer.GroupKey}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{configtxorderer.GroupKey}, configvaluesmsp.AdminsPolicyKey),
		}

		for _, org := range conf.Orderer.Organizations {
			mspConfig, err := msp.GetLocalMspConfig(org.MSPDir, org.ID)
			if err != nil {
				logger.Panicf("Error loading MSP configuration for org %s: %s", org.Name, err)
			}
			bs.ordererGroups = append(bs.ordererGroups, configvaluesmsp.TemplateGroupMSP([]string{configtxorderer.GroupKey, org.Name}, mspConfig))
		}

		switch conf.Orderer.OrdererType {
		case ConsensusTypeSolo, ConsensusTypeSbft:
		case ConsensusTypeKafka:
			bs.ordererGroups = append(bs.ordererGroups, configtxorderer.TemplateKafkaBrokers(conf.Orderer.Kafka.Brokers))
		default:
			panic(fmt.Errorf("Wrong consenter type value given: %s", conf.Orderer.OrdererType))
		}

		bs.ordererSystemChannelGroups = []*cb.ConfigGroup{
			// Policies
			configtxorderer.TemplateChainCreationPolicyNames(DefaultChainCreationPolicyNames),
		}
	}

	if conf.Application != nil {

		bs.applicationGroups = []*cb.ConfigGroup{
			// Initialize the default Reader/Writer/Admins application policies
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxapplication.GroupKey}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{configtxapplication.GroupKey}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{configtxapplication.GroupKey}, configvaluesmsp.AdminsPolicyKey),
		}
		for _, org := range conf.Application.Organizations {
			mspConfig, err := msp.GetLocalMspConfig(org.MSPDir, org.ID)
			if err != nil {
				logger.Panicf("Error loading MSP configuration for org %s: %s", org.Name, err)
			}
			bs.ordererGroups = append(bs.ordererGroups, configvaluesmsp.TemplateGroupMSP([]string{configtxapplication.GroupKey, org.Name}, mspConfig))
		}

	}

	return bs
}

func (bs *bootstrapper) ChannelTemplate() configtx.Template {
	return configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(bs.channelGroups...),
		configtx.NewSimpleTemplate(bs.ordererGroups...),
		configtx.NewSimpleTemplate(bs.applicationGroups...),
	)
}

// XXX deprecate and remove
func (bs *bootstrapper) GenesisBlock() *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewCompositeTemplate(
			configtx.NewSimpleTemplate(bs.ordererSystemChannelGroups...),
			bs.ChannelTemplate(),
		),
	).Block(TestChainID)

	if err != nil {
		panic(err)
	}
	return block
}

func (bs *bootstrapper) GenesisBlockForChannel(channelID string) *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewCompositeTemplate(
			configtx.NewSimpleTemplate(bs.ordererSystemChannelGroups...),
			bs.ChannelTemplate(),
		),
	).Block(channelID)

	if err != nil {
		panic(err)
	}
	return block
}
