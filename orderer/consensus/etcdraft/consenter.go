/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"reflect"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name ChainGetter -case underscore -output mocks

// ChainGetter obtains instances of ChainSupport for the given channel
type ChainGetter interface {
	// GetChain obtains the ChainSupport for the given channel.
	// Returns nil, false when the ChainSupport for the given channel
	// isn't found.
	GetChain(chainID string) *multichannel.ChainSupport
}

// Consenter implements etddraft consenter
type Consenter struct {
	Communication cluster.Communicator
	*Dispatcher
	Chains ChainGetter
	Logger *flogging.FabricLogger
	Cert   []byte
}

// TargetChannel extracts the channel from the given proto.Message.
// Returns an empty string on failure.
func (c *Consenter) TargetChannel(message proto.Message) string {
	switch req := message.(type) {
	case *orderer.StepRequest:
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
	cs := c.Chains.GetChain(channelID)
	if cs == nil {
		return nil
	}
	if cs.Chain == nil {
		c.Logger.Panicf("Programming error - Chain %s is nil although it exists in the mapping", channelID)
	}
	if etcdRaftChain, isEtcdRaftChain := cs.Chain.(*Chain); isEtcdRaftChain {
		return etcdRaftChain
	}
	c.Logger.Warningf("Chain %s is of type %v and not etcdraft.Chain", channelID, reflect.TypeOf(cs.Chain))
	return nil
}

func (c *Consenter) detectSelfID(m *etcdraft.Metadata) (uint64, error) {
	var serverCertificates []string
	for i, cst := range m.Consenters {
		serverCertificates = append(serverCertificates, string(cst.ServerTlsCert))
		if bytes.Equal(c.Cert, cst.ServerTlsCert) {
			return uint64(i + 1), nil
		}
	}

	c.Logger.Error("Could not find", string(c.Cert), "among", serverCertificates)
	return 0, errors.Errorf("failed to detect own Raft ID because no matching certificate found")
}

// HandleChain returns a new Chain instance or an error upon failure
func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	m := &etcdraft.Metadata{}
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consensus metadata")
	}

	if m.Options == nil {
		return nil, errors.New("etcdraft options have not been provided")
	}

	id, err := c.detectSelfID(m)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	peers := make([]raft.Peer, len(m.Consenters))
	for i := range peers {
		peers[i].ID = uint64(i + 1)
	}

	opts := Options{
		RaftID:  id,
		Clock:   clock.NewClock(),
		Storage: raft.NewMemoryStorage(),
		Logger:  c.Logger,

		TickInterval:    time.Duration(m.Options.TickInterval) * time.Millisecond,
		ElectionTick:    int(m.Options.ElectionTick),
		HeartbeatTick:   int(m.Options.HeartbeatTick),
		MaxInflightMsgs: int(m.Options.MaxInflightMsgs),
		MaxSizePerMsg:   m.Options.MaxSizePerMsg,

		Peers: peers,
	}

	rpc := &cluster.RPC{Channel: support.ChainID(), Comm: c.Communication}
	return NewChain(support, opts, c.Communication, rpc, nil)
}

func New(clusterDialer *cluster.PredicateDialer, conf *localconfig.TopLevel,
	srvConf comm.ServerConfig, srv *comm.GRPCServer, r *multichannel.Registrar) *Consenter {
	logger := flogging.MustGetLogger("orderer/consensus/etcdraft")
	consenter := &Consenter{
		Cert:   srvConf.SecOpts.Certificate,
		Logger: logger,
		Chains: r,
	}
	consenter.Dispatcher = &Dispatcher{
		Logger:        logger,
		ChainSelector: consenter,
	}

	comm := createComm(clusterDialer, conf, consenter)
	consenter.Communication = comm
	svc := &cluster.Service{
		Logger:     flogging.MustGetLogger("orderer/common/cluster"),
		Dispatcher: comm,
	}
	orderer.RegisterClusterServer(srv.Server(), svc)
	return consenter
}

func createComm(clusterDialer *cluster.PredicateDialer,
	conf *localconfig.TopLevel,
	c *Consenter) *cluster.Comm {
	comm := &cluster.Comm{
		Logger:       flogging.MustGetLogger("orderer/common/cluster"),
		Chan2Members: make(map[string]cluster.MemberMapping),
		Connections:  cluster.NewConnectionStore(clusterDialer),
		RPCTimeout:   conf.General.Cluster.RPCTimeout,
		ChanExt:      c,
		H:            c,
	}
	c.Communication = comm
	return comm
}
