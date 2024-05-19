/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
)

// Org stores the common organizational config
type Org interface {
	// Name returns the name this org is referred to in config
	Name() string

	// MSPID returns the MSP ID associated with this org
	MSPID() string

	// MSP returns the MSP implementation for this org.
	MSP() msp.MSP
}

// ApplicationOrg stores the per org application config
type ApplicationOrg interface {
	Org

	// AnchorPeers returns the list of gossip anchor peers
	AnchorPeers() []*pb.AnchorPeer
}

// OrdererOrg stores the per org orderer config.
type OrdererOrg interface {
	Org

	// Endpoints returns the endpoints of orderer nodes.
	Endpoints() []string
}

// Application stores the common shared application config
type Application interface {
	// Organizations returns a map of org ID to ApplicationOrg
	Organizations() map[string]ApplicationOrg

	// APIPolicyMapper returns a PolicyMapper that maps API names to policies
	APIPolicyMapper() PolicyMapper

	// Capabilities defines the capabilities for the application portion of a channel
	Capabilities() ApplicationCapabilities
}

// Channel gives read only access to the channel configuration
type Channel interface {
	// HashingAlgorithm returns the default algorithm to be used when hashing
	// such as computing block hashes, and CreationPolicy digests
	HashingAlgorithm() func(input []byte) []byte

	// BlockDataHashingStructureWidth returns the width to use when constructing the
	// Merkle tree to compute the BlockData hash
	BlockDataHashingStructureWidth() uint32

	// OrdererAddresses returns the list of valid orderer addresses to connect to invoke Broadcast/Deliver
	OrdererAddresses() []string

	// Capabilities defines the capabilities for a channel
	Capabilities() ChannelCapabilities
}

// Consortiums represents the set of consortiums serviced by an ordering service
type Consortiums interface {
	// Consortiums returns the set of consortiums
	Consortiums() map[string]Consortium
}

// Consortium represents a group of orgs which may create channels together
type Consortium interface {
	// ChannelCreationPolicy returns the policy to check when instantiating a channel for this consortium
	ChannelCreationPolicy() *cb.Policy

	// Organizations returns the organizations for this consortium
	Organizations() map[string]Org
}

// Orderer stores the common shared orderer config
type Orderer interface {
	// ConsensusType returns the configured consensus type
	ConsensusType() string

	// ConsensusMetadata returns the metadata associated with the consensus type.
	ConsensusMetadata() []byte

	// ConsensusState returns the consensus-type state.
	ConsensusState() ab.ConsensusType_State

	// BatchSize returns the maximum number of messages to include in a block
	BatchSize() *ab.BatchSize

	// BatchTimeout returns the amount of time to wait before creating a batch
	BatchTimeout() time.Duration

	// MaxChannelsCount returns the maximum count of channels to allow for an ordering network
	MaxChannelsCount() uint64

	Consenters() []*cb.Consenter

	// Organizations returns the organizations for the ordering service
	Organizations() map[string]OrdererOrg

	// Capabilities defines the capabilities for the orderer portion of a channel
	Capabilities() OrdererCapabilities
}

// ChannelCapabilities defines the capabilities for a channel
type ChannelCapabilities interface {
	// Supported returns an error if there are unknown capabilities in this channel which are required
	Supported() error

	// MSPVersion specifies the version of the MSP this channel must understand, including the MSP types
	// and MSP principal types.
	MSPVersion() msp.MSPVersion

	// ConsensusTypeMigration return true if consensus-type migration is permitted in both orderer and peer.
	ConsensusTypeMigration() bool

	// OrgSpecificOrdererEndpoints return true if the channel config processing allows orderer orgs to specify their own endpoints
	OrgSpecificOrdererEndpoints() bool

	// ConsensusTypeBFT returns true if the channel must support BFT consensus
	ConsensusTypeBFT() bool
}

// ApplicationCapabilities defines the capabilities for the application portion of a channel
type ApplicationCapabilities interface {
	// Supported returns an error if there are unknown capabilities in this channel which are required
	Supported() error

	// ForbidDuplicateTXIdInBlock specifies whether two transactions with the same TXId are permitted
	// in the same block or whether we mark the second one as TxValidationCode_DUPLICATE_TXID
	ForbidDuplicateTXIdInBlock() bool

	// ACLs returns true is ACLs may be specified in the Application portion of the config tree
	ACLs() bool

	// PrivateChannelData returns true if support for private channel data (a.k.a. collections) is enabled.
	// In v1.1, the private channel data is experimental and has to be enabled explicitly.
	// In v1.2, the private channel data is enabled by default.
	PrivateChannelData() bool

	// CollectionUpgrade returns true if this channel is configured to allow updates to
	// existing collection or add new collections through chaincode upgrade (as introduced in v1.2)
	CollectionUpgrade() bool

	// V1_1Validation returns true is this channel is configured to perform stricter validation
	// of transactions (as introduced in v1.1).
	V1_1Validation() bool

	// V1_2Validation returns true is this channel is configured to perform stricter validation
	// of transactions (as introduced in v1.2).
	V1_2Validation() bool

	// V1_3Validation returns true if this channel supports transaction validation
	// as introduced in v1.3. This includes:
	//  - policies expressible at a ledger key granularity, as described in FAB-8812
	//  - new chaincode lifecycle, as described in FAB-11237
	V1_3Validation() bool

	// StorePvtDataOfInvalidTx returns true if the peer needs to store the pvtData of
	// invalid transactions (as introduced in v142).
	StorePvtDataOfInvalidTx() bool

	// V2_0Validation returns true if this channel supports transaction validation
	// as introduced in v2.0. This includes:
	//  - new chaincode lifecycle
	//  - implicit per-org collections
	V2_0Validation() bool

	// LifecycleV20 indicates whether the peer should use the deprecated and problematic
	// v1.x lifecycle, or whether it should use the newer per channel approve/commit definitions
	// process introduced in v2.0.  Note, this should only be used on the endorsing side
	// of peer processing, so that we may safely remove all checks against it in v2.1.
	LifecycleV20() bool

	// MetadataLifecycle always returns false
	MetadataLifecycle() bool

	// KeyLevelEndorsement returns true if this channel supports endorsement
	// policies expressible at a ledger key granularity, as described in FAB-8812
	KeyLevelEndorsement() bool

	// PurgePvtData returns true if this channel supports purging of private
	// data entries
	PurgePvtData() bool
}

// OrdererCapabilities defines the capabilities for the orderer portion of a channel
type OrdererCapabilities interface {
	// PredictableChannelTemplate specifies whether the v1.0 undesirable behavior of setting the /Channel
	// group's mod_policy to "" and copy versions from the orderer system channel config should be fixed or not.
	PredictableChannelTemplate() bool

	// Resubmission specifies whether the v1.0 non-deterministic commitment of tx should be fixed by re-submitting
	// the re-validated tx.
	Resubmission() bool

	// Supported returns an error if there are unknown capabilities in this channel which are required
	Supported() error

	// ExpirationCheck specifies whether the orderer checks for identity expiration checks
	// when validating messages
	ExpirationCheck() bool

	// ConsensusTypeMigration checks whether the orderer permits a consensus-type migration.
	ConsensusTypeMigration() bool

	// UseChannelCreationPolicyAsAdmins checks whether the orderer should use more sophisticated
	// channel creation logic using channel creation policy as the Admins policy if
	// the creation transaction appears to support it.
	UseChannelCreationPolicyAsAdmins() bool
}

// PolicyMapper is an interface for
type PolicyMapper interface {
	// PolicyRefForAPI takes the name of an API, and returns the policy name
	// or the empty string if the API is not found
	PolicyRefForAPI(apiName string) string
}

// Resources is the common set of config resources for all channels
// Depending on whether chain is used at the orderer or at the peer, other
// config resources may be available
type Resources interface {
	// ConfigtxValidator returns the configtx.Validator for the channel
	ConfigtxValidator() configtx.Validator

	// PolicyManager returns the policies.Manager for the channel
	PolicyManager() policies.Manager

	// ChannelConfig returns the config.Channel for the chain
	ChannelConfig() Channel

	// OrdererConfig returns the config.Orderer for the channel
	// and whether the Orderer config exists
	OrdererConfig() (Orderer, bool)

	// ConsortiumsConfig returns the config.Consortiums for the channel
	// and whether the consortiums' config exists
	ConsortiumsConfig() (Consortiums, bool)

	// ApplicationConfig returns the configtxapplication.SharedConfig for the channel
	// and whether the Application config exists
	ApplicationConfig() (Application, bool)

	// MSPManager returns the msp.MSPManager for the chain
	MSPManager() msp.MSPManager

	// ValidateNew should return an error if a new set of configuration resources is incompatible with the current one
	ValidateNew(resources Resources) error
}
