/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"
	"math"
	"os"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mspprotos "github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	// ReadersPolicyKey is the key used for the read policy
	ReadersPolicyKey = "Readers"

	// WritersPolicyKey is the key used for the read policy
	WritersPolicyKey = "Writers"

	// AdminsPolicyKey is the key used for the read policy
	AdminsPolicyKey = "Admins"

	defaultHashingAlgorithm = bccsp.SHA256

	defaultBlockDataHashingStructureWidth = math.MaxUint32
)

// ConfigValue defines a common representation for different *cb.ConfigValue values.
type ConfigValue interface {
	// Key is the key this value should be stored in the *cb.ConfigGroup.Values map.
	Key() string

	// Value is the message which should be marshaled to opaque bytes for the *cb.ConfigValue.value.
	Value() proto.Message
}

// StandardConfigValue implements the ConfigValue interface.
type StandardConfigValue struct {
	key   string
	value proto.Message
}

// Key is the key this value should be stored in the *cb.ConfigGroup.Values map.
func (scv *StandardConfigValue) Key() string {
	return scv.key
}

// Value is the message which should be marshaled to opaque bytes for the *cb.ConfigValue.value.
func (scv *StandardConfigValue) Value() proto.Message {
	return scv.value
}

// ConsortiumValue returns the config definition for the consortium name.
// It is a value for the channel group.
func ConsortiumValue(name string) *StandardConfigValue {
	return &StandardConfigValue{
		key: ConsortiumKey,
		value: &cb.Consortium{
			Name: name,
		},
	}
}

// HashingAlgorithmValue returns the default hashing algorithm.
// It is a value for the /Channel group.
func HashingAlgorithmValue() *StandardConfigValue {
	return &StandardConfigValue{
		key: HashingAlgorithmKey,
		value: &cb.HashingAlgorithm{
			Name: defaultHashingAlgorithm,
		},
	}
}

// BlockDataHashingStructureValue returns the only currently valid block data hashing structure.
// It is a value for the /Channel group.
func BlockDataHashingStructureValue() *StandardConfigValue {
	return &StandardConfigValue{
		key: BlockDataHashingStructureKey,
		value: &cb.BlockDataHashingStructure{
			Width: defaultBlockDataHashingStructureWidth,
		},
	}
}

// OrdererAddressesValue returns the a config definition for the orderer addresses.
// It is a value for the /Channel group.
func OrdererAddressesValue(addresses []string) *StandardConfigValue {
	return &StandardConfigValue{
		key: OrdererAddressesKey,
		value: &cb.OrdererAddresses{
			Addresses: addresses,
		},
	}
}

// ConsensusTypeValue returns the config definition for the orderer consensus type.
// It is a value for the /Channel/Orderer group.
func ConsensusTypeValue(consensusType string, consensusMetadata []byte) *StandardConfigValue {
	return &StandardConfigValue{
		key: ConsensusTypeKey,
		value: &ab.ConsensusType{
			Type:     consensusType,
			Metadata: consensusMetadata,
		},
	}
}

// BatchSizeValue returns the config definition for the orderer batch size.
// It is a value for the /Channel/Orderer group.
func BatchSizeValue(maxMessages, absoluteMaxBytes, preferredMaxBytes uint32) *StandardConfigValue {
	return &StandardConfigValue{
		key: BatchSizeKey,
		value: &ab.BatchSize{
			MaxMessageCount:   maxMessages,
			AbsoluteMaxBytes:  absoluteMaxBytes,
			PreferredMaxBytes: preferredMaxBytes,
		},
	}
}

// BatchTimeoutValue returns the config definition for the orderer batch timeout.
// It is a value for the /Channel/Orderer group.
func BatchTimeoutValue(timeout string) *StandardConfigValue {
	return &StandardConfigValue{
		key: BatchTimeoutKey,
		value: &ab.BatchTimeout{
			Timeout: timeout,
		},
	}
}

// ChannelRestrictionsValue returns the config definition for the orderer channel restrictions.
// It is a value for the /Channel/Orderer group.
func ChannelRestrictionsValue(maxChannelCount uint64) *StandardConfigValue {
	return &StandardConfigValue{
		key: ChannelRestrictionsKey,
		value: &ab.ChannelRestrictions{
			MaxCount: maxChannelCount,
		},
	}
}

// MSPValue returns the config definition for an MSP.
// It is a value for the /Channel/Orderer/*, /Channel/Application/*, and /Channel/Consortiums/*/*/* groups.
func MSPValue(mspDef *mspprotos.MSPConfig) *StandardConfigValue {
	return &StandardConfigValue{
		key:   MSPKey,
		value: mspDef,
	}
}

// CapabilitiesValue returns the config definition for a set of capabilities.
// It is a value for the /Channel/Orderer, Channel/Application/, and /Channel groups.
func CapabilitiesValue(capabilities map[string]bool) *StandardConfigValue {
	c := &cb.Capabilities{
		Capabilities: make(map[string]*cb.Capability),
	}

	for capability, required := range capabilities {
		if !required {
			continue
		}
		c.Capabilities[capability] = &cb.Capability{}
	}

	return &StandardConfigValue{
		key:   CapabilitiesKey,
		value: c,
	}
}

func OrderersValue(consenters []*cb.Consenter) *StandardConfigValue {
	o := &cb.Orderers{
		ConsenterMapping: consenters,
	}
	return &StandardConfigValue{
		key:   OrderersKey,
		value: o,
	}
}

// EndpointsValue returns the config definition for the orderer addresses at an org scoped level.
// It is a value for the /Channel/Orderer/<OrgName> group.
func EndpointsValue(addresses []string) *StandardConfigValue {
	return &StandardConfigValue{
		key: EndpointsKey,
		value: &cb.OrdererAddresses{
			Addresses: addresses,
		},
	}
}

// AnchorPeersValue returns the config definition for an org's anchor peers.
// It is a value for the /Channel/Application/*.
func AnchorPeersValue(anchorPeers []*pb.AnchorPeer) *StandardConfigValue {
	return &StandardConfigValue{
		key:   AnchorPeersKey,
		value: &pb.AnchorPeers{AnchorPeers: anchorPeers},
	}
}

// ChannelCreationPolicyValue returns the config definition for a consortium's channel creation policy
// It is a value for the /Channel/Consortiums/*/*.
func ChannelCreationPolicyValue(policy *cb.Policy) *StandardConfigValue {
	return &StandardConfigValue{
		key:   ChannelCreationPolicyKey,
		value: policy,
	}
}

// ACLValues returns the config definition for an applications resources based ACL definitions.
// It is a value for the /Channel/Application/.
func ACLValues(acls map[string]string) *StandardConfigValue {
	a := &pb.ACLs{
		Acls: make(map[string]*pb.APIResource),
	}

	for apiResource, policyRef := range acls {
		a.Acls[apiResource] = &pb.APIResource{PolicyRef: policyRef}
	}

	return &StandardConfigValue{
		key:   ACLsKey,
		value: a,
	}
}

// ValidateCapabilities validates whether the peer can meet the capabilities requirement in the given config block
func ValidateCapabilities(block *cb.Block, bccsp bccsp.BCCSP) error {
	cc, err := extractChannelConfig(block, bccsp)
	if err != nil {
		return err
	}
	// Check the channel top-level capabilities
	if err := cc.Capabilities().Supported(); err != nil {
		return err
	}

	// Check the application capabilities
	return cc.ApplicationConfig().Capabilities().Supported()
}

// ExtractMSPIDsForApplicationOrgs extracts MSPIDs for application organizations
func ExtractMSPIDsForApplicationOrgs(block *cb.Block, bccsp bccsp.BCCSP) ([]string, error) {
	cc, err := extractChannelConfig(block, bccsp)
	if err != nil {
		return nil, err
	}

	if cc.ApplicationConfig() == nil {
		return nil, errors.Errorf("could not get application config for the channel")
	}
	orgs := cc.ApplicationConfig().Organizations()
	mspids := make([]string, 0, len(orgs))
	for _, org := range orgs {
		mspids = append(mspids, org.MSPID())
	}
	return mspids, nil
}

func extractChannelConfig(block *cb.Block, bccsp bccsp.BCCSP) (*ChannelConfig, error) {
	envelopeConfig, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.WithMessage(err, "malformed configuration block")
	}

	configEnv := &cb.ConfigEnvelope{}
	_, err = protoutil.UnmarshalEnvelopeOfType(envelopeConfig, cb.HeaderType_CONFIG, configEnv)
	if err != nil {
		return nil, errors.WithMessage(err, "malformed configuration envelope")
	}

	if configEnv.Config == nil {
		return nil, errors.New("no config found in envelope")
	}

	if configEnv.Config.ChannelGroup == nil {
		return nil, errors.New("no channel configuration found in the config block")
	}

	if configEnv.Config.ChannelGroup.Groups == nil {
		return nil, errors.New("no channel configuration groups are available")
	}

	_, exists := configEnv.Config.ChannelGroup.Groups[ApplicationGroupKey]
	if !exists {
		return nil, errors.Errorf("invalid configuration block, missing %s configuration group", ApplicationGroupKey)
	}

	cc, err := NewChannelConfig(configEnv.Config.ChannelGroup, bccsp)
	if err != nil {
		return nil, errors.WithMessage(err, "no valid channel configuration found")
	}
	return cc, nil
}

// MarshalEtcdRaftMetadata serializes etcd RAFT metadata.
func MarshalEtcdRaftMetadata(md *etcdraft.ConfigMetadata) ([]byte, error) {
	copyMd := proto.Clone(md).(*etcdraft.ConfigMetadata)
	for _, c := range copyMd.Consenters {
		// Expect the user to set the config value for client/server certs to the
		// path where they are persisted locally, then load these files to memory.
		clientCert, err := os.ReadFile(string(c.GetClientTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load client cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.ClientTlsCert = clientCert

		serverCert, err := os.ReadFile(string(c.GetServerTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load server cert for consenter %s:%d: %s", c.GetHost(), c.GetPort(), err)
		}
		c.ServerTlsCert = serverCert
	}
	return proto.Marshal(copyMd)
}

// MarshalBFTOptions serializes smartbft options.
func MarshalBFTOptions(op *smartbft.Options) ([]byte, error) {
	if copyMd, ok := proto.Clone(op).(*smartbft.Options); ok {
		return proto.Marshal(copyMd)
	} else {
		return nil, errors.New("consenter options type mismatch")
	}
}
