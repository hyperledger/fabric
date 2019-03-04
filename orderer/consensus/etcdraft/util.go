/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
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
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
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

	bp := &cluster.BlockPuller{
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
	}

	return &LedgerBlockPuller{
		Height:         support.Height,
		BlockRetriever: support,
		BlockPuller:    bp,
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
func ConsensusMetadataFromConfigBlock(block *common.Block) (*etcdraft.Metadata, error) {
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

	payload, err := protoutil.ExtractPayload(configEnvelope)
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

// ConsenterCertificate denotes a TLS certificate of a consenter
type ConsenterCertificate []byte

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

// SliceOfConsentersIDs converts maps of consenters into slice of consenters ids
func SliceOfConsentersIDs(consenters map[uint64]*etcdraft.Consenter) []uint64 {
	result := make([]uint64, 0)
	for id := range consenters {
		result = append(result, id)
	}

	return result
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

// ConfChange computes Raft configuration changes based on current Raft configuration state and
// consenters mapping stored in RaftMetadata
func ConfChange(raftMetadata *etcdraft.RaftMetadata, confState *raftpb.ConfState) *raftpb.ConfChange {
	raftConfChange := &raftpb.ConfChange{}

	raftConfChange.ID = raftMetadata.ConfChangeCounts
	// need to compute conf changes to propose
	if len(confState.Nodes) < len(raftMetadata.Consenters) {
		// adding new node
		raftConfChange.Type = raftpb.ConfChangeAddNode
		for consenterID := range raftMetadata.Consenters {
			if NodeExists(consenterID, confState.Nodes) {
				continue
			}
			raftConfChange.NodeID = consenterID
		}
	} else {
		// removing node
		raftConfChange.Type = raftpb.ConfChangeRemoveNode
		consentersIDs := SliceOfConsentersIDs(raftMetadata.Consenters)
		for _, nodeID := range confState.Nodes {
			if NodeExists(nodeID, consentersIDs) {
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
	writeBlock                 func(block *common.Block, metadata []byte)
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

	es.logger.Infof("Last config block was found to be block %d", lastConfigBlock.Header.Number)

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
		return
	}

	es.logger.Warningf("Detected our own eviction from the chain in block %d", lastConfigBlock.Header.Number)

	es.logger.Infof("Waiting for chain to halt")
	es.halt()
	es.halted = true
	es.logger.Infof("Chain has been halted, pulling remaining blocks up to (and including) eviction block.")

	nextBlock := height
	es.logger.Infof("Will now pull blocks %d to %d", nextBlock, lastConfigBlock.Header.Number)
	for seq := nextBlock; seq <= lastConfigBlock.Header.Number; seq++ {
		es.logger.Infof("Pulling block %d", seq)
		block := puller.PullBlock(seq)
		es.writeBlock(block, nil)
	}

	es.logger.Infof("Pulled all blocks up to eviction block.")
}
