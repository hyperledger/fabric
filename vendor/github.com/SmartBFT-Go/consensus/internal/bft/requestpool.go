// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/metrics/disabled"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
)

const (
	defaultRequestTimeout    = 10 * time.Second // for unit tests only
	defaultMaxBytes          = 100 * 1024       // default max request size would be of size 100Kb
	defaultSizeOfDelElements = 1000             // default size slice of delete elements
	defaultEraseTimeout      = 5 * time.Second  // for cicle erase silice of delete elements
)

var (
	ErrReqAlreadyExists    = fmt.Errorf("request already exists")
	ErrReqAlreadyProcessed = fmt.Errorf("request already processed")
	ErrRequestTooBig       = fmt.Errorf("submitted request is too big")
	ErrSubmitTimeout       = fmt.Errorf("timeout submitting to request pool")
)

//go:generate mockery -dir . -name RequestTimeoutHandler -case underscore -output ./mocks/

// RequestTimeoutHandler defines the methods called by request timeout timers created by time.AfterFunc.
// This interface is implemented by the bft.Controller.
type RequestTimeoutHandler interface {
	// OnRequestTimeout is called when a request timeout expires.
	OnRequestTimeout(request []byte, requestInfo types.RequestInfo)
	// OnLeaderFwdRequestTimeout is called when a leader forwarding timeout expires.
	OnLeaderFwdRequestTimeout(request []byte, requestInfo types.RequestInfo)
	// OnAutoRemoveTimeout is called when a auto-remove timeout expires.
	OnAutoRemoveTimeout(requestInfo types.RequestInfo)
}

// Pool implements requests pool, maintains pool of given size provided during
// construction. In case there are more incoming request than given size it will
// block during submit until there will be place to submit new ones.
type Pool struct {
	logger    api.Logger
	metrics   *MetricsRequestPool
	inspector api.RequestInspector
	options   PoolOptions

	cancel         context.CancelFunc
	lock           sync.RWMutex
	fifo           *list.List
	semaphore      *semaphore.Weighted
	existMap       map[types.RequestInfo]*list.Element
	timeoutHandler RequestTimeoutHandler
	closed         bool
	stopped        bool
	submittedChan  chan struct{}
	sizeBytes      uint64
	delMap         map[types.RequestInfo]struct{}
	delSlice       []types.RequestInfo
}

// requestItem captures request related information
type requestItem struct {
	request           []byte
	timeout           *time.Timer
	additionTimestamp time.Time
}

// PoolOptions is the pool configuration
type PoolOptions struct {
	QueueSize         int64
	ForwardTimeout    time.Duration
	ComplainTimeout   time.Duration
	AutoRemoveTimeout time.Duration
	RequestMaxBytes   uint64
	SubmitTimeout     time.Duration
	MetricsProvider   *api.CustomerProvider
}

// NewPool constructs new requests pool
func NewPool(log api.Logger, inspector api.RequestInspector, th RequestTimeoutHandler, options PoolOptions, submittedChan chan struct{}) *Pool {
	if options.ForwardTimeout == 0 {
		options.ForwardTimeout = defaultRequestTimeout
	}
	if options.ComplainTimeout == 0 {
		options.ComplainTimeout = defaultRequestTimeout
	}
	if options.AutoRemoveTimeout == 0 {
		options.AutoRemoveTimeout = defaultRequestTimeout
	}
	if options.RequestMaxBytes == 0 {
		options.RequestMaxBytes = defaultMaxBytes
	}
	if options.SubmitTimeout == 0 {
		options.SubmitTimeout = defaultRequestTimeout
	}
	if options.MetricsProvider == nil {
		options.MetricsProvider = api.NewCustomerProvider(&disabled.Provider{})
	}

	ctx, cancel := context.WithCancel(context.Background())

	rp := &Pool{
		cancel:         cancel,
		timeoutHandler: th,
		logger:         log,
		metrics:        NewMetricsRequestPool(options.MetricsProvider),
		inspector:      inspector,
		fifo:           list.New(),
		semaphore:      semaphore.NewWeighted(options.QueueSize),
		existMap:       make(map[types.RequestInfo]*list.Element),
		options:        options,
		submittedChan:  submittedChan,
		delMap:         make(map[types.RequestInfo]struct{}),
		delSlice:       make([]types.RequestInfo, 0, defaultSizeOfDelElements),
	}

	go func() {
		tic := time.NewTicker(defaultEraseTimeout)

		for {
			select {
			case <-tic.C:
				rp.eraseFromDelSlice()
			case <-ctx.Done():
				tic.Stop()

				return
			}
		}
	}()

	return rp
}

// ChangeTimeouts changes the timeout of the pool
func (rp *Pool) ChangeTimeouts(th RequestTimeoutHandler, options PoolOptions) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	if !rp.stopped {
		rp.logger.Errorf("Trying to change timeouts but the pool is not stopped")
		return
	}

	if options.ForwardTimeout == 0 {
		options.ForwardTimeout = defaultRequestTimeout
	}
	if options.ComplainTimeout == 0 {
		options.ComplainTimeout = defaultRequestTimeout
	}
	if options.AutoRemoveTimeout == 0 {
		options.AutoRemoveTimeout = defaultRequestTimeout
	}

	rp.options.ForwardTimeout = options.ForwardTimeout
	rp.options.ComplainTimeout = options.ComplainTimeout
	rp.options.AutoRemoveTimeout = options.AutoRemoveTimeout

	rp.timeoutHandler = th

	rp.logger.Debugf("Changed pool timeouts")
}

func (rp *Pool) isClosed() bool {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	return rp.closed
}

// Submit a request into the pool, returns an error when request is already in the pool
func (rp *Pool) Submit(request []byte) error {
	reqInfo := rp.inspector.RequestID(request)
	if rp.isClosed() {
		return errors.Errorf("pool closed, request rejected: %s", reqInfo)
	}

	if uint64(len(request)) > rp.options.RequestMaxBytes {
		rp.metrics.CountOfFailAddRequestToPool.With(
			rp.metrics.LabelsForWith(nameReasonFailAdd, reasonRequestMaxBytes)...,
		).Add(1)
		return fmt.Errorf(
			"submitted request (%d) is bigger than request max bytes (%d)",
			len(request),
			rp.options.RequestMaxBytes,
		)
	}

	rp.lock.RLock()
	_, alreadyExists := rp.existMap[reqInfo]
	_, alreadyDelete := rp.delMap[reqInfo]
	rp.lock.RUnlock()

	if alreadyExists {
		rp.logger.Debugf("request %s already exists in the pool", reqInfo)
		return ErrReqAlreadyExists
	}

	if alreadyDelete {
		rp.logger.Debugf("request %s already processed", reqInfo)
		return ErrReqAlreadyProcessed
	}

	ctx, cancel := context.WithTimeout(context.Background(), rp.options.SubmitTimeout)
	defer cancel()
	// do not wait for a semaphore with a lock, as it will prevent draining the pool.
	if err := rp.semaphore.Acquire(ctx, 1); err != nil {
		rp.metrics.CountOfFailAddRequestToPool.With(
			rp.metrics.LabelsForWith(nameReasonFailAdd, reasonSemaphoreAcquireFail)...,
		).Add(1)
		return errors.Wrapf(err, "acquiring semaphore for request: %s", reqInfo)
	}

	reqCopy := append(make([]byte, 0), request...)

	rp.lock.Lock()
	defer rp.lock.Unlock()

	if _, existsEl := rp.existMap[reqInfo]; existsEl {
		rp.semaphore.Release(1)
		rp.logger.Debugf("request %s has been already added to the pool", reqInfo)
		return ErrReqAlreadyExists
	}

	if _, deleteEl := rp.delMap[reqInfo]; deleteEl {
		rp.semaphore.Release(1)
		rp.logger.Debugf("request %s has been already processed", reqInfo)
		return ErrReqAlreadyProcessed
	}

	to := time.AfterFunc(
		rp.options.ForwardTimeout,
		func() { rp.onRequestTO(reqCopy, reqInfo) },
	)
	if rp.stopped {
		rp.logger.Debugf("pool stopped, submitting with a stopped timer, request: %s", reqInfo)
		to.Stop()
	}
	reqItem := &requestItem{
		request:           reqCopy,
		timeout:           to,
		additionTimestamp: time.Now(),
	}

	element := rp.fifo.PushBack(reqItem)
	rp.metrics.CountOfRequestPool.Add(1)
	rp.metrics.CountOfRequestPoolAll.Add(1)
	rp.existMap[reqInfo] = element

	if len(rp.existMap) != rp.fifo.Len() {
		rp.logger.Panicf("RequestPool map and list are of different length: map=%d, list=%d", len(rp.existMap), rp.fifo.Len())
	}

	rp.logger.Debugf("Request %s submitted; started a timeout: %s", reqInfo, rp.options.ForwardTimeout)

	// notify that a request was submitted
	select {
	case rp.submittedChan <- struct{}{}:
	default:
	}

	rp.sizeBytes += uint64(len(element.Value.(*requestItem).request))

	return nil
}

// Size returns the number of requests currently residing the pool
func (rp *Pool) Size() int {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	return len(rp.existMap)
}

// NextRequests returns the next requests to be batched.
// It returns at most maxCount requests, and at most maxSizeBytes, in a newly allocated slice.
// Return variable full indicates that the batch cannot be increased further by calling again with the same arguments.
func (rp *Pool) NextRequests(maxCount int, maxSizeBytes uint64, check bool) (batch [][]byte, full bool) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	if check {
		if (len(rp.existMap) < maxCount) && (rp.sizeBytes < maxSizeBytes) {
			return nil, false
		}
	}

	count := minInt(rp.fifo.Len(), maxCount)
	var totalSize uint64
	batch = make([][]byte, 0, count)
	element := rp.fifo.Front()
	for i := 0; i < count; i++ {
		req := element.Value.(*requestItem).request
		reqLen := uint64(len(req))
		if totalSize+reqLen > maxSizeBytes {
			rp.logger.Debugf("Returning batch of %d requests totalling %dB as it exceeds threshold of %dB",
				len(batch), totalSize, maxSizeBytes)
			return batch, true
		}
		batch = append(batch, req)
		totalSize += reqLen
		element = element.Next()
	}

	fullS := totalSize >= maxSizeBytes
	fullC := len(batch) == maxCount
	full = fullS || fullC
	if len(batch) > 0 {
		rp.logger.Debugf("Returning batch of %d requests totalling %dB",
			len(batch), totalSize)
	}
	return batch, full
}

// Prune removes requests for which the given predicate returns error.
func (rp *Pool) Prune(predicate func([]byte) error) {
	reqVec, infoVec := rp.copyRequests()

	var numPruned int
	for i, req := range reqVec {
		err := predicate(req)
		if err == nil {
			continue
		}

		if remErr := rp.RemoveRequest(infoVec[i]); remErr != nil {
			rp.logger.Debugf("Failed to prune request: %s; predicate error: %s; remove error: %s", infoVec[i], err, remErr)
		} else {
			rp.logger.Debugf("Pruned request: %s; predicate error: %s", infoVec[i], err)
			numPruned++
		}
	}

	rp.logger.Debugf("Pruned %d requests", numPruned)
}

func (rp *Pool) copyRequests() (requestVec [][]byte, infoVec []types.RequestInfo) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	requestVec = make([][]byte, len(rp.existMap))
	infoVec = make([]types.RequestInfo, len(rp.existMap))

	var i int
	for info, item := range rp.existMap {
		infoVec[i] = info
		requestVec[i] = item.Value.(*requestItem).request
		i++
	}

	return
}

// RemoveRequest removes the given request from the pool.
func (rp *Pool) RemoveRequest(requestInfo types.RequestInfo) error {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	element, exist := rp.existMap[requestInfo]
	if !exist {
		rp.moveToDelSlice(requestInfo)
		errStr := fmt.Sprintf("request %s is not in the pool at remove time", requestInfo)
		rp.logger.Debugf(errStr)
		return fmt.Errorf(errStr)
	}

	rp.deleteRequest(element, requestInfo)
	rp.sizeBytes -= uint64(len(element.Value.(*requestItem).request))
	return nil
}

func (rp *Pool) deleteRequest(element *list.Element, requestInfo types.RequestInfo) {
	item := element.Value.(*requestItem)
	item.timeout.Stop()

	rp.fifo.Remove(element)
	rp.metrics.CountOfRequestPool.Add(-1)
	rp.metrics.LatencyOfRequestPool.Observe(time.Since(item.additionTimestamp).Seconds())
	delete(rp.existMap, requestInfo)
	rp.moveToDelSlice(requestInfo)
	rp.logger.Infof("Removed request %s from request pool", requestInfo)
	rp.semaphore.Release(1)

	if len(rp.existMap) != rp.fifo.Len() {
		rp.logger.Panicf("RequestPool map and list are of different length: map=%d, list=%d", len(rp.existMap), rp.fifo.Len())
	}
}

func (rp *Pool) moveToDelSlice(requestInfo types.RequestInfo) {
	_, exist := rp.delMap[requestInfo]
	if exist {
		return
	}

	rp.delMap[requestInfo] = struct{}{}
	rp.delSlice = append(rp.delSlice, requestInfo)
}

func (rp *Pool) eraseFromDelSlice() {
	rp.lock.RLock()
	l := len(rp.delSlice)
	rp.lock.RUnlock()

	if l <= defaultSizeOfDelElements {
		return
	}

	rp.lock.Lock()
	defer rp.lock.Unlock()

	n := len(rp.delSlice) - defaultSizeOfDelElements

	for _, r := range rp.delSlice[:n] {
		delete(rp.delMap, r)
	}

	rp.delSlice = rp.delSlice[n:]
}

// Close removes all the requests, stops all the timeout timers.
func (rp *Pool) Close() {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	rp.closed = true

	for requestInfo, element := range rp.existMap {
		rp.deleteRequest(element, requestInfo)
	}

	rp.cancel()
}

// StopTimers stops all the timeout timers attached to the pending requests, and marks the pool as "stopped".
// This which prevents submission of new requests, and renewal of timeouts by timer go-routines that where running
// at the time of the call to StopTimers().
func (rp *Pool) StopTimers() {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	rp.stopped = true

	for _, element := range rp.existMap {
		item := element.Value.(*requestItem)
		item.timeout.Stop()
	}

	rp.logger.Debugf("Stopped all timers: size=%d", len(rp.existMap))
}

// RestartTimers restarts all the timeout timers attached to the pending requests, as RequestForwardTimeout, and re-allows
// submission of new requests.
func (rp *Pool) RestartTimers() {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	rp.stopped = false

	for reqInfo, element := range rp.existMap {
		item := element.Value.(*requestItem)
		item.timeout.Stop()
		ri := reqInfo
		to := time.AfterFunc(
			rp.options.ForwardTimeout,
			func() { rp.onRequestTO(item.request, ri) },
		)
		item.timeout = to
	}

	rp.logger.Debugf("Restarted all timers: size=%d", len(rp.existMap))
}

// called by the goroutine spawned by time.AfterFunc
func (rp *Pool) onRequestTO(request []byte, reqInfo types.RequestInfo) {
	rp.lock.Lock()

	element, contains := rp.existMap[reqInfo]
	if !contains {
		rp.lock.Unlock()
		rp.logger.Debugf("Request %s no longer in pool", reqInfo)
		return
	}

	if rp.closed || rp.stopped {
		rp.lock.Unlock()
		rp.logger.Debugf("Pool stopped, will NOT start a leader-forwarding timeout")
		return
	}

	// start a second timeout
	item := element.Value.(*requestItem)
	item.timeout = time.AfterFunc(
		rp.options.ComplainTimeout,
		func() { rp.onLeaderFwdRequestTO(request, reqInfo) },
	)
	rp.logger.Debugf("Request %s; started a leader-forwarding timeout: %s", reqInfo, rp.options.ComplainTimeout)

	rp.lock.Unlock()

	// may take time, in case Comm channel to leader is full; hence w/o the lock.
	rp.logger.Debugf("Request %s timeout expired, going to send to leader", reqInfo)
	rp.metrics.CountOfLeaderForwardRequest.Add(1)
	rp.timeoutHandler.OnRequestTimeout(request, reqInfo)
}

// called by the goroutine spawned by time.AfterFunc
func (rp *Pool) onLeaderFwdRequestTO(request []byte, reqInfo types.RequestInfo) {
	rp.lock.Lock()

	element, contains := rp.existMap[reqInfo]
	if !contains {
		rp.lock.Unlock()
		rp.logger.Debugf("Request %s no longer in pool", reqInfo)
		return
	}

	if rp.closed || rp.stopped {
		rp.lock.Unlock()
		rp.logger.Debugf("Pool stopped, will NOT start auto-remove timeout")
		return
	}

	// start a third timeout
	item := element.Value.(*requestItem)
	item.timeout = time.AfterFunc(
		rp.options.AutoRemoveTimeout,
		func() { rp.onAutoRemoveTO(reqInfo) },
	)
	rp.logger.Debugf("Request %s; started auto-remove timeout: %s", reqInfo, rp.options.AutoRemoveTimeout)

	rp.lock.Unlock()

	// may take time, in case Comm channel is full; hence w/o the lock.
	rp.logger.Debugf("Request %s leader-forwarding timeout expired, going to complain on leader", reqInfo)
	rp.metrics.CountTimeoutTwoStep.Add(1)
	rp.timeoutHandler.OnLeaderFwdRequestTimeout(request, reqInfo)
}

// called by the goroutine spawned by time.AfterFunc
func (rp *Pool) onAutoRemoveTO(reqInfo types.RequestInfo) {
	rp.logger.Debugf("Request %s auto-remove timeout expired, going to remove from pool", reqInfo)
	if err := rp.RemoveRequest(reqInfo); err != nil {
		rp.logger.Errorf("Removal of request %s failed; error: %s", reqInfo, err)
		return
	}
	rp.metrics.CountOfDeleteRequestPool.Add(1)
	rp.timeoutHandler.OnAutoRemoveTimeout(reqInfo)
}
