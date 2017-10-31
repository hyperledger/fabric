/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	pkgLogID                = "common/tools/configtxgen/encoder"
	ordererAdminsPolicyName = "/Channel/Orderer/Admins"

	msgVersion = int32(0)
	epoch      = 0
)

var logger = flogging.MustGetLogger(pkgLogID)

func init() {
	flogging.SetModuleLevel(pkgLogID, "info")
}

const (
	// ConsensusTypeSolo identifies the solo consensus implementation.
	ConsensusTypeSolo = "solo"
	// ConsensusTypeKafka identifies the Kafka-based consensus implementation.
	ConsensusTypeKafka = "kafka"

	// BlockValidationPolicyKey TODO
	BlockValidationPolicyKey = "BlockValidation"

	// OrdererAdminsPolicy is the absolute path to the orderer admins policy
	OrdererAdminsPolicy = "/Channel/Orderer/Admins"
)

func addValue(cg *cb.ConfigGroup, value channelconfig.ConfigValue, modPolicy string) {
	cg.Values[value.Key()] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(value.Value()),
		ModPolicy: modPolicy,
	}
}

func addPolicy(cg *cb.ConfigGroup, policy policies.ConfigPolicy, modPolicy string) {
	cg.Policies[policy.Key()] = &cb.ConfigPolicy{
		Policy:    policy.Value(),
		ModPolicy: modPolicy,
	}
}

// addImplicitMetaPolicyDefaults adds the Readers/Writers/Admins policies, with Any/Any/Majority rules respectively.
func addImplicitMetaPolicyDefaults(cg *cb.ConfigGroup) {
	addPolicy(cg, policies.ImplicitMetaMajorityPolicy(channelconfig.AdminsPolicyKey), channelconfig.AdminsPolicyKey)
	addPolicy(cg, policies.ImplicitMetaAnyPolicy(channelconfig.ReadersPolicyKey), channelconfig.AdminsPolicyKey)
	addPolicy(cg, policies.ImplicitMetaAnyPolicy(channelconfig.WritersPolicyKey), channelconfig.AdminsPolicyKey)
}

// addSignaturePolicyDefaults adds the Readers/Writers/Admins policies as signature policies requiring one signature from the given mspID.
// If devMode is set to true, the Admins policy will accept arbitrary user certs for admin functions, otherwise it requires the cert satisfies
// the admin role principal.
func addSignaturePolicyDefaults(cg *cb.ConfigGroup, mspID string, devMode bool) {
	if devMode {
		addPolicy(cg, policies.SignaturePolicy(channelconfig.AdminsPolicyKey, cauthdsl.SignedByMspMember(mspID)), channelconfig.AdminsPolicyKey)
	} else {
		addPolicy(cg, policies.SignaturePolicy(channelconfig.AdminsPolicyKey, cauthdsl.SignedByMspAdmin(mspID)), channelconfig.AdminsPolicyKey)
	}
	addPolicy(cg, policies.SignaturePolicy(channelconfig.ReadersPolicyKey, cauthdsl.SignedByMspMember(mspID)), channelconfig.AdminsPolicyKey)
	addPolicy(cg, policies.SignaturePolicy(channelconfig.WritersPolicyKey, cauthdsl.SignedByMspMember(mspID)), channelconfig.AdminsPolicyKey)
}

// NewChannelGroup defines the root of the channel configuration.  It defines basic operating principles like the hashing
// algorithm used for the blocks, as well as the location of the ordering service.  It will recursively call into the
// NewOrdererGroup, NewConsortiumsGroup, and NewApplicationGroup depending on whether these sub-elements are set in the
// configuration.  All mod_policy values are set to "Admins" for this group, with the exception of the OrdererAddresses
// value which is set to "/Channel/Orderer/Admins".
func NewChannelGroup(conf *genesisconfig.Profile) (*cb.ConfigGroup, error) {
	if conf.Orderer == nil {
		return nil, errors.New("missing orderer config section")
	}

	channelGroup := cb.NewConfigGroup()
	addImplicitMetaPolicyDefaults(channelGroup)
	addValue(channelGroup, channelconfig.HashingAlgorithmValue(), channelconfig.AdminsPolicyKey)
	addValue(channelGroup, channelconfig.BlockDataHashingStructureValue(), channelconfig.AdminsPolicyKey)
	addValue(channelGroup, channelconfig.OrdererAddressesValue(conf.Orderer.Addresses), ordererAdminsPolicyName)

	if conf.Consortium != "" {
		addValue(channelGroup, channelconfig.ConsortiumValue(conf.Consortium), channelconfig.AdminsPolicyKey)
	}

	if len(conf.Capabilities) > 0 {
		addValue(channelGroup, channelconfig.CapabilitiesValue(conf.Capabilities), channelconfig.AdminsPolicyKey)
	}

	var err error
	channelGroup.Groups[channelconfig.OrdererGroupKey], err = NewOrdererGroup(conf.Orderer)
	if err != nil {
		return nil, errors.Wrap(err, "could not create orderer group")
	}

	if conf.Application != nil {
		channelGroup.Groups[channelconfig.ApplicationGroupKey], err = NewApplicationGroup(conf.Application)
		if err != nil {
			return nil, errors.Wrap(err, "could not create application group")
		}
	}

	if conf.Consortiums != nil {
		channelGroup.Groups[channelconfig.ConsortiumsGroupKey], err = NewConsortiumsGroup(conf.Consortiums)
		if err != nil {
			return nil, errors.Wrap(err, "could not create consortiums group")
		}
	}

	channelGroup.ModPolicy = channelconfig.AdminsPolicyKey
	return channelGroup, nil
}

// NewOrdererGroup returns the orderer component of the channel configuration.  It defines parameters of the ordering service
// about how large blocks should be, how frequently they should be emitted, etc. as well as the organizations of the ordering network.
// It sets the mod_policy of all elements to "Admins".  This group is always present in any channel configuration.
func NewOrdererGroup(conf *genesisconfig.Orderer) (*cb.ConfigGroup, error) {
	ordererGroup := cb.NewConfigGroup()
	addImplicitMetaPolicyDefaults(ordererGroup)
	ordererGroup.Policies[BlockValidationPolicyKey] = &cb.ConfigPolicy{
		Policy:    policies.ImplicitMetaAnyPolicy(channelconfig.WritersPolicyKey).Value(),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}
	addValue(ordererGroup, channelconfig.ConsensusTypeValue(conf.OrdererType), channelconfig.AdminsPolicyKey)
	addValue(ordererGroup, channelconfig.BatchSizeValue(
		conf.BatchSize.MaxMessageCount,
		conf.BatchSize.AbsoluteMaxBytes,
		conf.BatchSize.PreferredMaxBytes,
	), channelconfig.AdminsPolicyKey)
	addValue(ordererGroup, channelconfig.BatchTimeoutValue(conf.BatchTimeout.String()), channelconfig.AdminsPolicyKey)
	addValue(ordererGroup, channelconfig.ChannelRestrictionsValue(conf.MaxChannels), channelconfig.AdminsPolicyKey)

	if len(conf.Capabilities) > 0 {
		addValue(ordererGroup, channelconfig.CapabilitiesValue(conf.Capabilities), channelconfig.AdminsPolicyKey)
	}

	switch conf.OrdererType {
	case ConsensusTypeSolo:
	case ConsensusTypeKafka:
		addValue(ordererGroup, channelconfig.KafkaBrokersValue(conf.Kafka.Brokers), channelconfig.AdminsPolicyKey)
	default:
		return nil, errors.Errorf("unknown orderer type: %s", conf.OrdererType)
	}

	for _, org := range conf.Organizations {
		var err error
		ordererGroup.Groups[org.Name], err = NewOrdererOrgGroup(org)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create orderer org")
		}
	}

	ordererGroup.ModPolicy = channelconfig.AdminsPolicyKey
	return ordererGroup, nil
}

// NewOrdererOrgGroup returns an orderer org component of the channel configuration.  It defines the crypto material for the
// organization (its MSP).  It sets the mod_policy of all elements to "Admins".
func NewOrdererOrgGroup(conf *genesisconfig.Organization) (*cb.ConfigGroup, error) {
	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org: %s", conf.Name)
	}

	ordererOrgGroup := cb.NewConfigGroup()
	addSignaturePolicyDefaults(ordererOrgGroup, conf.ID, conf.AdminPrincipal != genesisconfig.AdminRoleAdminPrincipal)
	addValue(ordererOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	ordererOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey
	return ordererOrgGroup, nil
}

// NewApplicationGroup returns the application component of the channel configuration.  It defines the organizations which are involved
// in application logic like chaincodes, and how these members may interact with the orderer.  It sets the mod_policy of all elements to "Admins".
func NewApplicationGroup(conf *genesisconfig.Application) (*cb.ConfigGroup, error) {
	applicationGroup := cb.NewConfigGroup()
	addImplicitMetaPolicyDefaults(applicationGroup)

	if len(conf.Capabilities) > 0 {
		addValue(applicationGroup, channelconfig.CapabilitiesValue(conf.Capabilities), channelconfig.AdminsPolicyKey)
	}

	for _, org := range conf.Organizations {
		var err error
		applicationGroup.Groups[org.Name], err = NewApplicationOrgGroup(org)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create application org")
		}
	}

	applicationGroup.ModPolicy = channelconfig.AdminsPolicyKey
	return applicationGroup, nil
}

// NewApplicationOrgGroup returns an application org component of the channel configuration.  It defines the crypto material for the organization
// (its MSP) as well as its anchor peers for use by the gossip network.  It sets the mod_policy of all elements to "Admins".
func NewApplicationOrgGroup(conf *genesisconfig.Organization) (*cb.ConfigGroup, error) {
	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org %s: %s", conf.Name)
	}

	applicationOrgGroup := cb.NewConfigGroup()
	addSignaturePolicyDefaults(applicationOrgGroup, conf.ID, conf.AdminPrincipal != genesisconfig.AdminRoleAdminPrincipal)
	addValue(applicationOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	var anchorProtos []*pb.AnchorPeer
	for _, anchorPeer := range conf.AnchorPeers {
		anchorProtos = append(anchorProtos, &pb.AnchorPeer{
			Host: anchorPeer.Host,
			Port: int32(anchorPeer.Port),
		})
	}
	addValue(applicationOrgGroup, channelconfig.AnchorPeersValue(anchorProtos), channelconfig.AdminsPolicyKey)

	applicationOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey
	return applicationOrgGroup, nil
}

// NewConsortiumsGroup returns the consortiums component of the channel configuration.  This element is only defined for the ordering system channel.
// It sets the mod_policy for all elements to "/Channel/Orderer/Admins".
func NewConsortiumsGroup(conf map[string]*genesisconfig.Consortium) (*cb.ConfigGroup, error) {
	consortiumsGroup := cb.NewConfigGroup()
	// This policy is not referenced anywhere, it is only used as part of the implicit meta policy rule at the channel level, so this setting
	// effectively degrades control of the ordering system channel to the ordering admins
	addPolicy(consortiumsGroup, policies.SignaturePolicy(channelconfig.AdminsPolicyKey, cauthdsl.AcceptAllPolicy), ordererAdminsPolicyName)

	for consortiumName, consortium := range conf {
		var err error
		consortiumsGroup.Groups[consortiumName], err = NewConsortiumGroup(consortium)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create consortium %s", consortiumName)
		}
	}

	consortiumsGroup.ModPolicy = ordererAdminsPolicyName
	return consortiumsGroup, nil
}

// NewConsortiums returns a consortiums component of the channel configuration.  Each consortium defines the organizations which may be involved in channel
// creation, as well as the channel creation policy the orderer checks at channel creation time to authorize the action.  It sets the mod_policy of all
// elements to "/Channel/Orderer/Admins".
func NewConsortiumGroup(conf *genesisconfig.Consortium) (*cb.ConfigGroup, error) {
	consortiumGroup := cb.NewConfigGroup()

	for _, org := range conf.Organizations {
		var err error
		// Note, NewOrdererOrgGroup is correct here, as the structure is identical
		consortiumGroup.Groups[org.Name], err = NewOrdererOrgGroup(org)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create consortium org")
		}
	}

	addValue(consortiumGroup, channelconfig.ChannelCreationPolicyValue(policies.ImplicitMetaAnyPolicy(channelconfig.AdminsPolicyKey).Value()), ordererAdminsPolicyName)

	consortiumGroup.ModPolicy = ordererAdminsPolicyName
	return consortiumGroup, nil
}

// NewChannelCreateConfigUpdate generates a ConfigUpdate which can be sent to the orderer to create a new channel.  Optionally, the channel group of the
// ordering system channel may be passed in, and the resulting ConfigUpdate will extract the appropriate versions from this file.
func NewChannelCreateConfigUpdate(channelID, consortiumName string, orgs []string, orderingSystemChannelGroup *cb.ConfigGroup) (*cb.ConfigUpdate, error) {
	var applicationGroup *cb.ConfigGroup
	channelGroupVersion := uint64(0)

	if orderingSystemChannelGroup != nil {
		// In the case that a ordering system channel definition was provided, pull the appropriate versions
		if orderingSystemChannelGroup.Groups == nil {
			return nil, errors.New("missing all channel groups")
		}

		consortiums, ok := orderingSystemChannelGroup.Groups[channelconfig.ConsortiumsGroupKey]
		if !ok {
			return nil, errors.New("bad consortiums group")
		}

		consortium, ok := consortiums.Groups[consortiumName]
		if !ok || (len(orgs) > 0 && consortium.Groups == nil) {
			return nil, errors.Errorf("bad consortium: %s", consortiumName)
		}

		applicationGroup = cb.NewConfigGroup()
		for _, org := range orgs {
			orgGroup, ok := consortium.Groups[org]
			if !ok {
				return nil, errors.Errorf("missing organization: %s", org)
			}
			applicationGroup.Groups[org] = &cb.ConfigGroup{Version: orgGroup.Version}
		}

		channelGroupVersion = orderingSystemChannelGroup.Version
	} else {
		// Otherwise assume the orgs have not been modified
		applicationGroup = cb.NewConfigGroup()
		for _, org := range orgs {
			applicationGroup.Groups[org] = &cb.ConfigGroup{}
		}
	}

	rSet := cb.NewConfigGroup()
	rSet.Version = channelGroupVersion

	// add the consortium name to the rSet

	addValue(rSet, channelconfig.ConsortiumValue(consortiumName), "") // TODO, this emulates the old behavior, but is it desirable?

	// create the new channel's application group

	rSet.Groups[channelconfig.ApplicationGroupKey] = applicationGroup

	wSet := proto.Clone(rSet).(*cb.ConfigGroup)

	applicationGroup = wSet.Groups[channelconfig.ApplicationGroupKey]
	applicationGroup.Version = 1
	applicationGroup.Policies = make(map[string]*cb.ConfigPolicy)
	addImplicitMetaPolicyDefaults(applicationGroup)
	applicationGroup.ModPolicy = channelconfig.AdminsPolicyKey

	return &cb.ConfigUpdate{
		ChannelId: channelID,
		ReadSet:   rSet,
		WriteSet:  wSet,
	}, nil
}

// MakeChannelCreationTransaction is a handy utility function for creating transactions for channel creation
func MakeChannelCreationTransaction(channelID string, consortium string, signer msp.SigningIdentity, orderingSystemChannelConfigGroup *cb.ConfigGroup, orgs ...string) (*cb.Envelope, error) {
	newChannelConfigUpdate, err := NewChannelCreateConfigUpdate(channelID, consortium, orgs, orderingSystemChannelConfigGroup)
	if err != nil {
		return nil, errors.Wrap(err, "config update generation failure")
	}

	newConfigUpdateEnv := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(newChannelConfigUpdate),
	}

	payloadSignatureHeader := &cb.SignatureHeader{}
	if signer != nil {
		sSigner, err := signer.Serialize()
		if err != nil {
			return nil, errors.Wrap(err, "serialization of identity failed")
		}

		newConfigUpdateEnv.Signatures = []*cb.ConfigSignature{&cb.ConfigSignature{
			SignatureHeader: utils.MarshalOrPanic(utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())),
		}}

		newConfigUpdateEnv.Signatures[0].Signature, err = signer.Sign(util.ConcatenateBytes(newConfigUpdateEnv.Signatures[0].SignatureHeader, newConfigUpdateEnv.ConfigUpdate))
		if err != nil {
			return nil, errors.Wrap(err, "signature failure over config update")
		}

		payloadSignatureHeader = utils.MakeSignatureHeader(sSigner, utils.CreateNonceOrPanic())
	}

	payloadChannelHeader := utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, msgVersion, channelID, epoch)
	utils.SetTxID(payloadChannelHeader, payloadSignatureHeader)
	payloadHeader := utils.MakePayloadHeader(payloadChannelHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: utils.MarshalOrPanic(newConfigUpdateEnv)}
	paylBytes := utils.MarshalOrPanic(payload)

	var sig []byte
	if signer != nil {
		// sign the payload
		sig, err = signer.Sign(paylBytes)
		if err != nil {
			return nil, errors.Wrap(err, "signature failure over config update envelope")
		}
	}

	return &cb.Envelope{Payload: paylBytes, Signature: sig}, nil
}

// Bootstrapper is a wrapper around NewChannelConfigGroup which can produce genesis blocks
type Bootstrapper struct {
	channelGroup *cb.ConfigGroup
}

// New creates a new Bootstrapper for generating genesis blocks
func New(config *genesisconfig.Profile) *Bootstrapper {
	channelGroup, err := NewChannelGroup(config)
	if err != nil {
		logger.Panicf("Error creating channel group: %s", err)
	}
	return &Bootstrapper{
		channelGroup: channelGroup,
	}
}

// GenesisBlock produces a genesis block for the default test chain id
func (bs *Bootstrapper) GenesisBlock() *cb.Block {
	block, err := genesis.NewFactoryImpl(bs.channelGroup).Block(genesisconfig.TestChainID)
	if err != nil {
		logger.Panicf("Error creating genesis block from channel group: %s", err)
	}
	return block
}

// GenesisBlockForChannel produces a genesis block for a given channel ID
func (bs *Bootstrapper) GenesisBlockForChannel(channelID string) *cb.Block {
	block, err := genesis.NewFactoryImpl(bs.channelGroup).Block(channelID)
	if err != nil {
		logger.Panicf("Error creating genesis block from channel group: %s", err)
	}
	return block
}
