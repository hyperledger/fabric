/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package provisional

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
)

const (
	pkgLogID = "common/tools/configtxgen/provisional"
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
	// networks. It is necessary to set and export this variable so that test
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
			channelconfig.DefaultHashingAlgorithm(),
			channelconfig.DefaultBlockDataHashingStructure(),

			// Default policies
			policies.TemplateImplicitMetaAnyPolicy([]string{}, channelconfig.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{}, channelconfig.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{}, channelconfig.AdminsPolicyKey),
		},
	}

	if len(conf.Capabilities) > 0 {
		bs.channelGroups = append(bs.channelGroups, channelconfig.TemplateChannelCapabilities(conf.Capabilities))
	}

	if conf.Orderer != nil {
		// Orderer addresses
		oa := channelconfig.TemplateOrdererAddresses(conf.Orderer.Addresses)
		oa.Values[channelconfig.OrdererAddressesKey].ModPolicy = OrdererAdminsPolicy

		bs.ordererGroups = []*cb.ConfigGroup{
			oa,

			// Orderer Config Types
			channelconfig.TemplateConsensusType(conf.Orderer.OrdererType),
			channelconfig.TemplateBatchSize(&ab.BatchSize{
				MaxMessageCount:   conf.Orderer.BatchSize.MaxMessageCount,
				AbsoluteMaxBytes:  conf.Orderer.BatchSize.AbsoluteMaxBytes,
				PreferredMaxBytes: conf.Orderer.BatchSize.PreferredMaxBytes,
			}),
			channelconfig.TemplateBatchTimeout(conf.Orderer.BatchTimeout.String()),
			channelconfig.TemplateChannelRestrictions(conf.Orderer.MaxChannels),

			// Initialize the default Reader/Writer/Admins orderer policies, as well as block validation policy
			policies.TemplateImplicitMetaPolicyWithSubPolicy([]string{channelconfig.OrdererGroupKey}, BlockValidationPolicyKey, channelconfig.WritersPolicyKey, cb.ImplicitMetaPolicy_ANY),
			policies.TemplateImplicitMetaAnyPolicy([]string{channelconfig.OrdererGroupKey}, channelconfig.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{channelconfig.OrdererGroupKey}, channelconfig.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{channelconfig.OrdererGroupKey}, channelconfig.AdminsPolicyKey),
		}

		if len(conf.Orderer.Capabilities) > 0 {
			bs.ordererGroups = append(bs.ordererGroups, channelconfig.TemplateOrdererCapabilities(conf.Orderer.Capabilities))
		}

		for _, org := range conf.Orderer.Organizations {
			mspConfig, err := msp.GetVerifyingMspConfig(org.MSPDir, org.ID)
			if err != nil {
				logger.Panicf("1 - Error loading MSP configuration for org %s: %s", org.Name, err)
			}
			bs.ordererGroups = append(bs.ordererGroups,
				channelconfig.TemplateGroupMSPWithAdminRolePrincipal([]string{channelconfig.OrdererGroupKey, org.Name},
					mspConfig, org.AdminPrincipal == genesisconfig.AdminRoleAdminPrincipal,
				),
			)
		}

		switch conf.Orderer.OrdererType {
		case ConsensusTypeSolo:
		case ConsensusTypeKafka:
			bs.ordererGroups = append(bs.ordererGroups, channelconfig.TemplateKafkaBrokers(conf.Orderer.Kafka.Brokers))
		default:
			panic(fmt.Errorf("Wrong consenter type value given: %s", conf.Orderer.OrdererType))
		}
	}

	if conf.Application != nil {

		bs.applicationGroups = []*cb.ConfigGroup{
			// Initialize the default Reader/Writer/Admins application policies
			policies.TemplateImplicitMetaAnyPolicy([]string{channelconfig.ApplicationGroupKey}, channelconfig.ReadersPolicyKey),
			policies.TemplateImplicitMetaAnyPolicy([]string{channelconfig.ApplicationGroupKey}, channelconfig.WritersPolicyKey),
			policies.TemplateImplicitMetaMajorityPolicy([]string{channelconfig.ApplicationGroupKey}, channelconfig.AdminsPolicyKey),
		}

		if len(conf.Application.Capabilities) > 0 {
			bs.applicationGroups = append(bs.applicationGroups, channelconfig.TemplateApplicationCapabilities(conf.Application.Capabilities))
		}

		for _, org := range conf.Application.Organizations {
			mspConfig, err := msp.GetVerifyingMspConfig(org.MSPDir, org.ID)
			if err != nil {
				logger.Panicf("2- Error loading MSP configuration for org %s: %s", org.Name, err)
			}

			bs.applicationGroups = append(bs.applicationGroups,
				channelconfig.TemplateGroupMSPWithAdminRolePrincipal([]string{channelconfig.ApplicationGroupKey, org.Name},
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

			bs.applicationGroups = append(bs.applicationGroups, channelconfig.TemplateAnchorPeers(org.Name, anchorProtos))
		}

	}

	if conf.Consortiums != nil {
		tcg := channelconfig.TemplateConsortiumsGroup()
		tcg.Groups[channelconfig.ConsortiumsGroupKey].ModPolicy = OrdererAdminsPolicy

		// Fix for https://jira.hyperledger.org/browse/FAB-4373
		// Note, AcceptAllPolicy in this context, does not grant any unrestricted
		// access, but allows the /Channel/Admins policy to evaluate to true
		// for the ordering system channel while set to MAJORITY with the addition
		// to the successful evaluation of the /Channel/Orderer/Admins policy (which
		// is not AcceptAll
		tcg.Groups[channelconfig.ConsortiumsGroupKey].Policies[channelconfig.AdminsPolicyKey] = &cb.ConfigPolicy{
			Policy: &cb.Policy{
				Type:  int32(cb.Policy_SIGNATURE),
				Value: utils.MarshalOrPanic(cauthdsl.AcceptAllPolicy),
			},
			ModPolicy: OrdererAdminsPolicy,
		}

		bs.consortiumsGroups = append(bs.consortiumsGroups, tcg)

		for consortiumName, consortium := range conf.Consortiums {
			cg := channelconfig.TemplateConsortiumChannelCreationPolicy(consortiumName, policies.ImplicitMetaPolicyWithSubPolicy(
				channelconfig.AdminsPolicyKey,
				cb.ImplicitMetaPolicy_ANY,
			).Policy)

			cg.Groups[channelconfig.ConsortiumsGroupKey].Groups[consortiumName].ModPolicy = OrdererAdminsPolicy
			cg.Groups[channelconfig.ConsortiumsGroupKey].Groups[consortiumName].Values[channelconfig.ChannelCreationPolicyKey].ModPolicy = OrdererAdminsPolicy
			bs.consortiumsGroups = append(bs.consortiumsGroups, cg)

			for _, org := range consortium.Organizations {
				mspConfig, err := msp.GetVerifyingMspConfig(org.MSPDir, org.ID)
				if err != nil {
					logger.Panicf("3 - Error loading MSP configuration for org %s: %s", org.Name, err)
				}
				bs.consortiumsGroups = append(bs.consortiumsGroups,
					channelconfig.TemplateGroupMSPWithAdminRolePrincipal(
						[]string{channelconfig.ConsortiumsGroupKey, consortiumName, org.Name},
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
		channelconfig.AdminsPolicyKey,
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
			channelconfig.AdminsPolicyKey,
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
			channelconfig.AdminsPolicyKey,
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
