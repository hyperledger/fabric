/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

const (
	// These values are fixed for the genesis block.
	msgVersion = 0
	epoch      = 0

	// ConsortiumKey is the key for the ConfigValue of a
	// Consortium.
	ConsortiumKey = "Consortium"

	// HashingAlgorithmKey is the key for the ConfigValue of a
	// HashingAlgorithm.
	HashingAlgorithmKey = "HashingAlgorithm"

	// BlockDataHashingStructureKey is the key for the ConfigValue
	// of a BlockDataHashingStructure.
	BlockDataHashingStructureKey = "BlockDataHashingStructure"

	// CapabilitiesKey is the key for the ConfigValue, capabilities.
	// CapabiltiesKey can be used at the channel, application, and orderer levels.
	CapabilitiesKey = "Capabilities"

	// EndpointsKey is the key for the ConfigValue, Endpoints in
	// a OrdererOrgGroup.
	EndpointsKey = "Endpoints"

	// MSPKey is the key for the ConfigValue, MSP.
	MSPKey = "MSP"

	// AdminsPolicyKey is the key used for the admin policy.
	AdminsPolicyKey = "Admins"

	// ReadersPolicyKey is the key used for the read policy.
	ReadersPolicyKey = "Readers"

	// WritersPolicyKey is the key used for the write policy.
	WritersPolicyKey = "Writers"

	// EndorsementPolicyKey is the key used for the endorsement policy.
	EndorsementPolicyKey = "Endorsement"

	// LifecycleEndorsementPolicyKey is the key used for the lifecycle endorsement
	// policy.
	LifecycleEndorsementPolicyKey = "LifecycleEndorsement"

	// BlockValidationPolicyKey is the key used for the block validation policy in
	// the OrdererOrgGroup.
	BlockValidationPolicyKey = "BlockValidation"

	// ChannelCreationPolicyKey is the key used in the consortium config to denote
	// the policy to be used in evaluating whether a channel creation request
	// is authorized.
	ChannelCreationPolicyKey = "ChannelCreationPolicy"

	// ChannelGroupKey is the group name for the channel config.
	ChannelGroupKey = "Channel"

	// ConsortiumsGroupKey is the group name for the consortiums config.
	ConsortiumsGroupKey = "Consortiums"

	// OrdererGroupKey is the group name for the orderer config.
	OrdererGroupKey = "Orderer"

	// ApplicationGroupKey is the group name for the Application config.
	ApplicationGroupKey = "Application"

	// ACLsKey is the name of the ACLs config.
	ACLsKey = "ACLs"

	// AnchorPeersKey is the key name for the AnchorPeers ConfigValue.
	AnchorPeersKey = "AnchorPeers"

	// ImplicitMetaPolicyType is the 'Type' string for implicit meta policies.
	ImplicitMetaPolicyType = "ImplicitMeta"

	// SignaturePolicyType is the 'Type' string for signature policies.
	SignaturePolicyType = "Signature"

	ordererAdminsPolicyName = "/Channel/Orderer/Admins"

	// OrdererAddressesKey is the key for the ConfigValue of OrdererAddresses.
	OrdererAddressesKey = "OrdererAddresses"
)
