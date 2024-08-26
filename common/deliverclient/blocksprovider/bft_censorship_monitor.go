/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"container/list"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

// BlockProgressReporter provides information on the last block fetched from an orderer.
//
//go:generate counterfeiter -o fake/block_progress_reporter.go --fake-name BlockProgressReporter . BlockProgressReporter
type BlockProgressReporter interface {
	// BlockProgress returns the last block fetched from an orderer, and the time it was fetched.
	// If the fetch time IsZero == true, no block had been fetched yet (block number will always be zero in that case).
	BlockProgress() (uint64, time.Time)
}

// DeliverClientRequester connects to an orderer, requests a stream of blocks or headers, and provides a deliver client.
//
//go:generate counterfeiter -o fake/deliver_client_requester.go --fake-name DeliverClientRequester . DeliverClientRequester
type DeliverClientRequester interface {
	SeekInfoHeadersFrom(ledgerHeight uint64) (*common.Envelope, error)
	Connect(seekInfoEnv *common.Envelope, endpoint *orderers.Endpoint) (orderer.AtomicBroadcast_DeliverClient, func(), error)
}

// BFTCensorshipMonitor monitors the progress of headers receivers versus the progress of the block receiver.
// We ask for a stream of headers from all sources except the one supplying blocks.
// We track the progress of header receivers against the block reception progress.
// If there is a header that is ahead of the last block, and a timeout had passed since that header was received, we
// declare that censorship was detected.
// When censorship is detected, ErrCensorship is sent to the errorCh which can be read by ErrorsChannel() method.
type BFTCensorshipMonitor struct {
	chainID                 string
	updatableHeaderVerifier UpdatableBlockVerifier
	requester               DeliverClientRequester
	fetchSources            []*orderers.Endpoint
	blockSourceIndex        int

	timeoutConfig        TimeoutConfig
	stopHistoryWindowDur time.Duration

	progressReporter     BlockProgressReporter // Provides the last block number and time
	suspicion            bool                  // If a suspicion is pending
	suspicionTime        time.Time             // The reception time of the header that is ahead of the block receiver
	suspicionBlockNumber uint64                // The block number of the header that is ahead of the block receiver

	mutex          sync.Mutex
	stopFlag       bool
	stopCh         chan struct{}
	errorCh        chan error
	hdrRcvTrackers map[string]*headerReceiverTracker

	logger *flogging.FabricLogger
}

const logTimeFormat = "2006-01-02T15:04:05.000"

type headerReceiverTracker struct {
	headerReceiver        *BFTHeaderReceiver
	connectFailureCounter uint       // the number of consecutive unsuccessful attempts to connect to an orderer
	stopTimes             *list.List // the sequence of times a receiver had stopped due to Recv() or verification errors
	retryDeadline         time.Time  // do not try to connect & restart before the deadline
}

func (tracker *headerReceiverTracker) pruneOlderThan(u time.Time) {
	if tracker.stopTimes == nil {
		tracker.stopTimes = list.New()
		return
	}

	for e := tracker.stopTimes.Front(); e != nil; e = e.Next() {
		if s := e.Value.(time.Time); s.Before(u) {
			tracker.stopTimes.Remove(e)
		} else {
			break
		}
	}
}

func (tracker *headerReceiverTracker) appendIfNewer(u time.Time) bool {
	if tracker.stopTimes == nil {
		tracker.stopTimes = list.New()
	}

	if tracker.stopTimes.Len() == 0 {
		tracker.stopTimes.PushBack(u)
		return true
	}

	if e := tracker.stopTimes.Back().Value.(time.Time); e.Before(u) {
		tracker.stopTimes.PushBack(u)
		return true
	}

	return false
}

func NewBFTCensorshipMonitor(
	chainID string,
	updatableVerifier UpdatableBlockVerifier,
	requester DeliverClientRequester,
	progressReporter BlockProgressReporter,
	fetchSources []*orderers.Endpoint,
	blockSourceIndex int,
	timeoutConf TimeoutConfig,
) *BFTCensorshipMonitor {
	timeoutConf.ApplyDefaults()
	// This window is calculated such that if a receiver continuously fails when the retry interval was scaled up to
	// MaxRetryInterval, the retry interval will stay at MaxRetryInterval.
	stopWindowDur := time.Duration(int64(numRetries2Max(2.0, timeoutConf.MinRetryInterval, timeoutConf.MaxRetryInterval)+1) * timeoutConf.MaxRetryInterval.Nanoseconds())

	m := &BFTCensorshipMonitor{
		chainID:                 chainID,
		updatableHeaderVerifier: updatableVerifier,
		requester:               requester,
		fetchSources:            fetchSources,
		progressReporter:        progressReporter,
		stopCh:                  make(chan struct{}),
		errorCh:                 make(chan error, 1), // Buffered to allow the Monitor() goroutine to exit without waiting
		hdrRcvTrackers:          make(map[string]*headerReceiverTracker),
		blockSourceIndex:        blockSourceIndex,
		timeoutConfig:           timeoutConf,
		stopHistoryWindowDur:    stopWindowDur,
		logger:                  flogging.MustGetLogger("BFTCensorshipMonitor").With("channel", chainID),
	}

	return m
}

// Monitor the progress of headers and compare to the progress of block fetching, trying to detect block censorship.
// Continuously try and relaunch the goroutines that monitor individual orderers. If an orderer is faulty we increase
// the interval between retries but never quit.
//
// This method should be run using a dedicated goroutine.
func (m *BFTCensorshipMonitor) Monitor() {
	m.logger.Debug("Starting to monitor block and header fetching progress")
	defer func() {
		m.logger.Debug("Stopping to monitor block and header fetching progress")
	}()

	for i, ep := range m.fetchSources {
		if i == m.blockSourceIndex {
			continue
		}
		m.hdrRcvTrackers[ep.Address] = &headerReceiverTracker{}
	}

	for {
		if err := m.launchHeaderReceivers(); err != nil {
			m.logger.Warningf("Failure while launching header receivers: %s", err)
			m.errorCh <- &ErrFatal{Message: err.Error()}
			m.Stop()
			return
		}

		select {
		case <-m.stopCh:
			m.errorCh <- &ErrStopping{Message: "received a stop signal"}
			return
		case <-time.After(m.timeoutConfig.BlockCensorshipTimeout / 100):
			if m.detectBlockCensorship() {
				m.logger.Warningf("Block censorship detected, block source endpoint: %s", m.fetchSources[m.blockSourceIndex])
				m.errorCh <- &ErrCensorship{Message: fmt.Sprintf("block censorship detected, endpoint: %s", m.fetchSources[m.blockSourceIndex])}
				m.Stop()
				return
			}
		}
	}
}

func (m *BFTCensorshipMonitor) ErrorsChannel() <-chan error {
	return m.errorCh
}

func (m *BFTCensorshipMonitor) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.stopFlag {
		return
	}

	m.stopFlag = true
	close(m.stopCh)
	for _, hRcvMon := range m.hdrRcvTrackers {
		if hRcvMon.headerReceiver != nil {
			_ = hRcvMon.headerReceiver.Stop()
		}
	}
}

func (m *BFTCensorshipMonitor) detectBlockCensorship() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	lastBlockNumber, lastBlockTime := m.progressReporter.BlockProgress()

	// When there is a suspicion, we're waiting for one of two things:
	// - either a new block arrives, with a number >= than the header that triggered the suspicion,
	//   in which case we remove the suspicion, or
	// - the timeout elapses, in which case the suspicion is reported as a censorship event.
	if m.suspicion {
		if !lastBlockTime.IsZero() && lastBlockNumber >= m.suspicionBlockNumber {
			m.logger.Debugf("[%s] last block number: %d >= than suspicion header number: %d; suspicion disproved", m.chainID, lastBlockNumber, m.suspicionBlockNumber)
			m.suspicion = false
			m.suspicionBlockNumber = 0
			m.suspicionTime = time.Time{}
		} else if now.After(m.suspicionTime.Add(m.timeoutConfig.BlockCensorshipTimeout)) {
			m.logger.Warningf("[%s] block censorship timeout (%s) expired; suspicion time: %s; last block number: %d < than suspicion header number: %d; censorship event detected",
				m.chainID, m.timeoutConfig.BlockCensorshipTimeout, m.suspicionTime.Format(logTimeFormat), lastBlockNumber, m.suspicionBlockNumber)
			m.suspicion = false
			m.suspicionBlockNumber = 0
			m.suspicionTime = time.Time{}
			return true
		}
	}

	if m.suspicion {
		// When there is a pending suspicion, advancing headers cannot disprove it, only a new block.
		// Thus, there is no point in checking the progress of headers now.
		m.logger.Debugf("[%s] suspicion pending: block number: %d, header number: %d, header time: %s; timeout expires in: %s",
			m.chainID, lastBlockNumber, m.suspicionBlockNumber, m.suspicionTime.Format(logTimeFormat), m.suspicionTime.Add(m.timeoutConfig.BlockCensorshipTimeout).Sub(now))
		return false
	}

	var ahead []timeNumber
	for ep, hRcvMon := range m.hdrRcvTrackers {
		if hRcvMon.headerReceiver == nil {
			m.logger.Debugf("[%s] header receiver: %s is nil, skipping", m.chainID, ep)
			continue
		}
		headerNum, headerTime, err := hRcvMon.headerReceiver.LastBlockNum()
		if err != nil {
			m.logger.Debugf("[%s] header receiver: %s, error getting last block number, skipping; err: %s", m.chainID, ep, err)
			continue
		}
		if (!lastBlockTime.IsZero() && headerNum > lastBlockNumber) || lastBlockTime.IsZero() {
			m.logger.Debugf("[%s] header receiver: %s, is ahead of block receiver; header num=%d, time=%s; block num=%d, time=%s",
				m.chainID, ep, headerNum, headerTime.Format(logTimeFormat), lastBlockNumber, lastBlockTime.Format(logTimeFormat))
			ahead = append(ahead, timeNumber{
				t: headerTime,
				n: headerNum,
			})
		}
	}

	if len(ahead) == 0 {
		return false
	}

	if m.logger.IsEnabledFor(zapcore.DebugLevel) {
		var b strings.Builder
		for _, tn := range ahead {
			b.WriteString(fmt.Sprintf("(t: %s, n: %d); ", tn.t.Format(logTimeFormat), tn.n))
		}
		m.logger.Debugf("[%s] %d header receivers are ahead of block receiver, out of %d endpoints; ahead: %s", m.chainID, len(ahead), len(m.fetchSources), b.String())
	}

	firstAhead := ahead[0]
	for _, tn := range ahead {
		if tn.t.Before(firstAhead.t) {
			firstAhead = tn
		}
	}

	m.suspicion = true
	m.suspicionTime = firstAhead.t
	m.suspicionBlockNumber = firstAhead.n

	m.logger.Debugf("[%s] block censorship suspicion triggered, header time=%s, number=%d; last block time=%s, number=%d",
		m.chainID, m.suspicionTime.Format(logTimeFormat), m.suspicionBlockNumber, lastBlockTime.Format(logTimeFormat), lastBlockNumber)

	return false
}

// GetSuspicion returns the suspicion flag, and the header block number that is ahead.
// If suspicion==false, then suspicionBlockNumber==0.
//
// Used mainly for testing.
func (m *BFTCensorshipMonitor) GetSuspicion() (bool, uint64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.suspicion, m.suspicionBlockNumber
}

func (m *BFTCensorshipMonitor) launchHeaderReceivers() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	numEP := len(m.fetchSources)
	if numEP <= 0 {
		return errors.New("no endpoints")
	}

	hRcvToCreate := make([]*orderers.Endpoint, 0)
	now := time.Now()
	for i, ep := range m.fetchSources {
		if i == m.blockSourceIndex {
			continue // skip the block source
		}

		hRcvMon := m.hdrRcvTrackers[ep.Address]
		// Create a header receiver to sources that
		// - don't have a running receiver already, and
		// - don't have a retry deadline in the future
		if hRcvMon.headerReceiver != nil {
			if !hRcvMon.headerReceiver.IsStopped() {
				m.logger.Debugf("[%s] Header receiver to: %s, is running", m.chainID, ep.Address)
				continue
			}

			// When a receiver stops, we retry to restart. If there are repeated failures, we use an increasing
			// retry interval. The retry interval is exponential in the number of failures in a certain time window,
			// but is not greater that a maximum interval.
			if hRcvMon.headerReceiver.IsStarted() && hRcvMon.headerReceiver.IsStopped() {
				// Prune all failure times older than the stop history window.
				// If it is a new failure event, add a restart deadline in the future.
				hRcvMon.pruneOlderThan(now.Add(-m.stopHistoryWindowDur))
				stopTime := hRcvMon.headerReceiver.GetErrorStopTime()
				if added := hRcvMon.appendIfNewer(stopTime); added {
					dur := backOffDuration(2.0, uint(hRcvMon.stopTimes.Len()-1), m.timeoutConfig.MinRetryInterval, m.timeoutConfig.MaxRetryInterval)
					m.logger.Debugf("[%s] Header receiver to: %s, had stopped, (%s), retry in %s", m.chainID, ep.Address, stopTime.Format(logTimeFormat), dur)
					hRcvMon.retryDeadline = now.Add(dur)
				}
			}
		}

		if hRcvMon.retryDeadline.After(now) {
			m.logger.Debugf("[%s] Headers receiver to: %s, has a retry deadline in the future, retry in %s", m.chainID, ep.Address, hRcvMon.retryDeadline.Sub(now))
			continue
		}

		hRcvToCreate = append(hRcvToCreate, ep)
	}

	m.logger.Debugf("[%s] Going to create %d header receivers: %+v", m.chainID, len(hRcvToCreate), hRcvToCreate)

	for _, ep := range hRcvToCreate {
		hrRcvMon := m.hdrRcvTrackers[ep.Address]
		// This may fail if the orderer is down or faulty. If it fails, we back off and retry.
		// We count connection failure attempts (here) and stop failures separately.
		headerClient, _, err := m.newHeaderClient(ep, hrRcvMon.headerReceiver) // TODO use the clientCloser function
		if err != nil {
			dur := backOffDuration(2.0, hrRcvMon.connectFailureCounter, m.timeoutConfig.MinRetryInterval, m.timeoutConfig.MaxRetryInterval)
			hrRcvMon.retryDeadline = time.Now().Add(dur)
			hrRcvMon.connectFailureCounter++
			m.logger.Debugf("[%s] Failed to create a header receiver to: %s, failure no. %d, will retry in %s", m.chainID, ep.Address, hrRcvMon.connectFailureCounter, dur)
			continue
		}

		hrRcvMon.headerReceiver = NewBFTHeaderReceiver(m.chainID, ep.Address, headerClient, m.updatableHeaderVerifier, hrRcvMon.headerReceiver, flogging.MustGetLogger("BFTHeaderReceiver"))
		hrRcvMon.connectFailureCounter = 0
		hrRcvMon.retryDeadline = time.Time{}

		m.logger.Debugf("[%s] Created a header receiver to: %s", m.chainID, ep.Address)
		go hrRcvMon.headerReceiver.DeliverHeaders()
		m.logger.Debugf("[%s] Launched a header receiver to: %s", m.chainID, ep.Address)
	}

	m.logger.Debugf("Exit: number of endpoints: %d", numEP)
	return nil
}

// newHeaderClient connects to the orderer's delivery service and requests a stream of headers.
// Seek from the largest of the block progress and the last good header from the previous header receiver.
func (m *BFTCensorshipMonitor) newHeaderClient(endpoint *orderers.Endpoint, prevHeaderReceiver *BFTHeaderReceiver) (deliverClient orderer.AtomicBroadcast_DeliverClient, clientCloser func(), err error) {
	blockNumber, blockTime := m.progressReporter.BlockProgress()
	if !blockTime.IsZero() {
		blockNumber++ // If blockTime.IsZero(), we request block number 0, else blockNumber+1
	}

	if prevHeaderReceiver != nil {
		hNum, _, errH := prevHeaderReceiver.LastBlockNum()
		if errH == nil && (hNum+1) > blockNumber {
			blockNumber = hNum + 1
		}
	}

	seekInfoEnv, err := m.requester.SeekInfoHeadersFrom(blockNumber)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not create a signed Deliver SeekInfo message, something is critically wrong")
	}

	deliverClient, clientCloser, err = m.requester.Connect(seekInfoEnv, endpoint)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not connect to ordering service")
	}

	return deliverClient, clientCloser, nil
}
