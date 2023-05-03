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

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
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
	BCCSP            bccsp.BCCSP
	ClusterService   *cluster.ClusterService
}

// New creates Consenter of type smart bft
func New(
	pmr PolicyManagerRetriever,
	signerSerializer SignerSerializer,
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	r *multichannel.Registrar,
	metricsProvider metrics.Provider,
	BCCSP bccsp.BCCSP,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.smartbft")

	metrics := cluster.NewMetrics(metricsProvider)

	var walConfig WALConfig
	err := mapstructure.Decode(conf.Consensus, &walConfig)
	if err != nil {
		logger.Panicf("Failed to decode consensus configuration: %s", err)
	}

	logger.Infof("WAL Directory is %s", walConfig.WALDir)

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
		Metrics:        metrics,
		SendBufferSize: conf.General.Cluster.SendBufferSize,
		Chan2Members:   make(cluster.MembersByChannel),
		Connections:    cluster.NewConnectionMgr(clusterDialer.Config),
		Signer:         signerSerializer,
		NodeIdentity:   sID.IdBytes,
	}

	consenter.ClusterService = &cluster.ClusterService{
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: metrics,
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
	configOptions := &smartbft.Options{}
	consenters := support.SharedConfig().Consenters()
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), configOptions); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}

	selfID, err := c.detectSelfID(consenters)
	if err != nil {
		return nil, errors.Wrap(err, "without a system channel, a follower should have been created")
	}
	c.Logger.Infof("Local consenter id is %d", selfID)

	puller, err := newBlockPuller(support, c.ClusterDialer, c.Conf.General.Cluster, c.BCCSP)
	if err != nil {
		c.Logger.Panicf("Failed initializing block puller")
	}

	config, err := configFromMetadataOptions((uint64)(selfID), configOptions)
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

	chain, err := NewChain(configValidator, (uint64)(selfID), config, path.Join(c.WALBaseDir, support.ChannelID()), puller, c.Comm, c.SignerSerializer, c.GetPolicyManager(support.ChannelID()), support, c.Metrics, c.BCCSP)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating a new BFTChain")
	}

	// refresh cluster service with updated consenters
	c.ClusterService.ConfigureNodeCerts(chain.Channel, consenters)
	chain.clusterService = c.ClusterService

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
