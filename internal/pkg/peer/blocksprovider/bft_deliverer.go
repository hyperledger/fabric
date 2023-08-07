/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/pkg/errors"
)

// BFTDeliverer TODO this is a skeleton
type BFTDeliverer struct { // TODO
	ChannelID       string
	BlockHandler    BlockHandler
	Ledger          LedgerInfo
	BlockVerifier   BlockVerifier
	Dialer          Dialer
	Orderers        OrdererConnectionSource
	DoneC           chan struct{}
	Signer          identity.SignerSerializer
	DeliverStreamer DeliverStreamer
	Logger          *flogging.FabricLogger

	// The maximal value of the actual retry interval, which cannot increase beyond this value
	MaxRetryInterval time.Duration
	// The initial value of the actual retry interval, which is increased on every failed retry
	InitialRetryInterval time.Duration
	// After this duration, the MaxRetryDurationExceededHandler is called to decide whether to keep trying
	MaxRetryDuration time.Duration
	// This function is called after MaxRetryDuration of failed retries to decide whether to keep trying
	MaxRetryDurationExceededHandler MaxRetryDurationExceededHandler

	// TLSCertHash should be nil when TLS is not enabled
	TLSCertHash []byte // util.ComputeSHA256(b.credSupport.GetClientCertificate().Certificate[0])

	sleeper sleeper

	requester *DeliveryRequester

	mutex                sync.Mutex // mutex protects the following fields
	stopFlag             bool       // mark the Deliverer as stopped
	nextBlockNumber      uint64     // next block number
	lastBlockTime        time.Time  // last block time
	lastBlockSourceIndex int        // the source index of the last block we got, or -1
	fetchFailureCounter  int        // counts the number of consecutive failures to fetch a block

	fetchSources     []*orderers.Endpoint
	fetchSourceIndex int
	fetchErrorsC     chan error

	blockReceiver *BlockReceiver

	// TODO here we'll have a CensorshipMonitor component that detects block censorship.
	// When it suspects censorship, it will emit an error to this channel.
	monitorErrorsC chan error
}

func (d *BFTDeliverer) Initialize() {
	d.requester = NewDeliveryRequester(
		d.ChannelID,
		d.Signer,
		d.TLSCertHash,
		d.Dialer,
		d.DeliverStreamer,
	)
}

func (d *BFTDeliverer) DeliverBlocks() {
	var err error

	d.lastBlockSourceIndex = -1
	d.lastBlockTime = time.Now()
	d.nextBlockNumber, err = d.Ledger.LedgerHeight()
	if err != nil {
		d.Logger.Error("Did not return ledger height, something is critically wrong", err)
		return
	}
	d.Logger.Infof("Starting DeliverBlocks on channel `%s`, block height=%d", d.ChannelID, d.nextBlockNumber)

	// select an initial source randomly
	d.refreshSources()

FetchAndMonitorLoop:
	for {
		// The backoff duration is doubled with every failed round.
		// A failed round is when we had moved through all the endpoints without success.
		// If we get a block successfully from a source, or endpoints are refreshed, the failure count is reset.
		if count := d.getFetchFailureCounter(); count > 0 {
			rounds := uint(count)
			if l := len(d.fetchSources); l > 0 {
				rounds = uint(count / len(d.fetchSources))
			}

			dur := backOffDuration(2.0, rounds, BftMinRetryInterval, BftMaxRetryInterval)
			d.sleeper.Sleep(dur, d.DoneC)
		}

		// assign other endpoints to the monitor

		// start a block fetcher and a monitor
		// a buffered channel so that the fetcher goroutine can send an error and exit w/o waiting for it to be consumed.
		d.fetchErrorsC = make(chan error, 1)
		source := d.fetchSources[d.fetchSourceIndex]
		go d.FetchBlocks(source)

		// TODO start a censorship monitor

		// wait for fetch  errors, censorship suspicions, or a stop signal.
		select {
		case <-d.DoneC:
			break FetchAndMonitorLoop
		case errFetch := <-d.fetchErrorsC:
			if errFetch != nil {
				switch errFetch.(type) {
				case *errStopping:
					// nothing to do
				case *errRefreshEndpoint:
					// get new endpoints and reassign fetcher and monitor
					d.refreshSources()
					d.resetFetchFailureCounter()
				default:
					d.fetchSourceIndex = (d.fetchSourceIndex + 1) % len(d.fetchSources)
					d.incFetchFailureCounter()
				}
				// TODO can it be nil?
			}
		case errMonitor := <-d.monitorErrorsC:
			// TODO until we implement the censorship monitor this nil channel is blocked
			if errMonitor != nil {
				d.Logger.Warningf("Censorship suspicion: %s", err)
				// TODO close the block receiver, increment the index
				// TODO
			}
			// TODO can it be nil?
		}
	}

	// clean up everything because we are closing
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.blockReceiver.Stop()
	// TODO stop the monitor
}

func (d *BFTDeliverer) refreshSources() {
	// select an initial source randomly
	d.fetchSources = shuffle(d.Orderers.Endpoints())
	d.Logger.Infof("Refreshed endpoints: %s", d.fetchSources)
	d.fetchSourceIndex = 0
}

// Stop
func (d *BFTDeliverer) Stop() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.stopFlag {
		return
	}

	d.stopFlag = true
	close(d.DoneC)
	d.blockReceiver.Stop()
}

func (d *BFTDeliverer) FetchBlocks(source *orderers.Endpoint) {
	d.Logger.Debugf("Trying to fetch blocks from orderer: %s", source.Address)

	for {
		select {
		case <-d.DoneC:
			d.fetchErrorsC <- &errStopping{message: "stopping"}
			return
		default:
		}

		seekInfoEnv, err := d.requester.SeekInfoBlocksFrom(d.getNextBlockNumber())
		if err != nil {
			d.Logger.Errorf("Could not create a signed Deliver SeekInfo message, something is critically wrong: %s", err)
			d.fetchErrorsC <- &errFatal{message: fmt.Sprintf("could not create a signed Deliver SeekInfo message: %s", err)}
			return
		}

		deliverClient, cancel, err := d.requester.Connect(seekInfoEnv, source)
		if err != nil {
			d.Logger.Warningf("Could not connect to ordering service: %s", err)
			d.fetchErrorsC <- errors.Wrapf(err, "could not connect to ordering service, orderer-address: %s", source.Address)
			return
		}

		blockRcv := &BlockReceiver{
			channelID:      d.ChannelID,
			blockHandler:   d.BlockHandler,
			blockVerifier:  d.BlockVerifier,
			deliverClient:  deliverClient,
			cancelSendFunc: cancel,
			recvC:          make(chan *orderer.DeliverResponse),
			stopC:          make(chan struct{}),
			endpoint:       source,
			logger:         d.Logger.With("orderer-address", source.Address),
		}

		d.mutex.Lock()
		d.blockReceiver = blockRcv
		d.mutex.Unlock()

		blockRcv.Start()

		if err := blockRcv.ProcessIncoming(d.onBlockProcessingSuccess); err != nil {
			d.Logger.Warningf("failure while processing incoming blocks: %s", err)
			d.fetchErrorsC <- errors.Wrapf(err, "failure while processing incoming blocks, orderer-address: %s", source.Address)
			return
		}
	}
}

func (d *BFTDeliverer) onBlockProcessingSuccess(blockNum uint64) {
	d.mutex.Lock()
	d.fetchFailureCounter = 0
	d.nextBlockNumber = blockNum + 1
	d.lastBlockTime = time.Now()
	d.mutex.Unlock()
}

func (d *BFTDeliverer) resetFetchFailureCounter() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fetchFailureCounter = 0
}

func (d *BFTDeliverer) getFetchFailureCounter() int {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.fetchFailureCounter
}

func (d *BFTDeliverer) incFetchFailureCounter() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fetchFailureCounter++
}

func (d *BFTDeliverer) getNextBlockNumber() uint64 {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.nextBlockNumber
}

func (d *BFTDeliverer) setSleeperFunc(sleepFunc func(duration time.Duration)) {
	d.sleeper.sleep = sleepFunc
}
