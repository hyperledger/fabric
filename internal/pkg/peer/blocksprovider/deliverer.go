/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"google.golang.org/grpc"
)

// LedgerInfo an adapter to provide the interface to query
// the ledger committer for current ledger height
//
//go:generate counterfeiter -o fake/ledger_info.go --fake-name LedgerInfo . LedgerInfo
type LedgerInfo interface {
	// LedgerHeight returns current local ledger height
	LedgerHeight() (uint64, error)
}

// GossipServiceAdapter serves to provide basic functionality
// required from gossip service by delivery service
//
//go:generate counterfeiter -o fake/gossip_service_adapter.go --fake-name GossipServiceAdapter . GossipServiceAdapter
type GossipServiceAdapter interface {
	// AddPayload adds payload to the local state sync buffer
	AddPayload(chainID string, payload *gossip.Payload) error

	// Gossip the message across the peers
	Gossip(msg *gossip.GossipMessage)
}

//go:generate counterfeiter -o fake/block_verifier.go --fake-name BlockVerifier . BlockVerifier
type BlockVerifier interface {
	VerifyBlock(channelID gossipcommon.ChannelID, blockNum uint64, block *common.Block) error

	// VerifyBlockAttestation does the same as VerifyBlock, except it assumes block.Data = nil. It therefore does not
	// compute the block.Data.Hash() and compare it to the block.Header.DataHash. This is used when the orderer
	// delivers a block with header & metadata only, as an attestation of block existence.
	VerifyBlockAttestation(channelID string, block *common.Block) error
}

//go:generate counterfeiter -o fake/orderer_connection_source.go --fake-name OrdererConnectionSource . OrdererConnectionSource
type OrdererConnectionSource interface {
	RandomEndpoint() (*orderers.Endpoint, error)
	Endpoints() []*orderers.Endpoint
}

//go:generate counterfeiter -o fake/dialer.go --fake-name Dialer . Dialer
type Dialer interface {
	Dial(address string, rootCerts [][]byte) (*grpc.ClientConn, error)
}

//go:generate counterfeiter -o fake/deliver_streamer.go --fake-name DeliverStreamer . DeliverStreamer
type DeliverStreamer interface {
	Deliver(context.Context, *grpc.ClientConn) (orderer.AtomicBroadcast_DeliverClient, error)
}

const backoffExponentBase = 1.2

// Deliverer the CFT implementation of the deliverservice.BlockDeliverer interface.
type Deliverer struct {
	ChannelID       string
	Gossip          GossipServiceAdapter
	Ledger          LedgerInfo
	BlockVerifier   BlockVerifier
	Dialer          Dialer
	Orderers        OrdererConnectionSource
	DoneC           chan struct{}
	Signer          identity.SignerSerializer
	DeliverStreamer DeliverStreamer
	Logger          *flogging.FabricLogger
	YieldLeadership bool

	BlockGossipDisabled bool
	MaxRetryDelay       time.Duration
	InitialRetryDelay   time.Duration
	MaxRetryDuration    time.Duration

	// TLSCertHash should be nil when TLS is not enabled
	TLSCertHash []byte // util.ComputeSHA256(b.credSupport.GetClientCertificate().Certificate[0])

	sleeper sleeper

	requester *DeliveryRequester

	mutex         sync.Mutex
	stopFlag      bool
	blockReceiver *BlockReceiver
}

func (d *Deliverer) Initialize() {
	d.requester = NewDeliveryRequester(
		d.ChannelID,
		d.Signer,
		d.TLSCertHash,
		d.Dialer,
		d.DeliverStreamer,
	)
}

// DeliverBlocks used to pull out blocks from the ordering service to distribute them across peers
func (d *Deliverer) DeliverBlocks() {
	failureCounter := 0
	totalDuration := time.Duration(0)

	// InitialRetryDelay * backoffExponentBase^n > MaxRetryDelay
	// backoffExponentBase^n > MaxRetryDelay / InitialRetryDelay
	// n * log(backoffExponentBase) > log(MaxRetryDelay / InitialRetryDelay)
	// n > log(MaxRetryDelay / InitialRetryDelay) / log(backoffExponentBase)
	maxFailures := int(math.Log(float64(d.MaxRetryDelay)/float64(d.InitialRetryDelay)) / math.Log(backoffExponentBase))
	for {
		select {
		case <-d.DoneC:
			return
		default:
		}

		if failureCounter > 0 {
			var sleepDuration time.Duration
			if failureCounter-1 > maxFailures {
				sleepDuration = d.MaxRetryDelay // configured from peer.deliveryclient.reConnectBackoffThreshold
			} else {
				sleepDuration = time.Duration(math.Pow(backoffExponentBase, float64(failureCounter-1))*100) * time.Millisecond
			}
			totalDuration += sleepDuration
			if totalDuration > d.MaxRetryDuration {
				if d.YieldLeadership {
					d.Logger.Warningf("attempted to retry block delivery for more than peer.deliveryclient.reconnectTotalTimeThreshold duration %v, giving up", d.MaxRetryDuration)
					return
				}
				d.Logger.Warningf("peer is a static leader, ignoring peer.deliveryclient.reconnectTotalTimeThreshold")
			}
			d.Logger.Warningf("Disconnected from ordering service. Attempt to re-connect in %v", sleepDuration)
			d.sleeper.Sleep(sleepDuration, d.DoneC)
		}

		ledgerHeight, err := d.Ledger.LedgerHeight()
		if err != nil {
			d.Logger.Error("Did not return ledger height, something is critically wrong", err)
			return
		}

		endpoint, err := d.Orderers.RandomEndpoint()
		if err != nil {
			d.Logger.Warningf("Could not connect to ordering service: could not get orderer endpoints: %s", err)
			failureCounter++
			continue
		}

		seekInfoEnv, err := d.requester.SeekInfoBlocksFrom(ledgerHeight)
		if err != nil {
			d.Logger.Error("Could not create a signed Deliver SeekInfo message, something is critically wrong", err)
			return
		}

		deliverClient, cancel, err := d.requester.Connect(seekInfoEnv, endpoint)
		if err != nil {
			d.Logger.Warningf("Could not connect to ordering service: %s", err)
			failureCounter++
			continue
		}

		d.mutex.Lock()
		blockReceiver := &BlockReceiver{
			channelID:           d.ChannelID,
			gossip:              d.Gossip,
			blockGossipDisabled: d.BlockGossipDisabled,
			blockVerifier:       d.BlockVerifier,
			deliverClient:       deliverClient,
			cancelSendFunc:      cancel,
			recvC:               make(chan *orderer.DeliverResponse),
			stopC:               make(chan struct{}),
			endpoint:            endpoint,
			logger:              d.Logger.With("orderer-address", endpoint.Address),
		}
		d.blockReceiver = blockReceiver
		d.mutex.Unlock()

		blockReceiver.Start() // starts an internal goroutine
		onSuccess := func(blockNum uint64) {
			failureCounter = 0
		}
		if err := blockReceiver.ProcessIncoming(onSuccess); err != nil {
			switch err.(type) {
			case *errRefreshEndpoint:
				// Don't count it as an error, we'll reconnect immediately.
			case *errStopping:
				// Don't count it as an error, it is a signal to stop.
			default:
				failureCounter++
			}
		}
	}
}

// Stop stops blocks delivery provider
func (d *Deliverer) Stop() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.stopFlag {
		d.Logger.Debugf("Already stopped")
		return
	}

	d.stopFlag = true
	close(d.DoneC)
	d.blockReceiver.Stop()
	d.Logger.Info("Stopped")
}

func (d *Deliverer) setSleeperFunc(sleepFunc func(duration time.Duration)) {
	d.sleeper.sleep = sleepFunc
}
