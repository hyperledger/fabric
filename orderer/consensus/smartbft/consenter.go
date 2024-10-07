/*
 *
 * Copyright IBM Corp. All Rights Reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 * /
 *
 */

package smartbft

import (
	"bytes"
	"encoding/pem"
	"path"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/hyperledger-labs/SmartBFT/pkg/wal"
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/metrics"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// CreateChainCallback creates a new chain
type CreateChainCallback func()

// ChainGetter obtains instances of ChainSupport for the given channel
type ChainGetter interface {
	// GetChain obtains the ChainSupport for the given channel.
	// Returns nil, false when the ChainSupport for the given channel
	// isn't found.
	GetChain(chainID string) *multichannel.ChainSupport
}

// PolicyManagerRetriever is the policy manager retriever function
type PolicyManagerRetriever func(channel string) policies.Manager

// Consenter implementation of the BFT smart based consenter
type Consenter struct {
	CreateChain      func(chainName string)
	GetPolicyManager PolicyManagerRetriever
	Logger           *flogging.FabricLogger
	Identity         []byte
	Comm             *cluster.AuthCommMgr
	Chains           ChainGetter
	SignerSerializer SignerSerializer
	Registrar        *multichannel.Registrar
	WALBaseDir       string
	ClusterDialer    *cluster.PredicateDialer
	Conf             *localconfig.TopLevel
	Metrics          *Metrics
	MetricsBFT       *api.Metrics
	MetricsWalBFT    *wal.Metrics
	BCCSP            bccsp.BCCSP
	ClusterService   *cluster.ClusterService
}

// New creates Consenter of type smart bft
func New(
	pmr PolicyManagerRetriever,
	signerSerializer SignerSerializer,
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srvConf comm.ServerConfig, // TODO why is this not used?
	srv *comm.GRPCServer,
	r *multichannel.Registrar,
	metricsProvider metrics.Provider,
	clusterMetrics *cluster.Metrics,
	BCCSP bccsp.BCCSP,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.smartbft")

	var walConfig WALConfig
	err := mapstructure.Decode(conf.Consensus, &walConfig)
	if err != nil {
		logger.Panicf("Failed to decode consensus configuration: %s", err)
	}

	logger.Infof("WAL Directory is %s", walConfig.WALDir)

	mpc := &MetricProviderConverter{
		MetricsProvider: metricsProvider,
	}

	consenter := &Consenter{
		Registrar:        r,
		GetPolicyManager: pmr,
		Conf:             conf,
		ClusterDialer:    clusterDialer,
		Logger:           logger,
		Chains:           r,
		SignerSerializer: signerSerializer,
		WALBaseDir:       walConfig.WALDir,
		Metrics:          NewMetrics(metricsProvider),
		MetricsBFT:       api.NewMetrics(mpc, "channel"),
		MetricsWalBFT:    wal.NewMetrics(mpc, "channel"),
		CreateChain:      r.CreateChain,
		BCCSP:            BCCSP,
	}

	identity, _ := signerSerializer.Serialize()
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(identity, sID); err != nil {
		logger.Panicf("failed unmarshaling identity: %s", err)
	}

	consenter.Identity = sID.IdBytes

	consenter.Comm = &cluster.AuthCommMgr{
		Logger:         flogging.MustGetLogger("orderer.common.cluster"),
		Metrics:        clusterMetrics,
		SendBufferSize: conf.General.Cluster.SendBufferSize,
		Chan2Members:   make(cluster.MembersByChannel),
		Connections:    cluster.NewConnectionMgr(clusterDialer.Config),
		Signer:         signerSerializer,
		NodeIdentity:   sID.IdBytes,
	}

	consenter.ClusterService = &cluster.ClusterService{
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: clusterMetrics,
		},
		Logger:                           flogging.MustGetLogger("orderer.common.cluster"),
		StepLogger:                       flogging.MustGetLogger("orderer.common.cluster.step"),
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		MembershipByChannel:              make(map[string]*cluster.ChannelMembersConfig),
		NodeIdentity:                     sID.IdBytes,
		RequestHandler: &Ingress{
			Logger:        logger,
			ChainSelector: consenter,
		},
	}

	ab.RegisterClusterNodeServiceServer(srv.Server(), consenter.ClusterService)

	return consenter
}

// ReceiverByChain returns the MessageReceiver for the given channelID or nil if not found.
func (c *Consenter) ReceiverByChain(channelID string) MessageReceiver {
	cs := c.Chains.GetChain(channelID)
	if cs == nil {
		return nil
	}
	if cs.Chain == nil {
		c.Logger.Panicf("Programming error - Chain %s is nil although it exists in the mapping", channelID)
	}
	if smartBFTChain, isBFTSmart := cs.Chain.(*BFTChain); isBFTSmart {
		return smartBFTChain
	}
	c.Logger.Warningf("Chain %s is of type %v and not smartbft.Chain", channelID, reflect.TypeOf(cs.Chain))
	return nil
}

// HandleChain returns a new Chain instance or an error upon failure
func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *cb.Metadata) (consensus.Chain, error) {
	consenters := support.SharedConfig().Consenters()
	configOptions, err := createSmartBftConfig(support.SharedConfig())
	if err != nil {
		return nil, err
	}

	selfID, err := c.detectSelfID(consenters)
	if err != nil {
		return nil, errors.Wrap(err, "without a system channel, a follower should have been created")
	}
	c.Logger.Infof("Local consenter id is %d", selfID)

	config, err := util.ConfigFromMetadataOptions((uint64)(selfID), configOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing smartbft configuration")
	}
	c.Logger.Debugf("SmartBFT-Go config: %+v", config)

	configValidator := &ConfigBlockValidator{
		ValidatingChannel:    support.ChannelID(),
		Filters:              c.Registrar,
		ConfigUpdateProposer: c.Registrar,
		Logger:               c.Logger,
	}

	egressCommFactory := func(runtimeConfig *atomic.Value, channelId string, comm cluster.Communicator) EgressComm {
		channelDecorator := zap.String("channel", channelId)
		return &Egress{
			RuntimeConfig: runtimeConfig,
			Channel:       channelId,
			Logger:        flogging.MustGetLogger("orderer.consensus.smartbft.egress").With(channelDecorator),
			RPC: &cluster.RPC{
				Logger:        flogging.MustGetLogger("orderer.consensus.smartbft.rpc").With(channelDecorator),
				Channel:       channelId,
				StreamsByType: cluster.NewStreamsByType(),
				Comm:          comm,
				Timeout:       5 * time.Minute, // TODO: Externalize configuration
			},
		}
	}

	chain, err := NewChain(
		configValidator,
		(uint64)(selfID),
		config,
		path.Join(c.WALBaseDir, support.ChannelID()),
		c.ClusterDialer,
		c.Conf.General.Cluster,
		c.Comm,
		c.SignerSerializer,
		c.GetPolicyManager(support.ChannelID()),
		support,
		c.Metrics,
		c.MetricsBFT,
		c.MetricsWalBFT,
		c.BCCSP,
		egressCommFactory,
		&synchronizerCreator{})
	if err != nil {
		return nil, errors.Wrap(err, "failed creating a new BFTChain")
	}

	// refresh cluster service with updated consenters
	c.ClusterService.ConfigureNodeCerts(chain.Channel, consenters)
	chain.ClusterService = c.ClusterService

	return chain, nil
}

func (c *Consenter) IsChannelMember(joinBlock *cb.Block) (bool, error) {
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
	member := false
	for _, consenter := range oc.Consenters() {
		santizedCert, err := crypto.SanitizeX509Cert(consenter.Identity)
		if err != nil {
			return false, err
		}
		if bytes.Equal(c.Identity, santizedCert) {
			member = true
			break
		}
	}

	return member, nil
}

// TargetChannel extracts the channel from the given proto.Message.
// Returns an empty string on failure.
func (c *Consenter) TargetChannel(message proto.Message) string {
	switch req := message.(type) {
	case *ab.ConsensusRequest:
		return req.Channel
	case *ab.SubmitRequest:
		return req.Channel
	default:
		return ""
	}
}

func pemToDER(pemBytes []byte, id uint64, certType string, logger *flogging.FabricLogger) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

func (c *Consenter) detectSelfID(consenters []*cb.Consenter) (uint32, error) {
	for _, cst := range consenters {
		santizedCert, err := crypto.SanitizeX509Cert(cst.Identity)
		if err != nil {
			return 0, err
		}
		if bytes.Equal(c.Comm.NodeIdentity, santizedCert) {
			return cst.Id, nil
		}
	}
	c.Logger.Warning("Could not find the node in channel consenters set")
	return 0, cluster.ErrNotInChannel
}
