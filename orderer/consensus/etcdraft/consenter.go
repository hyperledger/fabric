/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"path"
	"reflect"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft/v3"
)

//go:generate counterfeiter -o mocks/chain_manager.go --fake-name ChainManager . ChainManager

// ChainManager defines the methods from multichannel.Registrar needed by the Consenter.
type ChainManager interface {
	GetConsensusChain(channelID string) consensus.Chain
	CreateChain(channelID string)
	SwitchChainToFollower(channelID string)
	ReportConsensusRelationAndStatusMetrics(channelID string, relation types.ConsensusRelation, status types.Status)
}

// Config contains etcdraft configurations
type Config struct {
	WALDir               string // WAL data of <my-channel> is stored in WALDir/<my-channel>
	SnapDir              string // Snapshots of <my-channel> are stored in SnapDir/<my-channel>
	EvictionSuspicion    string // Duration threshold that the node samples in order to suspect its eviction from the channel.
	TickIntervalOverride string // Duration to use for tick interval instead of what is specified in the channel config.
}

// Consenter implements etcdraft consenter
type Consenter struct {
	ChainManager  ChainManager
	Dialer        *cluster.PredicateDialer
	Communication cluster.Communicator
	*Dispatcher
	Logger         *flogging.FabricLogger
	EtcdRaftConfig Config
	OrdererConfig  localconfig.TopLevel
	Cert           []byte
	Metrics        *Metrics
	BCCSP          bccsp.BCCSP
}

// TargetChannel extracts the channel from the given proto.Message.
// Returns an empty string on failure.
func (c *Consenter) TargetChannel(message proto.Message) string {
	switch req := message.(type) {
	case *orderer.ConsensusRequest:
		return req.Channel
	case *orderer.SubmitRequest:
		return req.Channel
	default:
		return ""
	}
}

// ReceiverByChain returns the MessageReceiver for the given channelID or nil
// if not found.
func (c *Consenter) ReceiverByChain(channelID string) MessageReceiver {
	chain := c.ChainManager.GetConsensusChain(channelID)
	if chain == nil {
		return nil
	}
	if etcdRaftChain, isEtcdRaftChain := chain.(*Chain); isEtcdRaftChain {
		return etcdRaftChain
	}
	c.Logger.Warningf("Chain %s is of type %v and not etcdraft.Chain", channelID, reflect.TypeOf(chain))
	return nil
}

func (c *Consenter) detectSelfID(consenters map[uint64]*etcdraft.Consenter) (uint64, error) {
	thisNodeCertAsDER, err := pemToDER(c.Cert, 0, "server", c.Logger)
	if err != nil {
		return 0, err
	}

	var serverCertificates []string
	for nodeID, cst := range consenters {
		serverCertificates = append(serverCertificates, string(cst.ServerTlsCert))

		certAsDER, err := pemToDER(cst.ServerTlsCert, nodeID, "server", c.Logger)
		if err != nil {
			return 0, err
		}

		if crypto.CertificatesWithSamePublicKey(thisNodeCertAsDER, certAsDER) == nil {
			return nodeID, nil
		}
	}

	c.Logger.Warning("Could not find", string(c.Cert), "among", serverCertificates)
	return 0, cluster.ErrNotInChannel
}

// HandleChain returns a new Chain instance or an error upon failure
func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	m := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}

	if m.Options == nil {
		return nil, errors.New("etcdraft options have not been provided")
	}

	isMigration := (metadata == nil || len(metadata.Value) == 0) && (support.Height() > 1)
	if isMigration {
		c.Logger.Debugf("Block metadata is nil at block height=%d, it is consensus-type migration", support.Height())
	}

	// determine raft replica set mapping for each node to its id
	// for newly started chain we need to read and initialize raft
	// metadata by creating mapping between conseter and its id.
	// In case chain has been restarted we restore raft metadata
	// information from the recently committed block meta data
	// field.
	blockMetadata, err := ReadBlockMetadata(metadata, m)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read Raft metadata")
	}

	consenters := CreateConsentersMap(blockMetadata, m)

	id, err := c.detectSelfID(consenters)
	if err != nil {
		return nil, errors.Wrap(err, "without a system channel, a follower should have been created")
	}

	var evictionSuspicion time.Duration
	if c.EtcdRaftConfig.EvictionSuspicion == "" {
		c.Logger.Infof("EvictionSuspicion not set, defaulting to %v", DefaultEvictionSuspicion)
		evictionSuspicion = DefaultEvictionSuspicion
	} else {
		evictionSuspicion, err = time.ParseDuration(c.EtcdRaftConfig.EvictionSuspicion)
		if err != nil {
			c.Logger.Panicf("Failed parsing Consensus.EvictionSuspicion: %s: %v", c.EtcdRaftConfig.EvictionSuspicion, err)
		}
	}

	var tickInterval time.Duration
	if c.EtcdRaftConfig.TickIntervalOverride == "" {
		tickInterval, err = time.ParseDuration(m.Options.TickInterval)
		if err != nil {
			return nil, errors.Errorf("failed to parse TickInterval (%s) to time duration", m.Options.TickInterval)
		}
	} else {
		tickInterval, err = time.ParseDuration(c.EtcdRaftConfig.TickIntervalOverride)
		if err != nil {
			return nil, errors.WithMessage(err, "failed parsing Consensus.TickIntervalOverride")
		}
		c.Logger.Infof("TickIntervalOverride is set, overriding channel configuration tick interval to %v", tickInterval)
	}

	opts := Options{
		RPCTimeout:    c.OrdererConfig.General.Cluster.RPCTimeout,
		RaftID:        id,
		Clock:         clock.NewClock(),
		MemoryStorage: raft.NewMemoryStorage(),
		Logger:        c.Logger,

		TickInterval:         tickInterval,
		ElectionTick:         int(m.Options.ElectionTick),
		HeartbeatTick:        int(m.Options.HeartbeatTick),
		MaxInflightBlocks:    int(m.Options.MaxInflightBlocks),
		MaxSizePerMsg:        uint64(support.SharedConfig().BatchSize().PreferredMaxBytes),
		SnapshotIntervalSize: m.Options.SnapshotIntervalSize,

		BlockMetadata: blockMetadata,
		Consenters:    consenters,

		MigrationInit: isMigration,

		WALDir:            path.Join(c.EtcdRaftConfig.WALDir, support.ChannelID()),
		SnapDir:           path.Join(c.EtcdRaftConfig.SnapDir, support.ChannelID()),
		EvictionSuspicion: evictionSuspicion,
		Cert:              c.Cert,
		Metrics:           c.Metrics,
	}

	rpc := &cluster.RPC{
		Timeout:       c.OrdererConfig.General.Cluster.RPCTimeout,
		Logger:        c.Logger,
		Channel:       support.ChannelID(),
		Comm:          c.Communication,
		StreamsByType: cluster.NewStreamsByType(),
	}

	// Called after the etcdraft.Chain halts when it detects eviction from the cluster.
	// When we do NOT have a system channel, we switch to a follower.Chain upon eviction.
	c.Logger.Info("After eviction from the cluster Registrar.SwitchToFollower will be called, and the orderer will become a follower of the channel.")
	haltCallback := func() { c.ChainManager.SwitchChainToFollower(support.ChannelID()) }

	return NewChain(
		support,
		opts,
		c.Communication,
		rpc,
		c.BCCSP,
		func() (BlockPuller, error) {
			return NewBlockPuller(support, c.Dialer, c.OrdererConfig.General.Cluster, c.BCCSP)
		},
		haltCallback,
		nil,
	)
}

func (c *Consenter) IsChannelMember(joinBlock *common.Block) (bool, error) {
	if joinBlock == nil {
		return false, errors.New("nil block")
	}
	envelopeConfig, err := protoutil.ExtractEnvelope(joinBlock, 0)
	if err != nil {
		return false, err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig, c.BCCSP)
	if err != nil {
		return false, err
	}
	oc, exists := bundle.OrdererConfig()
	if !exists {
		return false, errors.New("no orderer config in bundle")
	}
	configMetadata := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), configMetadata); err != nil {
		return false, err
	}

	verifyOpts, err := createX509VerifyOptions(oc)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create x509 verify options from orderer config")
	}

	if err := VerifyConfigMetadata(configMetadata, verifyOpts); err != nil {
		return false, errors.Wrapf(err, "failed to validate config metadata of ordering config")
	}

	consenters := make(map[uint64]*etcdraft.Consenter)
	for i, c := range configMetadata.Consenters {
		consenters[uint64(i+1)] = c // the IDs don't matter
	}

	if _, err := c.detectSelfID(consenters); err != nil {
		if err != cluster.ErrNotInChannel {
			return false, errors.Wrapf(err, "failed to detect self ID by comparing public keys")
		}
		return false, nil
	}

	return true, nil
}

// ReadBlockMetadata attempts to read raft metadata from block metadata, if available.
// otherwise, it reads raft metadata from config metadata supplied.
func ReadBlockMetadata(blockMetadata *common.Metadata, configMetadata *etcdraft.ConfigMetadata) (*etcdraft.BlockMetadata, error) {
	if blockMetadata != nil && len(blockMetadata.Value) != 0 { // we have consenters mapping from block
		m := &etcdraft.BlockMetadata{}
		if err := proto.Unmarshal(blockMetadata.Value, m); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal block's metadata")
		}
		return m, nil
	}

	m := &etcdraft.BlockMetadata{
		NextConsenterId: 1,
		ConsenterIds:    make([]uint64, len(configMetadata.Consenters)),
	}
	// need to read consenters from the configuration
	for i := range m.ConsenterIds {
		m.ConsenterIds[i] = m.NextConsenterId
		m.NextConsenterId++
	}

	return m, nil
}

// New creates a etcdraft Consenter
func New(
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	registrar ChainManager,
	metricsProvider metrics.Provider,
	bccsp bccsp.BCCSP,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.etcdraft")

	var cfg Config
	err := mapstructure.Decode(conf.Consensus, &cfg)
	if err != nil {
		logger.Panicf("Failed to decode etcdraft configuration: %s", err)
	}

	consenter := &Consenter{
		ChainManager:   registrar,
		Cert:           srvConf.SecOpts.Certificate,
		Logger:         logger,
		EtcdRaftConfig: cfg,
		OrdererConfig:  *conf,
		Dialer:         clusterDialer,
		Metrics:        NewMetrics(metricsProvider),
		BCCSP:          bccsp,
	}
	consenter.Dispatcher = &Dispatcher{
		Logger:        logger,
		ChainSelector: consenter,
	}

	comm := createComm(clusterDialer, consenter, conf.General.Cluster, metricsProvider)
	consenter.Communication = comm
	svc := &cluster.Service{
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: comm.Metrics,
		},
		StepLogger: flogging.MustGetLogger("orderer.common.cluster.step"),
		Logger:     flogging.MustGetLogger("orderer.common.cluster"),
		Dispatcher: comm,
	}
	orderer.RegisterClusterServer(srv.Server(), svc)

	return consenter
}

func createComm(clusterDialer *cluster.PredicateDialer, c *Consenter, config localconfig.Cluster, p metrics.Provider) *cluster.Comm {
	metrics := cluster.NewMetrics(p)
	logger := flogging.MustGetLogger("orderer.common.cluster")

	compareCert := cluster.CachePublicKeyComparisons(func(a, b []byte) bool {
		err := crypto.CertificatesWithSamePublicKey(a, b)
		if err != nil && err != crypto.ErrPubKeyMismatch {
			crypto.LogNonPubKeyMismatchErr(logger.Errorf, err, a, b)
		}
		return err == nil
	})

	comm := &cluster.Comm{
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		CertExpWarningThreshold:          config.CertExpirationWarningThreshold,
		SendBufferSize:                   config.SendBufferSize,
		Logger:                           logger,
		Chan2Members:                     make(map[string]cluster.MemberMapping),
		Connections:                      cluster.NewConnectionStore(clusterDialer, metrics.EgressTLSConnectionCount),
		Metrics:                          metrics,
		ChanExt:                          c,
		H:                                c,
		CompareCertificate:               compareCert,
	}
	c.Communication = comm
	return comm
}
