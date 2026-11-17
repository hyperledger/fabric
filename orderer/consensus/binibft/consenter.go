/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package binibft is a Hyperledger Fabric ordering-service consenter backed by
// the BiniBFT protocol. It is a drop-in replacement for the SmartBFT consenter:
// it registers for the "BFT" consensus type, takes its consenter set from the
// channel config, and writes blocks that Fabric peers verify with the same BFT
// block-validation policy (a quorum of consenter signatures).
package binibft

import (
	"bytes"
	"encoding/pem"
	"reflect"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	fabricmsp "github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

type ChainGetter interface {
	GetChain(chainID string) *multichannel.ChainSupport
}

// Consenter implements consensus.Consenter and consensus.ClusterConsenter.
type Consenter struct {
	Logger         *flogging.FabricLogger
	Identity       []byte // PEM certificate of the local orderer's signing identity
	Comm           *cluster.AuthCommMgr
	Chains         ChainGetter
	Conf           *localconfig.TopLevel
	BCCSP          bccsp.BCCSP
	ClusterService *cluster.ClusterService
}

var (
	_ consensus.Consenter        = (*Consenter)(nil)
	_ consensus.ClusterConsenter = (*Consenter)(nil)
	_ ReceiverGetter             = (*Consenter)(nil)
)

// New creates the BiniBFT consenter and registers the cluster service, which
// Fabric allows only once per orderer process.
func New(
	signerSerializer identity.SignerSerializer,
	clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	srv *comm.GRPCServer,
	r *multichannel.Registrar,
	clusterMetrics *cluster.Metrics,
	bccsp bccsp.BCCSP,
) *Consenter {
	logger := flogging.MustGetLogger("orderer.consensus.binibft")

	serialized, err := signerSerializer.Serialize()
	if err != nil {
		logger.Panicf("failed serializing the local identity: %s", err)
	}
	sID := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(serialized, sID); err != nil {
		logger.Panicf("failed unmarshaling the local identity: %s", err)
	}

	c := &Consenter{
		Logger:   logger,
		Identity: sID.IdBytes,
		Chains:   r,
		Conf:     conf,
		BCCSP:    bccsp,
	}

	c.Comm = &cluster.AuthCommMgr{
		Logger:         flogging.MustGetLogger("orderer.common.cluster"),
		Metrics:        clusterMetrics,
		SendBufferSize: conf.General.Cluster.SendBufferSize,
		Chan2Members:   make(cluster.MembersByChannel),
		Connections:    cluster.NewConnectionMgr(clusterDialer.Config),
		Signer:         signerSerializer,
		NodeIdentity:   sID.IdBytes,
	}

	c.ClusterService = &cluster.ClusterService{
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
			ChainSelector: c,
		},
	}

	ab.RegisterClusterNodeServiceServer(srv.Server(), c.ClusterService)

	return c
}

func (c *Consenter) ReceiverByChain(channelID string) MessageReceiver {
	cs := c.Chains.GetChain(channelID)
	if cs == nil {
		return nil
	}
	if cs.Chain == nil {
		c.Logger.Panicf("Programming error - Chain %s is nil although it exists in the mapping", channelID)
	}
	if chain, ok := cs.Chain.(*Chain); ok {
		return chain
	}
	c.Logger.Warningf("Chain %s is of type %v and not a binibft.Chain", channelID, reflect.TypeOf(cs.Chain))
	return nil
}

func (c *Consenter) HandleChain(support consensus.ConsenterSupport, _ *cb.Metadata) (consensus.Chain, error) {
	consenters := support.SharedConfig().Consenters()

	msps, err := c.channelMSPs(support)
	if err != nil {
		return nil, errors.Wrap(err, "obtaining the channel MSPs")
	}
	set, err := newConsenterSet(consenters, msps)
	if err != nil {
		return nil, errors.Wrap(err, "reading the channel consenters")
	}

	selfID, err := set.findSelf(c.Identity)
	if err != nil {
		return nil, errors.Wrap(err, "without a system channel, a follower should have been created")
	}
	c.Logger.Infof("Channel %s: local consenter ID is %d, consenters %v", support.ChannelID(), selfID, set.IDs)

	chain, err := newChain(support, set, selfID, c.Comm, c.Logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating the BiniBFT chain")
	}

	c.Comm.Configure(support.ChannelID(), set.remoteNodes())
	if err := c.ClusterService.ConfigureNodeCerts(support.ChannelID(), consenters); err != nil {
		return nil, errors.Wrap(err, "configuring node certificates")
	}

	return chain, nil
}

// channelMSPs returns the MSPs of the channel as defined by its last config block.
func (c *Consenter) channelMSPs(support consensus.ConsenterSupport) (map[string]fabricmsp.MSP, error) {
	height := support.Height()
	if height == 0 {
		return nil, errors.New("ledger is empty")
	}
	last := support.Block(height - 1)
	if last == nil {
		return nil, errors.Errorf("cannot read block %d", height-1)
	}
	configBlock, err := cluster.LastConfigBlock(last, support)
	if err != nil {
		return nil, errors.Wrap(err, "finding the last config block")
	}
	env, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return nil, err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(env, c.BCCSP)
	if err != nil {
		return nil, errors.Wrap(err, "building the channel bundle")
	}
	return bundle.MSPManager().GetMSPs()
}

// IsChannelMember reports whether the join block lists this orderer as a
// consenter, so `osnadmin channel join` joins it as a consenter, not a follower.
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

	// Both sides are PEM certificates and must be decoded the same way: comparing
	// a PEM against DER silently never matches.
	mine, err := publicKeyOfPEMCert(c.Identity)
	if err != nil {
		return false, errors.Wrap(err, "local identity")
	}
	for _, consenter := range oc.Consenters() {
		theirs, err := publicKeyOfPEMCert(consenter.Identity)
		if err != nil {
			c.Logger.Warnf("Consenter %d: unusable identity: %v", consenter.Id, err)
			continue
		}
		if bytes.Equal(mine, theirs) {
			return true, nil
		}
	}
	return false, nil
}

func publicKeyOfPEMCert(certPEM []byte) ([]byte, error) {
	sanitized, err := crypto.SanitizeX509Cert(certPEM)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(sanitized)
	if block == nil {
		return nil, errors.New("not a PEM certificate")
	}
	return cluster.ExtractPublicKeyFromCert(block.Bytes)
}

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
