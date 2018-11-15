/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"encoding/pem"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// MembershipChanges keeps information about membership
// changes introduced during configuration update
type MembershipChanges struct {
	AddedNodes   []*etcdraft.Consenter
	RemovedNodes []*etcdraft.Consenter
	TotalChanges uint32
}

// UpdateRaftMetadataAndConfChange given the membership changes and RaftMetadata method calculates
// updates to be applied to the raft  cluster configuration in addition updates mapping between
// consenter and its id within metadata
func (mc *MembershipChanges) UpdateRaftMetadataAndConfChange(raftMetadata *etcdraft.RaftMetadata) *raftpb.ConfChange {
	if mc == nil || mc.TotalChanges == 0 {
		return nil
	}

	var confChange *raftpb.ConfChange

	// producing corresponding raft configuration changes
	if len(mc.AddedNodes) > 0 {
		nodeID := raftMetadata.NextConsenterId
		raftMetadata.Consenters[nodeID] = mc.AddedNodes[0]
		raftMetadata.NextConsenterId++
		confChange = &raftpb.ConfChange{
			ID:     raftMetadata.ConfChangeCounts,
			NodeID: nodeID,
			Type:   raftpb.ConfChangeAddNode,
		}
		raftMetadata.ConfChangeCounts++
		return confChange
	}

	if len(mc.RemovedNodes) > 0 {
		for _, c := range mc.RemovedNodes {
			for nodeID, node := range raftMetadata.Consenters {
				if bytes.Equal(c.ClientTlsCert, node.ClientTlsCert) {
					delete(raftMetadata.Consenters, nodeID)
					confChange = &raftpb.ConfChange{
						ID:     raftMetadata.ConfChangeCounts,
						NodeID: nodeID,
						Type:   raftpb.ConfChangeRemoveNode,
					}
					raftMetadata.ConfChangeCounts++
					break
				}
			}
		}
	}

	return confChange
}

// EndpointconfigFromFromSupport extracts TLS CA certificates and endpoints from the ConsenterSupport
func EndpointconfigFromFromSupport(support consensus.ConsenterSupport) (*cluster.EndpointConfig, error) {
	lastConfigBlock, err := lastConfigBlockFromSupport(support)
	if err != nil {
		return nil, err
	}
	endpointconf, err := cluster.EndpointconfigFromConfigBlock(lastConfigBlock)
	if err != nil {
		return nil, err
	}
	return endpointconf, nil
}

func lastConfigBlockFromSupport(support consensus.ConsenterSupport) (*common.Block, error) {
	lastBlockSeq := support.Height() - 1
	lastBlock := support.Block(lastBlockSeq)
	if lastBlock == nil {
		return nil, errors.Errorf("unable to retrieve block %d", lastBlockSeq)
	}
	lastConfigBlock, err := LastConfigBlock(lastBlock, support)
	if err != nil {
		return nil, err
	}
	return lastConfigBlock, nil
}

// LastConfigBlock returns the last config block relative to the given block.
func LastConfigBlock(block *common.Block, support consensus.ConsenterSupport) (*common.Block, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	if support == nil {
		return nil, errors.New("nil support")
	}
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_LAST_CONFIG) {
		return nil, errors.New("no metadata in block")
	}
	lastConfigBlockNum, err := utils.GetLastConfigIndexFromBlock(block)
	if err != nil {
		return nil, err
	}
	lastConfigBlock := support.Block(lastConfigBlockNum)
	if lastConfigBlock == nil {
		return nil, errors.Errorf("unable to retrieve last config block %d", lastConfigBlockNum)
	}
	return lastConfigBlock, nil
}

// newBlockPuller creates a new block puller
func newBlockPuller(support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster) (*cluster.BlockPuller, error) {

	verifyBlockSequence := func(blocks []*common.Block) error {
		return cluster.VerifyBlocks(blocks, support)
	}

	secureConfig, err := baseDialer.ClientConfig()
	if err != nil {
		return nil, err
	}
	secureConfig.AsyncConnect = false
	stdDialer := &cluster.StandardDialer{
		Dialer: cluster.NewTLSPinningDialer(secureConfig),
	}

	// Extract the TLS CA certs and endpoints from the configuration,
	endpointConfig, err := EndpointconfigFromFromSupport(support)
	if err != nil {
		return nil, err
	}
	// and overwrite them.
	secureConfig.SecOpts.ServerRootCAs = endpointConfig.TLSRootCAs
	stdDialer.Dialer.SetConfig(secureConfig)

	der, _ := pem.Decode(secureConfig.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(secureConfig.SecOpts.Certificate))
	}

	return &cluster.BlockPuller{
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller"),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpointConfig.Endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChainID(),
		Dialer:              stdDialer,
	}, nil
}

// RaftPeers maps consenters to slice of raft.Peer
func RaftPeers(consenters map[uint64]*etcdraft.Consenter) []raft.Peer {
	var peers []raft.Peer

	for raftID := range consenters {
		peers = append(peers, raft.Peer{ID: raftID})
	}
	return peers
}

// ConsentersToMap maps consenters into set where key is client TLS certificate
func ConsentersToMap(consenters []*etcdraft.Consenter) map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range consenters {
		set[string(c.ClientTlsCert)] = struct{}{}
	}
	return set
}

// MembershipByCert convert consenters map into set encapsulated by map
// where key is client TLS certificate
func MembershipByCert(consenters map[uint64]*etcdraft.Consenter) map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range consenters {
		set[string(c.ClientTlsCert)] = struct{}{}
	}
	return set
}

// ComputeMembershipChanges computes membership update based on information about new conseters, returns
// two slices: a slice of added consenters and a slice of consenters to be removed
func ComputeMembershipChanges(oldConsenters map[uint64]*etcdraft.Consenter, newConsenters []*etcdraft.Consenter) *MembershipChanges {
	result := &MembershipChanges{
		AddedNodes:   []*etcdraft.Consenter{},
		RemovedNodes: []*etcdraft.Consenter{},
	}

	currentConsentersSet := MembershipByCert(oldConsenters)
	for _, c := range newConsenters {
		if _, exists := currentConsentersSet[string(c.ClientTlsCert)]; !exists {
			result.AddedNodes = append(result.AddedNodes, c)
			result.TotalChanges++
		}
	}

	newConsentersSet := ConsentersToMap(newConsenters)
	for _, c := range oldConsenters {
		if _, exists := newConsentersSet[string(c.ClientTlsCert)]; !exists {
			result.RemovedNodes = append(result.RemovedNodes, c)
			result.TotalChanges++
		}
	}

	return result
}

// MetadataFromConfigValue reads and translates configuration updates from config value into raft metadata
func MetadataFromConfigValue(configValue *common.ConfigValue) (*etcdraft.Metadata, error) {
	consensusTypeValue := &orderer.ConsensusType{}
	if err := proto.Unmarshal(configValue.Value, consensusTypeValue); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensusType config update")
	}

	updatedMetadata := &etcdraft.Metadata{}
	if err := proto.Unmarshal(consensusTypeValue.Metadata, updatedMetadata); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal updated (new) etcdraft metadata configuration")
	}

	return updatedMetadata, nil
}

// MetadataFromConfigUpdate extracts consensus metadata from config update
func MetadataFromConfigUpdate(update *common.ConfigUpdate) (*etcdraft.Metadata, error) {
	if ordererConfigGroup, ok := update.WriteSet.Groups["Orderer"]; ok {
		if val, ok := ordererConfigGroup.Values["ConsensusType"]; ok {
			return MetadataFromConfigValue(val)
		}
	}
	return nil, nil
}

// ConfigEnvelopeFromBlock extracts configuration envelop from the block based on the
// config type, i.e. HeaderType_ORDERER_TRANSACTION or HeaderType_CONFIG
func ConfigEnvelopeFromBlock(block *common.Block) (*common.Envelope, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	envelope, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract envelop from the block")
	}

	channelHeader, err := utils.ChannelHeader(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract channel header")
	}

	switch channelHeader.Type {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		payload, err := utils.UnmarshalPayload(envelope.Payload)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal envelope to extract config payload for orderer transaction")
		}
		configEnvelop, err := utils.UnmarshalEnvelope(payload.Data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal config envelope for orderer type transaction")
		}

		return configEnvelop, nil
	case int32(common.HeaderType_CONFIG):
		return envelope, nil
	default:
		return nil, errors.Errorf("unexpected header type: %v", channelHeader.Type)
	}
}

// ConsensusMetadataFromConfigBlock reads consensus metadata updates from the configuration block
func ConsensusMetadataFromConfigBlock(block *common.Block) (*etcdraft.Metadata, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	if !utils.IsConfigBlock(block) {
		return nil, errors.New("not a config block")
	}

	configEnvelope, err := ConfigEnvelopeFromBlock(block)
	if err != nil {
		return nil, errors.Wrap(err, "cannot read config update")
	}

	payload, err := utils.ExtractPayload(configEnvelope)
	if err != nil {
		return nil, errors.Wrap(err, "failed extract payload from config envelope")
	}
	// get config update
	configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
	if err != nil {
		return nil, errors.Wrap(err, "could not read config update")
	}

	return MetadataFromConfigUpdate(configUpdate)
}

// IsMembershipUpdate checks whenever block is config block and carries
// raft cluster membership updates
func IsMembershipUpdate(block *common.Block, currentMetadata *etcdraft.RaftMetadata) (bool, error) {
	if !utils.IsConfigBlock(block) {
		return false, nil
	}

	metadata, err := ConsensusMetadataFromConfigBlock(block)
	if err != nil {
		return false, errors.Wrap(err, "error reading consensus metadata")
	}

	if metadata != nil {
		changes := ComputeMembershipChanges(currentMetadata.Consenters, metadata.Consenters)

		return changes.TotalChanges > 0, nil
	}

	return false, nil
}

// ConsenterCertificate denotes a TLS certificate of a consenter
type ConsenterCertificate []byte

// IsConsenterOfChannel returns whether the caller is a consenter of a channel
// by inspecting the given configuration block.
// It returns nil if true, else returns an error.
func (conCert ConsenterCertificate) IsConsenterOfChannel(configBlock *common.Block) error {
	if configBlock == nil {
		return errors.New("nil block")
	}
	envelopeConfig, err := utils.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig)
	if err != nil {
		return err
	}
	oc, exists := bundle.OrdererConfig()
	if !exists {
		return errors.New("no orderer config in bundle")
	}
	m := &etcdraft.Metadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), m); err != nil {
		return err
	}

	for _, consenter := range m.Consenters {
		if bytes.Equal(conCert, consenter.ServerTlsCert) || bytes.Equal(conCert, consenter.ClientTlsCert) {
			return nil
		}
	}
	return cluster.ErrNotInChannel
}
