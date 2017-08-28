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
	"github.com/hyperledger/fabric/common/config"
	configvaluesmsp "github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
)

const (
	pkgLogID = "common/configtx/tool/provisional"
)

var (
	logger *logging.Logger
)

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
	flogging.SetModuleLevel(pkgLogID, "info")
}

// Generator can either create an orderer genesis block or config template
type Generator interface {
	bootstrap.Helper

	// ChannelTemplate returns a template which can be used to help initialize a channel
	ChannelTemplate() configtx.Template

	// GenesisBlockForChannel TODO
	GenesisBlockForChannel(channelID string) *cb.Block
}

const (
	// ConsensusTypeSolo identifies the solo consensus implementation.
	ConsensusTypeSolo = "solo"
	// ConsensusTypeKafka identifies the Kafka-based consensus implementation.
	ConsensusTypeKafka = "kafka"

	// TestChainID is the default value of ChainID. It is used by all testing
	// networks. It it necessary to set and export this variable so that test
	// clients can connect without being rejected for targeting a chain which
	// does not exist.
	TestChainID = "testchainid"

	// BlockValidationPolicyKey TODO
	BlockValidationPolicyKey = "BlockValidation"

	// OrdererAdminsPolicy is the absolute path to the orderer admins policy
	OrdererAdminsPolicy = "/Channel/Orderer/Admins"
)

type bootstrapper struct {
	channelGroups     []*cb.ConfigGroup
	ordererGroups     []*cb.ConfigGroup
	applicationGroups []*cb.ConfigGroup
	consortiumsGroups []*cb.ConfigGroup
}

// New returns a new provisional bootstrap helper.
func New(conf *genesisconfig.Profile) Generator {
	bs := &bootstrapper{
		channelGroups: []*cb.ConfigGroup{
			// Chain Config Types
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),

			// Default policies
			policies.TemplateImplicitMetaAnyPolicy([]string{}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{}, configvaluesmsp.AdminsPolicyKey),
		},
	}

	if conf.Orderer != nil {
		// Orderer addresses
		oa := config.TemplateOrdererAddresses(conf.Orderer.Addresses)
		oa.Values[config.OrdererAddressesKey].ModPolicy = OrdererAdminsPolicy

		bs.ordererGroups = []*cb.ConfigGroup{
			oa,

			// Orderer Config Types
			config.TemplateConsensusType(conf.Orderer.OrdererType),
			config.TemplateBatchSize(&ab.BatchSize{
				MaxMessageCount:   conf.Orderer.BatchSize.MaxMessageCount,
				AbsoluteMaxBytes:  conf.Orderer.BatchSize.AbsoluteMaxBytes,
				PreferredMaxBytes: conf.Orderer.BatchSize.PreferredMaxBytes,
			}),
			config.TemplateBatchTimeout(conf.Orderer.BatchTimeout.String()),
			config.TemplateChannelRestrictions(conf.Orderer.MaxChannels),

			// Initialize the default Reader/Writer/Admins orderer policies, as well as block validation policy
			policies.TemplateImplicitMetaPolicyWithSubPolicy([]string{config.OrdererGroupKey}, BlockValidationPolicyKey, configvaluesmsp.WritersPolicyKey, cb.ImplicitMetaPolicy_ANY),
			policies.TemplateImplicitMetaAnyPolicy([]string{config.OrdererGroupKey}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{config.OrdererGroupKey}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{config.OrdererGroupKey}, configvaluesmsp.AdminsPolicyKey),
		}

		for _, org := range conf.Orderer.Organizations {
			mspConfig, err := msp.GetVerifyingMspConfig(org.MSPDir, org.ID)
			if err != nil {
				logger.Panicf("1 - Error loading MSP configuration for org %s: %s", org.Name, err)
			}
			bs.ordererGroups = append(bs.ordererGroups,
				configvaluesmsp.TemplateGroupMSPWithAdminRolePrincipal([]string{config.OrdererGroupKey, org.Name},
					mspConfig, org.AdminPrincipal == genesisconfig.AdminRoleAdminPrincipal,
				),
			)
		}

		switch conf.Orderer.OrdererType {
		case ConsensusTypeSolo:
		case ConsensusTypeKafka:
			bs.ordererGroups = append(bs.ordererGroups, config.TemplateKafkaBrokers(conf.Orderer.Kafka.Brokers))
		default:
			panic(fmt.Errorf("Wrong consenter type value given: %s", conf.Orderer.OrdererType))
		}
	}

	if conf.Application != nil {

		bs.applicationGroups = []*cb.ConfigGroup{
			// Initialize the default Reader/Writer/Admins application policies
			policies.TemplateImplicitMetaAnyPolicy([]string{config.ApplicationGroupKey}, configvaluesmsp.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{config.ApplicationGroupKey}, configvaluesmsp.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{config.ApplicationGroupKey}, configvaluesmsp.AdminsPolicyKey),
		}
		for _, org := range conf.Application.Organizations {
			mspConfig, err := msp.GetVerifyingMspConfig(org.MSPDir, org.ID)
			if err != nil {
				logger.Panicf("2- Error loading MSP configuration for org %s: %s", org.Name, err)
			}

			bs.applicationGroups = append(bs.applicationGroups,
				configvaluesmsp.TemplateGroupMSPWithAdminRolePrincipal([]string{config.ApplicationGroupKey, org.Name},
					mspConfig, org.AdminPrincipal == genesisconfig.AdminRoleAdminPrincipal,
				),
			)
			var anchorProtos []*pb.AnchorPeer
			for _, anchorPeer := range org.AnchorPeers {
				anchorProtos = append(anchorProtos, &pb.AnchorPeer{
					Host: anchorPeer.Host,
					Port: int32(anchorPeer.Port),
				})
			}

			bs.applicationGroups = append(bs.applicationGroups, config.TemplateAnchorPeers(org.Name, anchorProtos))
		}

	}

	if conf.Consortiums != nil {
		tcg := config.TemplateConsortiumsGroup()
		tcg.Groups[config.ConsortiumsGroupKey].ModPolicy = OrdererAdminsPolicy

		// Fix for https://jira.hyperledger.org/browse/FAB-4373
		// Note, AcceptAllPolicy in this context, does not grant any unrestricted
		// access, but allows the /Channel/Admins policy to evaluate to true
		// for the ordering system channel while set to MAJORITY with the addition
		// to the successful evaluation of the /Channel/Orderer/Admins policy (which
		// is not AcceptAll
		tcg.Groups[config.ConsortiumsGroupKey].Policies[configvaluesmsp.AdminsPolicyKey] = &cb.ConfigPolicy{
			Policy: &cb.Policy{
				Type:  int32(cb.Policy_SIGNATURE),
				Value: utils.MarshalOrPanic(cauthdsl.AcceptAllPolicy),
			},
			ModPolicy: OrdererAdminsPolicy,
		}

		bs.consortiumsGroups = append(bs.consortiumsGroups, tcg)

		for consortiumName, consortium := range conf.Consortiums {
			cg := config.TemplateConsortiumChannelCreationPolicy(consortiumName, policies.ImplicitMetaPolicyWithSubPolicy(
				configvaluesmsp.AdminsPolicyKey,
				cb.ImplicitMetaPolicy_ANY,
			).Policy)

			cg.Groups[config.ConsortiumsGroupKey].Groups[consortiumName].ModPolicy = OrdererAdminsPolicy
			cg.Groups[config.ConsortiumsGroupKey].Groups[consortiumName].Values[config.ChannelCreationPolicyKey].ModPolicy = OrdererAdminsPolicy
			bs.consortiumsGroups = append(bs.consortiumsGroups, cg)

			for _, org := range consortium.Organizations {
				mspConfig, err := msp.GetVerifyingMspConfig(org.MSPDir, org.ID)
				if err != nil {
					logger.Panicf("3 - Error loading MSP configuration for org %s: %s", org.Name, err)
				}
				bs.consortiumsGroups = append(bs.consortiumsGroups,
					configvaluesmsp.TemplateGroupMSPWithAdminRolePrincipal(
						[]string{config.ConsortiumsGroupKey, consortiumName, org.Name},
						mspConfig, org.AdminPrincipal == genesisconfig.AdminRoleAdminPrincipal,
					),
				)
			}
		}
	}

	return bs
}

// ChannelTemplate TODO
func (bs *bootstrapper) ChannelTemplate() configtx.Template {
	return configtx.NewModPolicySettingTemplate(
		configvaluesmsp.AdminsPolicyKey,
		configtx.NewCompositeTemplate(
			configtx.NewSimpleTemplate(bs.channelGroups...),
			configtx.NewSimpleTemplate(bs.ordererGroups...),
			configtx.NewSimpleTemplate(bs.applicationGroups...),
		),
	)
}

// GenesisBlock TODO Deprecate and remove
func (bs *bootstrapper) GenesisBlock() *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewModPolicySettingTemplate(
			configvaluesmsp.AdminsPolicyKey,
			configtx.NewCompositeTemplate(
				configtx.NewSimpleTemplate(bs.consortiumsGroups...),
				bs.ChannelTemplate(),
			),
		),
	).Block(TestChainID)

	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForChannel TODO
func (bs *bootstrapper) GenesisBlockForChannel(channelID string) *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewModPolicySettingTemplate(
			configvaluesmsp.AdminsPolicyKey,
			configtx.NewCompositeTemplate(
				configtx.NewSimpleTemplate(bs.consortiumsGroups...),
				bs.ChannelTemplate(),
			),
		),
	).Block(channelID)

	if err != nil {
		panic(err)
	}
	return block
}
