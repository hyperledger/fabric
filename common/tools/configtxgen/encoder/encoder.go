/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	ordererAdminsPolicyName = "/Channel/Orderer/Admins"

	msgVersion = int32(0)
	epoch      = 0
)

var logger = flogging.MustGetLogger("common.tools.configtxgen.encoder")

const (
	// ConsensusTypeSolo identifies the solo consensus implementation.
	ConsensusTypeSolo = "solo"
	// ConsensusTypeKafka identifies the Kafka-based consensus implementation.
	ConsensusTypeKafka = "kafka"

	// BlockValidationPolicyKey TODO
	BlockValidationPolicyKey = "BlockValidation"

	// OrdererAdminsPolicy is the absolute path to the orderer admins policy
	OrdererAdminsPolicy = "/Channel/Orderer/Admins"

	// SignaturePolicyType is the 'Type' string for signature policies
	SignaturePolicyType = "Signature"

	// ImplicitMetaPolicyType is the 'Type' string for implicit meta policies
	ImplicitMetaPolicyType = "ImplicitMeta"
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

func addPolicies(cg *cb.ConfigGroup, policyMap map[string]*genesisconfig.Policy, modPolicy string) error {
	for policyName, policy := range policyMap {
		switch policy.Type {
		case ImplicitMetaPolicyType:
			imp, err := policies.ImplicitMetaFromString(policy.Rule)
			if err != nil {
				return errors.Wrapf(err, "invalid implicit meta policy rule '%s'", policy.Rule)
			}
			cg.Policies[policyName] = &cb.ConfigPolicy{
				ModPolicy: modPolicy,
				Policy: &cb.Policy{
					Type:  int32(cb.Policy_IMPLICIT_META),
					Value: utils.MarshalOrPanic(imp),
				},
			}
		case SignaturePolicyType:
			sp, err := cauthdsl.FromString(policy.Rule)
			if err != nil {
				return errors.Wrapf(err, "invalid signature policy rule '%s'", policy.Rule)
			}
			cg.Policies[policyName] = &cb.ConfigPolicy{
				ModPolicy: modPolicy,
				Policy: &cb.Policy{
					Type:  int32(cb.Policy_SIGNATURE),
					Value: utils.MarshalOrPanic(sp),
				},
			}
		default:
			return errors.Errorf("unknown policy type: %s", policy.Type)
		}
	}
	return nil
}

// addImplicitMetaPolicyDefaults adds the Readers/Writers/Admins policies, with Any/Any/Majority rules respectively.
func addImplicitMetaPolicyDefaults(cg *cb.ConfigGroup) {
	addPolicy(cg, policies.ImplicitMetaMajorityPolicy(channelconfig.AdminsPolicyKey), channelconfig.AdminsPolicyKey)
	addPolicy(cg, policies.ImplicitMetaAnyPolicy(channelconfig.ReadersPolicyKey), channelconfig.AdminsPolicyKey)
	addPolicy(cg, policies.ImplicitMetaAnyPolicy(channelconfig.WritersPolicyKey), channelconfig.AdminsPolicyKey)
}

// addOrdererImplicitMetaPolicyDefaults adds the orderer's Readers/Writers/Admins/BlockValidation policies, with Any/Any/Majority/Any rules respectively.
func addOrdererImplicitMetaPolicyDefaults(cg *cb.ConfigGroup) {
	cg.Policies[BlockValidationPolicyKey] = &cb.ConfigPolicy{
		Policy:    policies.ImplicitMetaAnyPolicy(channelconfig.WritersPolicyKey).Value(),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}
	addImplicitMetaPolicyDefaults(cg)
}

// addSignaturePolicyDefaults adds the Readers/Writers/Admins policies as signature policies requiring one signature from the given mspID.
// If devMode is set to true, the Admins policy will accept arbitrary user certs for admin functions, otherwise it requires the cert satisfies
// the admin role principal.
func addSignaturePolicyDefaults(cg *cb.ConfigGroup, mspID string, devMode bool) {
	if devMode {
		logger.Warningf("Specifying AdminPrincipal is deprecated and will be removed in a future release, override the admin principal with explicit policies.")
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
	channelGroup := cb.NewConfigGroup()
	if len(conf.Policies) == 0 {
		logger.Warningf("Default policy emission is deprecated, please include policy specifications for the channel group in configtx.yaml")
		addImplicitMetaPolicyDefaults(channelGroup)
	} else {
		if err := addPolicies(channelGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
			return nil, errors.Wrapf(err, "error adding policies to channel group")
		}
	}

	addValue(channelGroup, channelconfig.HashingAlgorithmValue(), channelconfig.AdminsPolicyKey)
	addValue(channelGroup, channelconfig.BlockDataHashingStructureValue(), channelconfig.AdminsPolicyKey)
	if conf.Orderer != nil && len(conf.Orderer.Addresses) > 0 {
		addValue(channelGroup, channelconfig.OrdererAddressesValue(conf.Orderer.Addresses), ordererAdminsPolicyName)
	}

	if conf.Consortium != "" {
		addValue(channelGroup, channelconfig.ConsortiumValue(conf.Consortium), channelconfig.AdminsPolicyKey)
	}

	if len(conf.Capabilities) > 0 {
		addValue(channelGroup, channelconfig.CapabilitiesValue(conf.Capabilities), channelconfig.AdminsPolicyKey)
	}

	var err error
	if conf.Orderer != nil {
		channelGroup.Groups[channelconfig.OrdererGroupKey], err = NewOrdererGroup(conf.Orderer)
		if err != nil {
			return nil, errors.Wrap(err, "could not create orderer group")
		}
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
	if len(conf.Policies) == 0 {
		logger.Warningf("Default policy emission is deprecated, please include policy specifications for the orderer group in configtx.yaml")
		addOrdererImplicitMetaPolicyDefaults(ordererGroup)
	} else {
		if err := addPolicies(ordererGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
			return nil, errors.Wrapf(err, "error adding policies to orderer group")
		}
	}
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

	var consensusMetadata []byte
	var err error

	switch conf.OrdererType {
	case ConsensusTypeSolo:
	case ConsensusTypeKafka:
		addValue(ordererGroup, channelconfig.KafkaBrokersValue(conf.Kafka.Brokers), channelconfig.AdminsPolicyKey)
	case etcdraft.TypeKey:
		if consensusMetadata, err = etcdraft.Marshal(conf.EtcdRaft); err != nil {
			return nil, errors.Errorf("cannot marshal metadata for orderer type %s: %s", etcdraft.TypeKey, err)
		}
	default:
		return nil, errors.Errorf("unknown orderer type: %s", conf.OrdererType)
	}

	addValue(ordererGroup, channelconfig.ConsensusTypeValue(conf.OrdererType, consensusMetadata), channelconfig.AdminsPolicyKey)

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

// NewConsortiumsGroup returns an org component of the channel configuration.  It defines the crypto material for the
// organization (its MSP).  It sets the mod_policy of all elements to "Admins".
func NewConsortiumOrgGroup(conf *genesisconfig.Organization) (*cb.ConfigGroup, error) {
	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org: %s", conf.Name)
	}

	consortiumOrgGroup := cb.NewConfigGroup()
	if len(conf.Policies) == 0 {
		logger.Warningf("Default policy emission is deprecated, please include policy specifications for the orderer org group %s in configtx.yaml", conf.Name)
		addSignaturePolicyDefaults(consortiumOrgGroup, conf.ID, conf.AdminPrincipal != genesisconfig.AdminRoleAdminPrincipal)
	} else {
		if err := addPolicies(consortiumOrgGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
			return nil, errors.Wrapf(err, "error adding policies to orderer org group '%s'", conf.Name)
		}
	}

	addValue(consortiumOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	consortiumOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey

	return consortiumOrgGroup, nil
}

// NewOrdererOrgGroup returns an orderer org component of the channel configuration.  It defines the crypto material for the
// organization (its MSP).  It sets the mod_policy of all elements to "Admins".
func NewOrdererOrgGroup(conf *genesisconfig.Organization) (*cb.ConfigGroup, error) {
	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org: %s", conf.Name)
	}

	ordererOrgGroup := cb.NewConfigGroup()
	if len(conf.Policies) == 0 {
		logger.Warningf("Default policy emission is deprecated, please include policy specifications for the orderer org group %s in configtx.yaml", conf.Name)
		addSignaturePolicyDefaults(ordererOrgGroup, conf.ID, conf.AdminPrincipal != genesisconfig.AdminRoleAdminPrincipal)
	} else {
		if err := addPolicies(ordererOrgGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
			return nil, errors.Wrapf(err, "error adding policies to orderer org group '%s'", conf.Name)
		}
	}

	addValue(ordererOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	ordererOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey

	if len(conf.OrdererEndpoints) > 0 {
		addValue(ordererOrgGroup, channelconfig.EndpointsValue(conf.OrdererEndpoints), channelconfig.AdminsPolicyKey)
	}

	return ordererOrgGroup, nil
}

// NewApplicationGroup returns the application component of the channel configuration.  It defines the organizations which are involved
// in application logic like chaincodes, and how these members may interact with the orderer.  It sets the mod_policy of all elements to "Admins".
func NewApplicationGroup(conf *genesisconfig.Application) (*cb.ConfigGroup, error) {
	applicationGroup := cb.NewConfigGroup()
	if len(conf.Policies) == 0 {
		logger.Warningf("Default policy emission is deprecated, please include policy specifications for the application group in configtx.yaml")
		addImplicitMetaPolicyDefaults(applicationGroup)
	} else {
		if err := addPolicies(applicationGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
			return nil, errors.Wrapf(err, "error adding policies to application group")
		}
	}

	if len(conf.ACLs) > 0 {
		addValue(applicationGroup, channelconfig.ACLValues(conf.ACLs), channelconfig.AdminsPolicyKey)
	}

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
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org %s", conf.Name)
	}

	applicationOrgGroup := cb.NewConfigGroup()
	if len(conf.Policies) == 0 {
		logger.Warningf("Default policy emission is deprecated, please include policy specifications for the application org group %s in configtx.yaml", conf.Name)
		addSignaturePolicyDefaults(applicationOrgGroup, conf.ID, conf.AdminPrincipal != genesisconfig.AdminRoleAdminPrincipal)
	} else {
		if err := addPolicies(applicationOrgGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
			return nil, errors.Wrapf(err, "error adding policies to application org group %s", conf.Name)
		}
	}
	addValue(applicationOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	var anchorProtos []*pb.AnchorPeer
	for _, anchorPeer := range conf.AnchorPeers {
		anchorProtos = append(anchorProtos, &pb.AnchorPeer{
			Host: anchorPeer.Host,
			Port: int32(anchorPeer.Port),
		})
	}

	// Avoid adding an unnecessary anchor peers element when one is not required.  This helps
	// prevent a delta from the orderer system channel when computing more complex channel
	// creation transactions
	if len(anchorProtos) > 0 {
		addValue(applicationOrgGroup, channelconfig.AnchorPeersValue(anchorProtos), channelconfig.AdminsPolicyKey)
	}

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
		consortiumGroup.Groups[org.Name], err = NewConsortiumOrgGroup(org)
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
func NewChannelCreateConfigUpdate(channelID string, conf *genesisconfig.Profile, templateConfig *cb.ConfigGroup) (*cb.ConfigUpdate, error) {
	if conf.Application == nil {
		return nil, errors.New("cannot define a new channel with no Application section")
	}

	if conf.Consortium == "" {
		return nil, errors.New("cannot define a new channel with no Consortium value")
	}

	newChannelGroup, err := NewChannelGroup(conf)
	if err != nil {
		return nil, errors.Wrapf(err, "could not turn parse profile into channel group")
	}

	updt, err := update.Compute(&cb.Config{ChannelGroup: templateConfig}, &cb.Config{ChannelGroup: newChannelGroup})
	if err != nil {
		return nil, errors.Wrapf(err, "could not compute update")
	}

	// Add the consortium name to create the channel for into the write set as required.
	updt.ChannelId = channelID
	updt.ReadSet.Values[channelconfig.ConsortiumKey] = &cb.ConfigValue{Version: 0}
	updt.WriteSet.Values[channelconfig.ConsortiumKey] = &cb.ConfigValue{
		Version: 0,
		Value: utils.MarshalOrPanic(&cb.Consortium{
			Name: conf.Consortium,
		}),
	}

	return updt, nil
}

// DefaultConfigTemplate generates a config template based on the assumption that
// the input profile is a channel creation template and no system channel context
// is available.
func DefaultConfigTemplate(conf *genesisconfig.Profile) (*cb.ConfigGroup, error) {
	channelGroup, err := NewChannelGroup(conf)
	if err != nil {
		return nil, errors.WithMessage(err, "error parsing configuration")
	}

	if _, ok := channelGroup.Groups[channelconfig.ApplicationGroupKey]; !ok {
		return nil, errors.New("channel template configs must contain an application section")
	}

	channelGroup.Groups[channelconfig.ApplicationGroupKey].Values = nil
	channelGroup.Groups[channelconfig.ApplicationGroupKey].Policies = nil

	return channelGroup, nil
}

func ConfigTemplateFromGroup(conf *genesisconfig.Profile, cg *cb.ConfigGroup) (*cb.ConfigGroup, error) {
	template := proto.Clone(cg).(*cb.ConfigGroup)
	if template.Groups == nil {
		return nil, errors.Errorf("supplied system channel group has no sub-groups")
	}

	template.Groups[channelconfig.ApplicationGroupKey] = &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{},
	}

	consortiums, ok := template.Groups[channelconfig.ConsortiumsGroupKey]
	if !ok {
		return nil, errors.Errorf("supplied system channel group does not appear to be system channel (missing consortiums group)")
	}

	if consortiums.Groups == nil {
		return nil, errors.Errorf("system channel consortiums group appears to have no consortiums defined")
	}

	consortium, ok := consortiums.Groups[conf.Consortium]
	if !ok {
		return nil, errors.Errorf("supplied system channel group is missing '%s' consortium", conf.Consortium)
	}

	if conf.Application == nil {
		return nil, errors.Errorf("supplied channel creation profile does not contain an application section")
	}

	for _, organization := range conf.Application.Organizations {
		var ok bool
		template.Groups[channelconfig.ApplicationGroupKey].Groups[organization.Name], ok = consortium.Groups[organization.Name]
		if !ok {
			return nil, errors.Errorf("consortium %s does not contain member org %s", conf.Consortium, organization.Name)
		}
	}
	delete(template.Groups, channelconfig.ConsortiumsGroupKey)

	addValue(template, channelconfig.ConsortiumValue(conf.Consortium), channelconfig.AdminsPolicyKey)

	return template, nil
}

// MakeChannelCreationTransaction is a handy utility function for creating transactions for channel creation.
// It assumes the invoker has no system channel context so ignores all but the application section.
func MakeChannelCreationTransaction(channelID string, signer crypto.LocalSigner, conf *genesisconfig.Profile) (*cb.Envelope, error) {
	template, err := DefaultConfigTemplate(conf)
	if err != nil {
		return nil, errors.WithMessage(err, "could not generate default config template")
	}
	return MakeChannelCreationTransactionFromTemplate(channelID, signer, conf, template)
}

// MakeChannelCreationTransactionWithSystemChannelContext is a utility function for creating channel creation txes.
// It requires a configuration representing the orderer system channel to allow more sophisticated channel creation
// transactions modifying pieces of the configuration like the orderer set.
func MakeChannelCreationTransactionWithSystemChannelContext(channelID string, signer crypto.LocalSigner, conf, systemChannelConf *genesisconfig.Profile) (*cb.Envelope, error) {
	cg, err := NewChannelGroup(systemChannelConf)
	if err != nil {
		return nil, errors.WithMessage(err, "could not parse system channel config")
	}

	template, err := ConfigTemplateFromGroup(conf, cg)
	if err != nil {
		return nil, errors.WithMessage(err, "could not create config template")
	}

	return MakeChannelCreationTransactionFromTemplate(channelID, signer, conf, template)
}

// MakeChannelCreationTransactionFromTemplate creates a transaction for creating a channel.  It uses
// the given template to produce the config update set.  Usually, the caller will want to invoke
// MakeChannelCreationTransaction or MakeChannelCreationTransactionWithSystemChannelContext.
func MakeChannelCreationTransactionFromTemplate(channelID string, signer crypto.LocalSigner, conf *genesisconfig.Profile, template *cb.ConfigGroup) (*cb.Envelope, error) {
	newChannelConfigUpdate, err := NewChannelCreateConfigUpdate(channelID, conf, template)
	if err != nil {
		return nil, errors.Wrap(err, "config update generation failure")
	}

	newConfigUpdateEnv := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(newChannelConfigUpdate),
	}

	if signer != nil {
		sigHeader, err := signer.NewSignatureHeader()
		if err != nil {
			return nil, errors.Wrap(err, "creating signature header failed")
		}

		newConfigUpdateEnv.Signatures = []*cb.ConfigSignature{{
			SignatureHeader: utils.MarshalOrPanic(sigHeader),
		}}

		newConfigUpdateEnv.Signatures[0].Signature, err = signer.Sign(util.ConcatenateBytes(newConfigUpdateEnv.Signatures[0].SignatureHeader, newConfigUpdateEnv.ConfigUpdate))
		if err != nil {
			return nil, errors.Wrap(err, "signature failure over config update")
		}

	}

	return utils.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, channelID, signer, newConfigUpdateEnv, msgVersion, epoch)
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
	return genesis.NewFactoryImpl(bs.channelGroup).Block(genesisconfig.TestChainID)
}

// GenesisBlockForChannel produces a genesis block for a given channel ID
func (bs *Bootstrapper) GenesisBlockForChannel(channelID string) *cb.Block {
	return genesis.NewFactoryImpl(bs.channelGroup).Block(channelID)
}
