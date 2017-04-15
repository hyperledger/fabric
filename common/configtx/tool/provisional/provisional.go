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
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/config"
	configvaluesmsp "github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/configtx/tool/provisional")

// Generator can either create an orderer genesis block or config template
type Generator interface {
	bootstrap.Helper

	// ChannelTemplate returns a template which can be used to help initialize a channel
	ChannelTemplate() configtx.Template

	GenesisBlockForChannel(channelID string) *cb.Block
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

	// BlockValidationPolicyKey
	BlockValidationPolicyKey = "BlockValidation"
)

func resolveMSPDir(path string) string {
	if path == "" || path[0] == os.PathSeparator {
		return path
	}

	// Look for MSP dir first in current path, then in ORDERER_CFG_PATH, then PEER_CFG_PATH, and finally in GOPATH
	searchPath := []string{
		".",
		os.Getenv("ORDERER_CFG_PATH"),
		os.Getenv("PEER_CFG_PATH"),
	}

	for _, p := range filepath.SplitList(os.Getenv("GOPATH")) {
		searchPath = append(searchPath, filepath.Join(p, "src/github.com/hyperledger/fabric/common/configtx/tool/"))
	}

	for _, baseDir := range searchPath {
		logger.Infof("Checking for MSPDir at: %s", baseDir)
		fqPath := filepath.Join(baseDir, path)
		if _, err := os.Stat(fqPath); err != nil {
			// The mspdir does not exist
			continue
		}
		return fqPath
	}

	logger.Panicf("Unable to resolve a path for MSPDir: %s", path)

	// Unreachable
	return ""
}

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
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses(conf.Orderer.Addresses), // TODO, move to conf.Channel when it exists

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
			mspConfig, err := msp.GetVerifyingMspConfig(resolveMSPDir(org.MSPDir), org.BCCSP, org.ID)
			if err != nil {
				logger.Panicf("Error loading MSP configuration for org %s: %s", org.Name, err)
			}
			bs.ordererGroups = append(bs.ordererGroups, configvaluesmsp.TemplateGroupMSP([]string{config.OrdererGroupKey, org.Name}, mspConfig))
		}

		switch conf.Orderer.OrdererType {
		case ConsensusTypeSolo, ConsensusTypeSbft:
		case ConsensusTypeKafka:
			bs.ordererGroups = append(bs.ordererGroups, config.TemplateKafkaBrokers(conf.Orderer.Kafka.Brokers))
		default:
			panic(fmt.Errorf("Wrong consenter type value given: %s", conf.Orderer.OrdererType))
		}

		bs.ordererSystemChannelGroups = []*cb.ConfigGroup{
			// Policies
			config.TemplateChainCreationPolicyNames(DefaultChainCreationPolicyNames),
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
			mspConfig, err := msp.GetVerifyingMspConfig(resolveMSPDir(org.MSPDir), org.BCCSP, org.ID)
			if err != nil {
				logger.Panicf("Error loading MSP configuration for org %s: %s", org.Name, err)
			}

			bs.applicationGroups = append(bs.applicationGroups, configvaluesmsp.TemplateGroupMSP([]string{config.ApplicationGroupKey, org.Name}, mspConfig))
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

	return bs
}

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

// XXX deprecate and remove
func (bs *bootstrapper) GenesisBlock() *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewModPolicySettingTemplate(
			configvaluesmsp.AdminsPolicyKey,
			configtx.NewCompositeTemplate(
				configtx.NewSimpleTemplate(bs.ordererSystemChannelGroups...),
				bs.ChannelTemplate(),
			),
		),
	).Block(TestChainID)

	if err != nil {
		panic(err)
	}
	return block
}

func (bs *bootstrapper) GenesisBlockForChannel(channelID string) *cb.Block {
	block, err := genesis.NewFactoryImpl(
		configtx.NewModPolicySettingTemplate(
			configvaluesmsp.AdminsPolicyKey,
			configtx.NewCompositeTemplate(
				configtx.NewSimpleTemplate(bs.ordererSystemChannelGroups...),
				bs.ChannelTemplate(),
			),
		),
	).Block(channelID)

	if err != nil {
		panic(err)
	}
	return block
}
