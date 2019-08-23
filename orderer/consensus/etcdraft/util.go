/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
)

// RaftPeers maps consenters to slice of raft.Peer
func RaftPeers(consenterIDs []uint64) []raft.Peer {
	var peers []raft.Peer

	for _, raftID := range consenterIDs {
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

// MetadataHasDuplication returns an error if the metadata has duplication of consenters.
// A duplication is defined by having a server or a client TLS certificate that is found
// in two different consenters, regardless of the type of certificate (client/server).
func MetadataHasDuplication(md *etcdraft.ConfigMetadata) error {
	if md == nil {
		return errors.New("nil metadata")
	}

	for _, consenter := range md.Consenters {
		if consenter == nil {
			return errors.New("nil consenter in metadata")
		}
	}

	seen := make(map[string]struct{})
	for _, consenter := range md.Consenters {
		serverKey := string(consenter.ServerTlsCert)
		clientKey := string(consenter.ClientTlsCert)
		_, duplicateServerCert := seen[serverKey]
		_, duplicateClientCert := seen[clientKey]
		if duplicateServerCert || duplicateClientCert {
			return errors.Errorf("duplicate consenter: server cert: %s, client cert: %s", serverKey, clientKey)
		}

		seen[serverKey] = struct{}{}
		seen[clientKey] = struct{}{}
	}
	return nil
}

// MetadataFromConfigValue reads and translates configuration updates from config value into raft metadata
func MetadataFromConfigValue(configValue *common.ConfigValue) (*etcdraft.ConfigMetadata, error) {
	consensusTypeValue := &orderer.ConsensusType{}
	if err := proto.Unmarshal(configValue.Value, consensusTypeValue); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensusType config update")
	}

	updatedMetadata := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(consensusTypeValue.Metadata, updatedMetadata); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal updated (new) etcdraft metadata configuration")
	}

	return updatedMetadata, nil
}

// MetadataFromConfigUpdate extracts consensus metadata from config update
func MetadataFromConfigUpdate(update *common.ConfigUpdate) (*etcdraft.ConfigMetadata, error) {
	var baseVersion uint64
	if update.ReadSet != nil && update.ReadSet.Groups != nil {
		if ordererConfigGroup, ok := update.ReadSet.Groups["Orderer"]; ok {
			if val, ok := ordererConfigGroup.Values["ConsensusType"]; ok {
				baseVersion = val.Version
			}
		}
	}

	if update.WriteSet != nil && update.WriteSet.Groups != nil {
		if ordererConfigGroup, ok := update.WriteSet.Groups["Orderer"]; ok {
			if val, ok := ordererConfigGroup.Values["ConsensusType"]; ok {
				if baseVersion == val.Version {
					// Only if the version in the write set differs from the read-set
					// should we consider this to be an update to the consensus type
					return nil, nil
				}
				return MetadataFromConfigValue(val)
			}
		}
	}
	return nil, nil
}

// ConfigChannelHeader expects a config block and returns the header type
// of the config envelope wrapped in it, e.g. HeaderType_ORDERER_TRANSACTION
func ConfigChannelHeader(block *common.Block) (hdr *common.ChannelHeader, err error) {
	envelope, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract envelope from the block")
	}

	channelHeader, err := protoutil.ChannelHeader(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract channel header")
	}

	return channelHeader, nil
}

// ConfigEnvelopeFromBlock extracts configuration envelope from the block based on the
// config type, i.e. HeaderType_ORDERER_TRANSACTION or HeaderType_CONFIG
func ConfigEnvelopeFromBlock(block *common.Block) (*common.Envelope, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	envelope, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract envelope from the block")
	}

	channelHeader, err := protoutil.ChannelHeader(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract channel header")
	}

	switch channelHeader.Type {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		payload, err := protoutil.UnmarshalPayload(envelope.Payload)
		if err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal envelope to extract config payload for orderer transaction")
		}
		configEnvelop, err := protoutil.UnmarshalEnvelope(payload.Data)
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
func ConsensusMetadataFromConfigBlock(block *common.Block) (*etcdraft.ConfigMetadata, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	if !protoutil.IsConfigBlock(block) {
		return nil, errors.New("not a config block")
	}

	configEnvelope, err := ConfigEnvelopeFromBlock(block)
	if err != nil {
		return nil, errors.Wrap(err, "cannot read config update")
	}

	payload, err := protoutil.UnmarshalPayload(configEnvelope.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract payload from config envelope")
	}
	// get config update
	configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
	if err != nil {
		return nil, errors.Wrap(err, "could not read config update")
	}

	return MetadataFromConfigUpdate(configUpdate)
}

// CheckConfigMetadata validates Raft config metadata
func CheckConfigMetadata(metadata *etcdraft.ConfigMetadata) error {
	if metadata == nil {
		// defensive check. this should not happen as CheckConfigMetadata
		// should always be called with non-nil config metadata
		return errors.Errorf("nil Raft config metadata")
	}

	if metadata.Options.HeartbeatTick == 0 ||
		metadata.Options.ElectionTick == 0 ||
		metadata.Options.MaxInflightBlocks == 0 {
		// if SnapshotIntervalSize is zero, DefaultSnapshotIntervalSize is used
		return errors.Errorf("none of HeartbeatTick (%d), ElectionTick (%d) and MaxInflightBlocks (%d) can be zero",
			metadata.Options.HeartbeatTick, metadata.Options.ElectionTick, metadata.Options.MaxInflightBlocks)
	}

	// check Raft options
	if metadata.Options.ElectionTick <= metadata.Options.HeartbeatTick {
		return errors.Errorf("ElectionTick (%d) must be greater than HeartbeatTick (%d)",
			metadata.Options.HeartbeatTick, metadata.Options.HeartbeatTick)
	}

	if d, err := time.ParseDuration(metadata.Options.TickInterval); err != nil {
		return errors.Errorf("failed to parse TickInterval (%s) to time duration: %s", metadata.Options.TickInterval, err)
	} else if d == 0 {
		return errors.Errorf("TickInterval cannot be zero")
	}

	if len(metadata.Consenters) == 0 {
		return errors.Errorf("empty consenter set")
	}

	// sanity check of certificates
	for _, consenter := range metadata.Consenters {
		if consenter == nil {
			return errors.Errorf("metadata has nil consenter")
		}
		if err := validateCert(consenter.ServerTlsCert, "server"); err != nil {
			return err
		}
		if err := validateCert(consenter.ClientTlsCert, "client"); err != nil {
			return err
		}
	}

	if err := MetadataHasDuplication(metadata); err != nil {
		return err
	}

	return nil
}

func validateCert(pemData []byte, certRole string) error {
	bl, _ := pem.Decode(pemData)

	if bl == nil {
		return errors.Errorf("%s TLS certificate is not PEM encoded: %s", certRole, string(pemData))
	}

	if _, err := x509.ParseCertificate(bl.Bytes); err != nil {
		return errors.Errorf("%s TLS certificate has invalid ASN1 structure, %v: %s", certRole, err, string(pemData))
	}
	return nil
}

// ConsenterCertificate denotes a TLS certificate of a consenter
type ConsenterCertificate struct {
	ConsenterCertificate []byte
	CryptoProvider       bccsp.BCCSP
}

// type ConsenterCertificate []byte

// IsConsenterOfChannel returns whether the caller is a consenter of a channel
// by inspecting the given configuration block.
// It returns nil if true, else returns an error.
func (conCert ConsenterCertificate) IsConsenterOfChannel(configBlock *common.Block) error {
	if configBlock == nil {
		return errors.New("nil block")
	}
	envelopeConfig, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig, conCert.CryptoProvider)
	if err != nil {
		return err
	}
	oc, exists := bundle.OrdererConfig()
	if !exists {
		return errors.New("no orderer config in bundle")
	}
	m := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), m); err != nil {
		return err
	}

	for _, consenter := range m.Consenters {
		if bytes.Equal(conCert.ConsenterCertificate, consenter.ServerTlsCert) || bytes.Equal(conCert.ConsenterCertificate, consenter.ClientTlsCert) {
			return nil
		}
	}
	return cluster.ErrNotInChannel
}

// NodeExists returns trues if node id exists in the slice
// and false otherwise
func NodeExists(id uint64, nodes []uint64) bool {
	for _, nodeID := range nodes {
		if nodeID == id {
			return true
		}
	}
	return false
}

// ConfChange computes Raft configuration changes based on current Raft
// configuration state and consenters IDs stored in RaftMetadata.
func ConfChange(blockMetadata *etcdraft.BlockMetadata, confState *raftpb.ConfState) *raftpb.ConfChange {
	raftConfChange := &raftpb.ConfChange{}

	// need to compute conf changes to propose
	if len(confState.Nodes) < len(blockMetadata.ConsenterIds) {
		// adding new node
		raftConfChange.Type = raftpb.ConfChangeAddNode
		for _, consenterID := range blockMetadata.ConsenterIds {
			if NodeExists(consenterID, confState.Nodes) {
				continue
			}
			raftConfChange.NodeID = consenterID
		}
	} else {
		// removing node
		raftConfChange.Type = raftpb.ConfChangeRemoveNode
		for _, nodeID := range confState.Nodes {
			if NodeExists(nodeID, blockMetadata.ConsenterIds) {
				continue
			}
			raftConfChange.NodeID = nodeID
		}
	}

	return raftConfChange
}

// CreateConsentersMap creates a map of Raft Node IDs to Consenter given the block metadata and the config metadata.
func CreateConsentersMap(blockMetadata *etcdraft.BlockMetadata, configMetadata *etcdraft.ConfigMetadata) map[uint64]*etcdraft.Consenter {
	consenters := map[uint64]*etcdraft.Consenter{}
	for i, consenter := range configMetadata.Consenters {
		consenters[blockMetadata.ConsenterIds[i]] = consenter
	}
	return consenters
}
