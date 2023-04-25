/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder

import (
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/configtxlator/update"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
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
	// ConsensusTypeEtcdRaft identifies the Raft-based consensus implementation.
	ConsensusTypeEtcdRaft = "etcdraft"
	// ConsensusTypeBFT identifies the BFT-based consensus implementation.
	ConsensusTypeBFT = "BFT"

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
		Value:     protoutil.MarshalOrPanic(value.Value()),
		ModPolicy: modPolicy,
	}
}

func addPolicy(cg *cb.ConfigGroup, policy policies.ConfigPolicy, modPolicy string) {
	cg.Policies[policy.Key()] = &cb.ConfigPolicy{
		Policy:    policy.Value(),
		ModPolicy: modPolicy,
	}
}

func AddOrdererPolicies(cg *cb.ConfigGroup, policyMap map[string]*genesisconfig.Policy, modPolicy string) error {
	switch {
	case policyMap == nil:
		return errors.Errorf("no policies defined")
	case policyMap[BlockValidationPolicyKey] == nil:
		return errors.Errorf("no BlockValidation policy defined")
	}

	return AddPolicies(cg, policyMap, modPolicy)
}

func AddPolicies(cg *cb.ConfigGroup, policyMap map[string]*genesisconfig.Policy, modPolicy string) error {
	switch {
	case policyMap == nil:
		return errors.Errorf("no policies defined")
	case policyMap[channelconfig.AdminsPolicyKey] == nil:
		return errors.Errorf("no Admins policy defined")
	case policyMap[channelconfig.ReadersPolicyKey] == nil:
		return errors.Errorf("no Readers policy defined")
	case policyMap[channelconfig.WritersPolicyKey] == nil:
		return errors.Errorf("no Writers policy defined")
	}

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
					Value: protoutil.MarshalOrPanic(imp),
				},
			}
		case SignaturePolicyType:
			sp, err := policydsl.FromString(policy.Rule)
			if err != nil {
				return errors.Wrapf(err, "invalid signature policy rule '%s'", policy.Rule)
			}
			cg.Policies[policyName] = &cb.ConfigPolicy{
				ModPolicy: modPolicy,
				Policy: &cb.Policy{
					Type:  int32(cb.Policy_SIGNATURE),
					Value: protoutil.MarshalOrPanic(sp),
				},
			}
		default:
			return errors.Errorf("unknown policy type: %s", policy.Type)
		}
	}
	return nil
}

// NewChannelGroup defines the root of the channel configuration.  It defines basic operating principles like the hashing
// algorithm used for the blocks, as well as the location of the ordering service.  It will recursively call into the
// NewOrdererGroup, NewConsortiumsGroup, and NewApplicationGroup depending on whether these sub-elements are set in the
// configuration.  All mod_policy values are set to "Admins" for this group, with the exception of the OrdererAddresses
// value which is set to "/Channel/Orderer/Admins".
func NewChannelGroup(conf *genesisconfig.Profile) (*cb.ConfigGroup, error) {
	channelGroup := protoutil.NewConfigGroup()
	if err := AddPolicies(channelGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
		return nil, errors.Wrapf(err, "error adding policies to channel group")
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
	ordererGroup := protoutil.NewConfigGroup()
	if err := AddOrdererPolicies(ordererGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
		return nil, errors.Wrapf(err, "error adding policies to orderer group")
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
	case ConsensusTypeEtcdRaft:
		if consensusMetadata, err = channelconfig.MarshalEtcdRaftMetadata(conf.EtcdRaft); err != nil {
			return nil, errors.Errorf("cannot marshal metadata for orderer type %s: %s", ConsensusTypeEtcdRaft, err)
		}
	case ConsensusTypeBFT:
		consenterProtos, err := consenterProtosFromConfig(conf.ConsenterMapping)
		if err != nil {
			return nil, errors.Errorf("cannot load consenter config for orderer type %s: %s", ConsensusTypeBFT, err)
		}
		addValue(ordererGroup, channelconfig.OrderersValue(consenterProtos), channelconfig.AdminsPolicyKey)
		if consensusMetadata, err = channelconfig.MarshalBFTOptions(conf.SmartBFT); err != nil {
			return nil, errors.Errorf("consenter options read failed with error %s for orderer type %s", err, ConsensusTypeBFT)
		}
		// Overwrite policy manually by computing it from the consenters
		policies.EncodeBFTBlockVerificationPolicy(consenterProtos, ordererGroup)
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

func consenterProtosFromConfig(consenterMapping []*genesisconfig.Consenter) ([]*cb.Consenter, error) {
	var consenterProtos []*cb.Consenter
	for _, consenter := range consenterMapping {
		c := &cb.Consenter{
			Id:    consenter.ID,
			Host:  consenter.Host,
			Port:  consenter.Port,
			MspId: consenter.MSPID,
		}
		// Expect the user to set the config value for client/server certs or identity to the
		// path where they are persisted locally, then load these files to memory.
		if consenter.ClientTLSCert != "" {
			clientCert, err := ioutil.ReadFile(consenter.ClientTLSCert)
			if err != nil {
				return nil, fmt.Errorf("cannot load client cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
			}
			c.ClientTlsCert = clientCert
		}

		if consenter.ServerTLSCert != "" {
			serverCert, err := ioutil.ReadFile(consenter.ServerTLSCert)
			if err != nil {
				return nil, fmt.Errorf("cannot load server cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
			}
			c.ServerTlsCert = serverCert
		}

		if consenter.Identity != "" {
			identity, err := ioutil.ReadFile(consenter.Identity)
			if err != nil {
				return nil, fmt.Errorf("cannot load identity for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
			}
			c.Identity = identity
		}

		consenterProtos = append(consenterProtos, c)
	}
	return consenterProtos, nil
}

// NewConsortiumsGroup returns an org component of the channel configuration.  It defines the crypto material for the
// organization (its MSP).  It sets the mod_policy of all elements to "Admins".
func NewConsortiumOrgGroup(conf *genesisconfig.Organization) (*cb.ConfigGroup, error) {
	consortiumsOrgGroup := protoutil.NewConfigGroup()
	consortiumsOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey

	if conf.SkipAsForeign {
		return consortiumsOrgGroup, nil
	}

	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org: %s", conf.Name)
	}

	if err := AddPolicies(consortiumsOrgGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
		return nil, errors.Wrapf(err, "error adding policies to consortiums org group '%s'", conf.Name)
	}

	addValue(consortiumsOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	return consortiumsOrgGroup, nil
}

// NewOrdererOrgGroup returns an orderer org component of the channel configuration.  It defines the crypto material for the
// organization (its MSP).  It sets the mod_policy of all elements to "Admins".
func NewOrdererOrgGroup(conf *genesisconfig.Organization) (*cb.ConfigGroup, error) {
	ordererOrgGroup := protoutil.NewConfigGroup()
	ordererOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey

	if conf.SkipAsForeign {
		return ordererOrgGroup, nil
	}

	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org: %s", conf.Name)
	}

	if err := AddPolicies(ordererOrgGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
		return nil, errors.Wrapf(err, "error adding policies to orderer org group '%s'", conf.Name)
	}

	addValue(ordererOrgGroup, channelconfig.MSPValue(mspConfig), channelconfig.AdminsPolicyKey)

	if len(conf.OrdererEndpoints) > 0 {
		addValue(ordererOrgGroup, channelconfig.EndpointsValue(conf.OrdererEndpoints), channelconfig.AdminsPolicyKey)
	}

	return ordererOrgGroup, nil
}

// NewApplicationGroup returns the application component of the channel configuration.  It defines the organizations which are involved
// in application logic like chaincodes, and how these members may interact with the orderer.  It sets the mod_policy of all elements to "Admins".
func NewApplicationGroup(conf *genesisconfig.Application) (*cb.ConfigGroup, error) {
	applicationGroup := protoutil.NewConfigGroup()
	if err := AddPolicies(applicationGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
		return nil, errors.Wrapf(err, "error adding policies to application group")
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
	applicationOrgGroup := protoutil.NewConfigGroup()
	applicationOrgGroup.ModPolicy = channelconfig.AdminsPolicyKey

	if conf.SkipAsForeign {
		return applicationOrgGroup, nil
	}

	mspConfig, err := msp.GetVerifyingMspConfig(conf.MSPDir, conf.ID, conf.MSPType)
	if err != nil {
		return nil, errors.Wrapf(err, "1 - Error loading MSP configuration for org %s", conf.Name)
	}

	if err := AddPolicies(applicationOrgGroup, conf.Policies, channelconfig.AdminsPolicyKey); err != nil {
		return nil, errors.Wrapf(err, "error adding policies to application org group %s", conf.Name)
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

	return applicationOrgGroup, nil
}

// NewConsortiumsGroup returns the consortiums component of the channel configuration.  This element is only defined for the ordering system channel.
// It sets the mod_policy for all elements to "/Channel/Orderer/Admins".
func NewConsortiumsGroup(conf map[string]*genesisconfig.Consortium) (*cb.ConfigGroup, error) {
	consortiumsGroup := protoutil.NewConfigGroup()
	// This policy is not referenced anywhere, it is only used as part of the implicit meta policy rule at the channel level, so this setting
	// effectively degrades control of the ordering system channel to the ordering admins
	addPolicy(consortiumsGroup, policies.SignaturePolicy(channelconfig.AdminsPolicyKey, policydsl.AcceptAllPolicy), ordererAdminsPolicyName)

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
	consortiumGroup := protoutil.NewConfigGroup()

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
		Value: protoutil.MarshalOrPanic(&cb.Consortium{
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
		Policies: map[string]*cb.ConfigPolicy{
			channelconfig.AdminsPolicyKey: {},
		},
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
// Deprecated
func MakeChannelCreationTransaction(
	channelID string,
	signer identity.SignerSerializer,
	conf *genesisconfig.Profile,
) (*cb.Envelope, error) {
	template, err := DefaultConfigTemplate(conf)
	if err != nil {
		return nil, errors.WithMessage(err, "could not generate default config template")
	}
	return MakeChannelCreationTransactionFromTemplate(channelID, signer, conf, template)
}

// MakeChannelCreationTransactionWithSystemChannelContext is a utility function for creating channel creation txes.
// It requires a configuration representing the orderer system channel to allow more sophisticated channel creation
// transactions modifying pieces of the configuration like the orderer set.
// Deprecated
func MakeChannelCreationTransactionWithSystemChannelContext(
	channelID string,
	signer identity.SignerSerializer,
	conf,
	systemChannelConf *genesisconfig.Profile,
) (*cb.Envelope, error) {
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
func MakeChannelCreationTransactionFromTemplate(
	channelID string,
	signer identity.SignerSerializer,
	conf *genesisconfig.Profile,
	template *cb.ConfigGroup,
) (*cb.Envelope, error) {
	newChannelConfigUpdate, err := NewChannelCreateConfigUpdate(channelID, conf, template)
	if err != nil {
		return nil, errors.Wrap(err, "config update generation failure")
	}

	newConfigUpdateEnv := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: protoutil.MarshalOrPanic(newChannelConfigUpdate),
	}

	if signer != nil {
		sigHeader, err := protoutil.NewSignatureHeader(signer)
		if err != nil {
			return nil, errors.Wrap(err, "creating signature header failed")
		}

		newConfigUpdateEnv.Signatures = []*cb.ConfigSignature{{
			SignatureHeader: protoutil.MarshalOrPanic(sigHeader),
		}}

		newConfigUpdateEnv.Signatures[0].Signature, err = signer.Sign(util.ConcatenateBytes(newConfigUpdateEnv.Signatures[0].SignatureHeader, newConfigUpdateEnv.ConfigUpdate))
		if err != nil {
			return nil, errors.Wrap(err, "signature failure over config update")
		}

	}

	return protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, channelID, signer, newConfigUpdateEnv, msgVersion, epoch)
}

// HasSkippedForeignOrgs is used to detect whether a configuration includes
// org definitions which should not be parsed because this tool is being
// run in a context where the user does not have access to that org's info
func HasSkippedForeignOrgs(conf *genesisconfig.Profile) error {
	var organizations []*genesisconfig.Organization

	if conf.Orderer != nil {
		organizations = append(organizations, conf.Orderer.Organizations...)
	}

	if conf.Application != nil {
		organizations = append(organizations, conf.Application.Organizations...)
	}

	for _, consortium := range conf.Consortiums {
		organizations = append(organizations, consortium.Organizations...)
	}

	for _, org := range organizations {
		if org.SkipAsForeign {
			return errors.Errorf("organization '%s' is marked to be skipped as foreign", org.Name)
		}
	}

	return nil
}

// Bootstrapper is a wrapper around NewChannelConfigGroup which can produce genesis blocks
type Bootstrapper struct {
	channelGroup *cb.ConfigGroup
}

// NewBootstrapper creates a bootstrapper but returns an error instead of panic-ing
func NewBootstrapper(config *genesisconfig.Profile) (*Bootstrapper, error) {
	if err := HasSkippedForeignOrgs(config); err != nil {
		return nil, errors.WithMessage(err, "all org definitions must be local during bootstrapping")
	}

	channelGroup, err := NewChannelGroup(config)
	if err != nil {
		return nil, errors.WithMessage(err, "could not create channel group")
	}

	return &Bootstrapper{
		channelGroup: channelGroup,
	}, nil
}

// New creates a new Bootstrapper for generating genesis blocks
func New(config *genesisconfig.Profile) *Bootstrapper {
	bs, err := NewBootstrapper(config)
	if err != nil {
		logger.Panicf("Error creating bootsrapper: %s", err)
	}
	return bs
}

// GenesisBlock produces a genesis block for the default test channel id
func (bs *Bootstrapper) GenesisBlock() *cb.Block {
	// TODO(mjs): remove
	return genesis.NewFactoryImpl(bs.channelGroup).Block("testchannelid")
}

// GenesisBlockForChannel produces a genesis block for a given channel ID
func (bs *Bootstrapper) GenesisBlockForChannel(channelID string) *cb.Block {
	return genesis.NewFactoryImpl(bs.channelGroup).Block(channelID)
}
