/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
)

// MembershipChanges keeps information about membership
// changes introduced during configuration update
type MembershipChanges struct {
	NewBlockMetadata *etcdraft.BlockMetadata
	NewConsenters    map[uint64]*etcdraft.Consenter
	AddedNodes       []*etcdraft.Consenter
	RemovedNodes     []*etcdraft.Consenter
	ConfChange       *raftpb.ConfChange
	RotatedNode      uint64
}

// Stringer implements fmt.Stringer interface
func (mc *MembershipChanges) String() string {
	return fmt.Sprintf("add %d node(s), remove %d node(s)", len(mc.AddedNodes), len(mc.RemovedNodes))
}

// Changed indicates whether these changes actually do anything
func (mc *MembershipChanges) Changed() bool {
	return len(mc.AddedNodes) > 0 || len(mc.RemovedNodes) > 0
}

// Rotated indicates whether the change was a rotation
func (mc *MembershipChanges) Rotated() bool {
	return len(mc.AddedNodes) == 1 && len(mc.RemovedNodes) == 1
}

// EndpointconfigFromFromSupport extracts TLS CA certificates and endpoints from the ConsenterSupport
func EndpointconfigFromFromSupport(support consensus.ConsenterSupport) ([]cluster.EndpointCriteria, error) {
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
		return nil, errors.Errorf("unable to retrieve block [%d]", lastBlockSeq)
	}
	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, support)
	if err != nil {
		return nil, err
	}
	return lastConfigBlock, nil
}

// newBlockPuller creates a new block puller
func newBlockPuller(support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster) (BlockPuller, error) {

	verifyBlockSequence := func(blocks []*common.Block, _ string) error {
		return cluster.VerifyBlocks(blocks, support)
	}

	stdDialer := &cluster.StandardDialer{
		ClientConfig: baseDialer.ClientConfig.Clone(),
	}
	stdDialer.ClientConfig.AsyncConnect = false
	stdDialer.ClientConfig.SecOpts.VerifyCertificate = nil

	// Extract the TLS CA certs and endpoints from the configuration,
	endpoints, err := EndpointconfigFromFromSupport(support)
	if err != nil {
		return nil, err
	}

	der, _ := pem.Decode(stdDialer.ClientConfig.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(stdDialer.ClientConfig.SecOpts.Certificate))
	}

	bp := &cluster.BlockPuller{
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller"),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChainID(),
		Dialer:              stdDialer,
	}

	return &LedgerBlockPuller{
		Height:         support.Height,
		BlockRetriever: support,
		BlockPuller:    bp,
	}, nil
}

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

// MembershipByCert convert consenters map into set encapsulated by map
// where key is client TLS certificate
func MembershipByCert(consenters map[uint64]*etcdraft.Consenter) map[string]uint64 {
	set := map[string]uint64{}
	for nodeID, c := range consenters {
		set[string(c.ClientTlsCert)] = nodeID
	}
	return set
}

// ComputeMembershipChanges computes membership update based on information about new conseters, returns
// two slices: a slice of added consenters and a slice of consenters to be removed
func ComputeMembershipChanges(oldMetadata *etcdraft.BlockMetadata, oldConsenters map[uint64]*etcdraft.Consenter, newConsenters []*etcdraft.Consenter) (mc *MembershipChanges, err error) {
	result := &MembershipChanges{
		NewConsenters:    map[uint64]*etcdraft.Consenter{},
		NewBlockMetadata: proto.Clone(oldMetadata).(*etcdraft.BlockMetadata),
		AddedNodes:       []*etcdraft.Consenter{},
		RemovedNodes:     []*etcdraft.Consenter{},
	}

	result.NewBlockMetadata.ConsenterIds = make([]uint64, len(newConsenters))

	var addedNodeIndex int
	currentConsentersSet := MembershipByCert(oldConsenters)
	for i, c := range newConsenters {
		if nodeID, exists := currentConsentersSet[string(c.ClientTlsCert)]; exists {
			result.NewBlockMetadata.ConsenterIds[i] = nodeID
			result.NewConsenters[nodeID] = c
			continue
		}
		addedNodeIndex = i
		result.AddedNodes = append(result.AddedNodes, c)
	}

	var deletedNodeID uint64
	newConsentersSet := ConsentersToMap(newConsenters)
	for nodeID, c := range oldConsenters {
		if _, exists := newConsentersSet[string(c.ClientTlsCert)]; !exists {
			result.RemovedNodes = append(result.RemovedNodes, c)
			deletedNodeID = nodeID
		}
	}

	switch {
	case len(result.AddedNodes) == 1 && len(result.RemovedNodes) == 1:
		// cert rotation
		result.RotatedNode = deletedNodeID
		result.NewBlockMetadata.ConsenterIds[addedNodeIndex] = deletedNodeID
		result.NewConsenters[deletedNodeID] = result.AddedNodes[0]
	case len(result.AddedNodes) == 1 && len(result.RemovedNodes) == 0:
		// new node
		nodeID := result.NewBlockMetadata.NextConsenterId
		result.NewConsenters[nodeID] = result.AddedNodes[0]
		result.NewBlockMetadata.ConsenterIds[addedNodeIndex] = nodeID
		result.NewBlockMetadata.NextConsenterId++
		result.ConfChange = &raftpb.ConfChange{
			NodeID: nodeID,
			Type:   raftpb.ConfChangeAddNode,
		}
	case len(result.AddedNodes) == 0 && len(result.RemovedNodes) == 1:
		// removed node
		nodeID := deletedNodeID
		result.ConfChange = &raftpb.ConfChange{
			Type:   raftpb.ConfChangeRemoveNode,
			NodeID: nodeID,
		}
		delete(result.NewConsenters, nodeID)
	case len(result.AddedNodes) == 0 && len(result.RemovedNodes) == 0:
		// no change
	default:
		// len(result.AddedNodes) > 1 || len(result.RemovedNodes) > 1 {
		return nil, errors.Errorf("update of more than one consenter at a time is not supported, requested changes: %s", result)
	}

	return result, nil
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
	envelope, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract envelope from the block")
	}

	channelHeader, err := utils.ChannelHeader(envelope)
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

	envelope, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract envelope from the block")
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
func ConsensusMetadataFromConfigBlock(block *common.Block) (*etcdraft.ConfigMetadata, error) {
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

	if metadata.Options == nil {
		return errors.Errorf("nil Raft config metadata options")
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
			metadata.Options.ElectionTick, metadata.Options.HeartbeatTick)
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
	m := &etcdraft.ConfigMetadata{}
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

// PeriodicCheck checks periodically a condition, and reports
// the cumulative consecutive period the condition was fulfilled.
type PeriodicCheck struct {
	Logger              *flogging.FabricLogger
	CheckInterval       time.Duration
	Condition           func() bool
	Report              func(cumulativePeriod time.Duration)
	conditionHoldsSince time.Time
	once                sync.Once // Used to prevent double initialization
	stopped             uint32
}

// Run runs the PeriodicCheck
func (pc *PeriodicCheck) Run() {
	pc.once.Do(pc.check)
}

// Stop stops the periodic checks
func (pc *PeriodicCheck) Stop() {
	pc.Logger.Info("Periodic check is stopping.")
	atomic.AddUint32(&pc.stopped, 1)
}

func (pc *PeriodicCheck) shouldRun() bool {
	return atomic.LoadUint32(&pc.stopped) == 0
}

func (pc *PeriodicCheck) check() {
	if pc.Condition() {
		pc.conditionFulfilled()
	} else {
		pc.conditionNotFulfilled()
	}

	if !pc.shouldRun() {
		return
	}
	time.AfterFunc(pc.CheckInterval, pc.check)
}

func (pc *PeriodicCheck) conditionNotFulfilled() {
	pc.conditionHoldsSince = time.Time{}
}

func (pc *PeriodicCheck) conditionFulfilled() {
	if pc.conditionHoldsSince.IsZero() {
		pc.conditionHoldsSince = time.Now()
	}

	pc.Report(time.Since(pc.conditionHoldsSince))
}

// LedgerBlockPuller pulls blocks upon demand, or fetches them
// from the ledger.
type LedgerBlockPuller struct {
	BlockPuller
	BlockRetriever cluster.BlockRetriever
	Height         func() uint64
}

func (ledgerPuller *LedgerBlockPuller) PullBlock(seq uint64) *common.Block {
	lastSeq := ledgerPuller.Height() - 1
	if lastSeq >= seq {
		return ledgerPuller.BlockRetriever.Block(seq)
	}
	return ledgerPuller.BlockPuller.PullBlock(seq)
}

type evictionSuspector struct {
	evictionSuspicionThreshold time.Duration
	logger                     *flogging.FabricLogger
	createPuller               CreateBlockPuller
	height                     func() uint64
	amIInChannel               cluster.SelfMembershipPredicate
	halt                       func()
	writeBlock                 func(block *common.Block) error
	triggerCatchUp             func(sn *raftpb.Snapshot)
	halted                     bool
}

func (es *evictionSuspector) confirmSuspicion(cumulativeSuspicion time.Duration) {
	if es.evictionSuspicionThreshold > cumulativeSuspicion || es.halted {
		return
	}
	es.logger.Infof("Suspecting our own eviction from the channel for %v", cumulativeSuspicion)
	puller, err := es.createPuller()
	if err != nil {
		es.logger.Panicf("Failed creating a block puller")
	}

	lastConfigBlock, err := cluster.PullLastConfigBlock(puller)
	if err != nil {
		es.logger.Errorf("Failed pulling the last config block: %v", err)
		return
	}

	es.logger.Infof("Last config block was found to be block [%d]", lastConfigBlock.Header.Number)

	height := es.height()

	if lastConfigBlock.Header.Number+1 <= height {
		es.logger.Infof("Our height is higher or equal than the height of the orderer we pulled the last block from, aborting.")
		return
	}

	err = es.amIInChannel(lastConfigBlock)
	if err != cluster.ErrNotInChannel && err != cluster.ErrForbidden {
		details := fmt.Sprintf(", our certificate was found in config block with sequence %d", lastConfigBlock.Header.Number)
		if err != nil {
			details = fmt.Sprintf(": %s", err.Error())
		}
		es.logger.Infof("Cannot confirm our own eviction from the channel%s", details)

		es.triggerCatchUp(&raftpb.Snapshot{Data: utils.MarshalOrPanic(lastConfigBlock)})
		return
	}

	es.logger.Warningf("Detected our own eviction from the channel in block [%d]", lastConfigBlock.Header.Number)

	es.logger.Infof("Waiting for chain to halt")
	es.halt()
	es.halted = true
	es.logger.Infof("Chain has been halted, pulling remaining blocks up to (and including) eviction block.")

	nextBlock := height
	es.logger.Infof("Will now pull blocks %d to %d", nextBlock, lastConfigBlock.Header.Number)
	for seq := nextBlock; seq <= lastConfigBlock.Header.Number; seq++ {
		es.logger.Infof("Pulling block [%d]", seq)
		block := puller.PullBlock(seq)
		err := es.writeBlock(block)
		if err != nil {
			es.logger.Panicf("Failed writing block [%d] to the ledger: %v", block.Header.Number, err)
		}
	}

	es.logger.Infof("Pulled all blocks up to eviction block.")
}
