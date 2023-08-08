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

//go:generate counterfeiter -o fake/censorship_detector.go --fake-name CensorshipDetector . CensorshipDetector
type CensorshipDetector interface {
	Monitor()
	Stop()
	ErrorsChannel() <-chan error
}

//go:generate counterfeiter -o fake/censorship_detector_factory.go --fake-name CensorshipDetectorFactory . CensorshipDetectorFactory
type CensorshipDetectorFactory interface {
	Create(
		chainID string,
		verifier BlockVerifier,
		requester DeliverClientRequester,
		progressReporter BlockProgressReporter,
		fetchSources []*orderers.Endpoint,
		blockSourceIndex int,
		timeoutConf TimeoutConfig,
	) CensorshipDetector
}

//go:generate counterfeiter -o fake/duration_exceeded_handler.go --fake-name DurationExceededHandler . DurationExceededHandler
type DurationExceededHandler interface {
	DurationExceededHandler() (stopRetries bool)
}

// BFTDeliverer TODO this is a skeleton
type BFTDeliverer struct { // TODO
	ChannelID                 string
	BlockHandler              BlockHandler
	Ledger                    LedgerInfo
	BlockVerifier             BlockVerifier
	Dialer                    Dialer
	Orderers                  OrdererConnectionSource
	DoneC                     chan struct{}
	Signer                    identity.SignerSerializer
	DeliverStreamer           DeliverStreamer
	CensorshipDetectorFactory CensorshipDetectorFactory
	Logger                    *flogging.FabricLogger

	// The initial value of the actual retry interval, which is increased on every failed retry
	InitialRetryInterval time.Duration
	// The maximal value of the actual retry interval, which cannot increase beyond this value
	MaxRetryInterval time.Duration
	// If a certain header from a header receiver is in front of the block receiver for more that this time, a
	// censorship event is declared and the block source is changed.
	BlockCensorshipTimeout time.Duration
	// After this duration, the MaxRetryDurationExceededHandler is called to decide whether to keep trying
	MaxRetryDuration time.Duration
	// This function is called after MaxRetryDuration of failed retries to decide whether to keep trying
	MaxRetryDurationExceededHandler MaxRetryDurationExceededHandler

	// TLSCertHash should be nil when TLS is not enabled
	TLSCertHash []byte // util.ComputeSHA256(b.credSupport.GetClientCertificate().Certificate[0])

	sleeper sleeper

	requester *DeliveryRequester

	mutex                          sync.Mutex    // mutex protects the following fields
	stopFlag                       bool          // mark the Deliverer as stopped
	nextBlockNumber                uint64        // next block number
	lastBlockTime                  time.Time     // last block time
	lastBlockSourceIndex           int           // the source index of the last block we got, or -1
	fetchFailureCounter            int           // counts the number of consecutive failures to fetch a block
	fetchFailureTotalSleepDuration time.Duration // the cumulative sleep time from when fetchFailureCounter goes 0->1

	fetchSources     []*orderers.Endpoint
	fetchSourceIndex int
	fetchErrorsC     chan error

	blockReceiver     *BlockReceiver
	censorshipMonitor CensorshipDetector
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

func (d *BFTDeliverer) BlockProgress() (uint64, time.Time) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.nextBlockNumber == 0 {
		return 0, time.Time{}
	}

	return d.nextBlockNumber - 1, d.lastBlockTime
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

	d.Logger.Infof("Starting to DeliverBlocks on channel `%s`, block height=%d", d.ChannelID, d.nextBlockNumber)
	defer func() {
		d.Logger.Infof("Stopping to DeliverBlocks on channel `%s`, block height=%d", d.ChannelID, d.nextBlockNumber)
	}()

	timeoutConfig := TimeoutConfig{
		MinRetryInterval:       d.InitialRetryInterval,
		MaxRetryInterval:       d.MaxRetryInterval,
		BlockCensorshipTimeout: d.BlockCensorshipTimeout,
	}

	// Refresh and randomize the sources, selects a random initial source, and incurs a random iteration order.
	d.refreshSources()

FetchAndMonitorLoop:
	for {
		// The backoff duration is doubled with every failed round.
		// A failed round is when we had moved through all the endpoints without success.
		// If we get a block successfully from a source, or endpoints are refreshed, the failure count is reset.
		failureCounter, failureTotalSleepDuration := d.getFetchFailureStats()
		if failureCounter > 0 {
			rounds := uint(failureCounter)
			if l := len(d.fetchSources); l > 0 {
				rounds = uint(failureCounter / l)
			}

			if failureTotalSleepDuration > d.MaxRetryDuration {
				if d.MaxRetryDurationExceededHandler() {
					d.Logger.Warningf("Attempted to retry block delivery for more than MaxRetryDuration (%s), giving up", d.MaxRetryDuration)
					break FetchAndMonitorLoop
				}
				d.Logger.Debugf("Attempted to retry block delivery for more than MaxRetryDuration (%s), but handler decided to continue retrying", d.MaxRetryDuration)
			}

			dur := backOffDuration(2.0, rounds, d.InitialRetryInterval, d.MaxRetryInterval)
			d.Logger.Warningf("Failed to fetch blocks, count=%d, round=%d, going to retry in %s", failureCounter, rounds, dur)
			d.sleeper.Sleep(dur, d.DoneC)
			d.addFetchFailureSleepDuration(dur)
		}

		// No endpoints is a non-recoverable error, as new endpoints are a result of fetching new blocks from an orderer.
		if len(d.fetchSources) == 0 {
			d.Logger.Error("Failure in DeliverBlocks, no orderer endpoints, something is critically wrong")
			break FetchAndMonitorLoop
		}
		// start a block fetcher and a monitor
		// a buffered channel so that the fetcher goroutine can send an error and exit w/o waiting for it to be consumed.
		d.fetchErrorsC = make(chan error, 1)
		source := d.fetchSources[d.fetchSourceIndex]
		go d.FetchBlocks(source)

		// create and start a censorship monitor
		d.censorshipMonitor = d.CensorshipDetectorFactory.Create(
			d.ChannelID, d.BlockVerifier, d.requester, d, d.fetchSources, d.fetchSourceIndex, timeoutConfig)
		go d.censorshipMonitor.Monitor()

		// wait for fetch  errors, censorship suspicions, or a stop signal.
		select {
		case <-d.DoneC:
			break FetchAndMonitorLoop
		case errFetch := <-d.fetchErrorsC:
			d.Logger.Debugf("Error received from fetchErrorsC channel: %s", errFetch)

			switch errFetch.(type) {
			case *ErrStopping:
				// nothing to do
				break FetchAndMonitorLoop
			case *errRefreshEndpoint:
				// get new endpoints and reassign fetcher and monitor
				d.refreshSources()
				d.resetFetchFailureCounter()
			case *ErrFatal:
				d.Logger.Errorf("Failure in FetchBlocks, something is critically wrong: %s", errFetch)
				break FetchAndMonitorLoop
			default:
				d.fetchSourceIndex = (d.fetchSourceIndex + 1) % len(d.fetchSources)
				d.incFetchFailureCounter()
			}
		case errMonitor := <-d.censorshipMonitor.ErrorsChannel():
			d.Logger.Debugf("Error received from censorshipMonitor.ErrorsChannel: %s", errMonitor)

			switch errMonitor.(type) {
			case *ErrStopping:
				// nothing to do
				break FetchAndMonitorLoop
			case *ErrCensorship:
				d.Logger.Warningf("Censorship suspicion: %s", errMonitor)
				d.mutex.Lock()
				d.blockReceiver.Stop()
				d.mutex.Unlock()
				d.fetchSourceIndex = (d.fetchSourceIndex + 1) % len(d.fetchSources)
				d.incFetchFailureCounter()
			case *ErrFatal:
				d.Logger.Errorf("Failure in CensorshipMonitor, something is critically wrong: %s", errMonitor)
				break FetchAndMonitorLoop
			default:
				d.Logger.Errorf("Unexpected error from CensorshipMonitor, something is critically wrong: %s", errMonitor)
				break FetchAndMonitorLoop
			}
		}

		d.censorshipMonitor.Stop()
	}

	// clean up everything because we are closing
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.blockReceiver.Stop()
	if d.censorshipMonitor != nil {
		d.censorshipMonitor.Stop()
	}
}

func (d *BFTDeliverer) refreshSources() {
	// select an initial source randomly
	d.fetchSources = d.Orderers.ShuffledEndpoints()
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
			d.fetchErrorsC <- &ErrStopping{Message: "stopping"}
			return
		default:
		}

		seekInfoEnv, err := d.requester.SeekInfoBlocksFrom(d.getNextBlockNumber())
		if err != nil {
			d.Logger.Errorf("Could not create a signed Deliver SeekInfo message, something is critically wrong: %s", err)
			d.fetchErrorsC <- &ErrFatal{Message: fmt.Sprintf("could not create a signed Deliver SeekInfo message: %s", err)}
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
			logger:         flogging.MustGetLogger("BlockReceiver").With("orderer-address", source.Address),
		}

		d.mutex.Lock()
		d.blockReceiver = blockRcv
		d.mutex.Unlock()

		// Starts a goroutine that receives blocks from the stream client and places them in the `recvC` channel
		blockRcv.Start()

		// Consume blocks fom the `recvC` channel
		if errProc := blockRcv.ProcessIncoming(d.onBlockProcessingSuccess); errProc != nil {
			switch errProc.(type) {
			case *ErrStopping:
				// nothing to do
				d.Logger.Debugf("BlockReceiver stopped while processing incoming blocks: %s", errProc)
			case *errRefreshEndpoint:
				d.Logger.Infof("Endpoint refreshed while processing incoming blocks: %s", errProc)
				d.fetchErrorsC <- errProc
			default:
				d.Logger.Warningf("Failure while processing incoming blocks: %s", errProc)
				d.fetchErrorsC <- errProc
			}

			return
		}
	}
}

func (d *BFTDeliverer) onBlockProcessingSuccess(blockNum uint64) {
	d.Logger.Debugf("blockNum: %d", blockNum)

	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fetchFailureCounter = 0
	d.fetchFailureTotalSleepDuration = 0

	d.nextBlockNumber = blockNum + 1
	d.lastBlockTime = time.Now()
}

func (d *BFTDeliverer) resetFetchFailureCounter() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fetchFailureCounter = 0
	d.fetchFailureTotalSleepDuration = 0
}

func (d *BFTDeliverer) getFetchFailureStats() (int, time.Duration) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.fetchFailureCounter, d.fetchFailureTotalSleepDuration
}

func (d *BFTDeliverer) incFetchFailureCounter() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fetchFailureCounter++
}

func (d *BFTDeliverer) addFetchFailureSleepDuration(dur time.Duration) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.fetchFailureTotalSleepDuration += dur
}

func (d *BFTDeliverer) getNextBlockNumber() uint64 {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.nextBlockNumber
}

func (d *BFTDeliverer) setSleeperFunc(sleepFunc func(duration time.Duration)) {
	d.sleeper.sleep = sleepFunc
}
