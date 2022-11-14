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
	"crypto/x509"
	"encoding/pem"
	"path"
	"reflect"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
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
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/inactive"
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

//go:generate mockery -dir . -name InactiveChainRegistry -case underscore -output mocks

// InactiveChainRegistry registers chains that are inactive
type InactiveChainRegistry interface {
	// TrackChain tracks a chain with the given name, and calls the given callback
	// when this chain should be created.
	TrackChain(chainName string, genesisBlock *cb.Block, createChain func())
	// Stop stops the InactiveChainRegistry. This is used when removing the
	// system channel.
	Stop()
}

// Consenter implementation of the BFT smart based consenter
type Consenter struct {
	CreateChain           func(chainName string)
	InactiveChainRegistry InactiveChainRegistry
	GetPolicyManager      PolicyManagerRetriever
	Logger                *flogging.FabricLogger
	Cert                  []byte
	Comm                  *cluster.Comm
	Chains                ChainGetter
	SignerSerializer      SignerSerializer
	Registrar             *multichannel.Registrar
	WALBaseDir            string
	ClusterDialer         *cluster.PredicateDialer
	Conf                  *localconfig.TopLevel
	Metrics               *Metrics
	BCCSP                 bccsp.BCCSP
}

// New creates Consenter of type smart bft
func New(
	icr InactiveChainRegistry,
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
		InactiveChainRegistry: icr,
		Registrar:             r,
		GetPolicyManager:      pmr,
		Conf:                  conf,
		ClusterDialer:         clusterDialer,
		Logger:                logger,
		Cert:                  srvConf.SecOpts.Certificate,
		Chains:                r,
		SignerSerializer:      signerSerializer,
		WALBaseDir:            walConfig.WALDir,
		Metrics:               NewMetrics(metricsProvider),
		CreateChain:           r.CreateChain,
		BCCSP:                 BCCSP,
	}

	compareCert := cluster.CachePublicKeyComparisons(func(a, b []byte) bool {
		err := crypto.CertificatesWithSamePublicKey(a, b)
		if err != nil && err != crypto.ErrPubKeyMismatch {
			crypto.LogNonPubKeyMismatchErr(logger.Errorf, err, a, b)
		}
		return err == nil
	})

	consenter.Comm = &cluster.Comm{
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		SendBufferSize:                   conf.General.Cluster.SendBufferSize,
		Logger:                           flogging.MustGetLogger("orderer.common.cluster"),
		Chan2Members:                     make(map[string]cluster.MemberMapping),
		Connections:                      cluster.NewConnectionStore(clusterDialer, metrics.EgressTLSConnectionCount),
		Metrics:                          metrics,
		ChanExt:                          consenter,
		H: &Ingress{
			Logger:        logger,
			ChainSelector: consenter,
		},
		CompareCertificate: compareCert,
	}

	svc := &cluster.Service{
		CertExpWarningThreshold:          conf.General.Cluster.CertExpirationWarningThreshold,
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: metrics,
		},
		StepLogger: flogging.MustGetLogger("orderer.common.cluster.step"),
		Logger:     flogging.MustGetLogger("orderer.common.cluster"),
		Dispatcher: consenter.Comm,
	}

	ab.RegisterClusterServer(srv.Server(), svc)

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
		if c.InactiveChainRegistry != nil {
			c.Logger.Errorf("channel %s is not serviced by me", support.ChannelID())
			c.InactiveChainRegistry.TrackChain(support.ChannelID(), support.Block(0), func() {
				c.CreateChain(support.ChannelID())
			})
			return &inactive.Chain{Err: errors.Errorf("channel %s is not serviced by me", support.ChannelID())}, nil
		}

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
		ChannelConfigTemplator: c.Registrar,
		ValidatingChannel:      support.ChannelID(),
		Filters:                c.Registrar,
		ConfigUpdateProposer:   c.Registrar,
		Logger:                 c.Logger,
	}

	chain, err := NewChain(configValidator, (uint64)(selfID), config, path.Join(c.WALBaseDir, support.ChannelID()), puller, c.Comm, c.SignerSerializer, c.GetPolicyManager(support.ChannelID()), support, c.Metrics, c.BCCSP)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating a new BFTChain")
	}

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
	configOptions := &smartbft.Options{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), configOptions); err != nil {
		return false, err
	}

	verifyOpts, err := etcdraft.CreateX509VerifyOptions(oc)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create x509 verify options from orderer config")
	}

	if err := VerifyConfigMetadata(configOptions, verifyOpts); err != nil {
		return false, errors.Wrapf(err, "failed to validate config metadata of ordering config")
	}

	member := false
	for _, consenter := range oc.Consenters() {
		if bytes.Equal(c.Cert, consenter.ServerTlsCert) || bytes.Equal(c.Cert, consenter.ClientTlsCert) {
			member = true
			break
		}
	}

	return member, nil
}

// RemoveInactiveChainRegistry stops and removes the inactive chain registry.
// This is used when removing the system channel.
func (c *Consenter) RemoveInactiveChainRegistry() {
	if c.InactiveChainRegistry == nil {
		return
	}
	c.InactiveChainRegistry.Stop()
	c.InactiveChainRegistry = nil
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
	var serverCertificates []string
	for _, cst := range consenters {
		serverCertificates = append(serverCertificates, string(cst.ServerTlsCert))
		if bytes.Equal(c.Cert, cst.ServerTlsCert) {
			return cst.Id, nil
		}
	}

	c.Logger.Warning("Could not find", string(c.Cert), "among", serverCertificates)
	return 0, cluster.ErrNotInChannel
}

// VerifyConfigMetadata validates SmartBFT config metadata.
// Note: ignores certificates expiration.
func VerifyConfigMetadata(options *smartbft.Options, verifyOpts x509.VerifyOptions) error {
	if options == nil {
		// defensive check. this should not happen as CheckConfigMetadata
		// should always be called with non-nil config metadata
		return errors.Errorf("nil SmartBFT config options")
	}

	// todo: check metadata

	return nil
}
