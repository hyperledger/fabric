package engine

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrReqAlreadyExists    = fmt.Errorf("request already exists")
	ErrReqAlreadyProcessed = fmt.Errorf("request already processed")
	ErrRequestTooBig       = fmt.Errorf("submitted request is too big")
	ErrSubmitTimeout       = fmt.Errorf("timeout submitting to request pool")
)

// RequestPoolInterface defines the interface for request pools
type RequestPoolInterface interface {
	Submit(request []byte) error
	GetRequest(requestID string) (*Request, bool)
	RemoveRequest(requestID string) error
	UpdateRequestPhase(requestID string, phase RequestPhase)
	Size() int
	Close()
	GetRequestsByPhase(phase RequestPhase) []*Request
	NextRequests(maxCount int, maxSizeBytes uint64, check bool) (batch [][]byte, full bool)
}

type requestItem struct {
	request           []byte
	timeout           *time.Timer
	additionTimestamp time.Time
}

// RequestPool manages pending requests with enhanced features
type RequestPool struct {
	requests      map[string]*Request
	mu            sync.RWMutex
	maxSize       uint64
	timeouts      map[string]*time.Timer
	logger        Logger
	closed        bool
	fifo          *list.List
	inspector     RequestInspector
	existMap      map[RequestInfo]*list.Element
	delMap        map[RequestInfo]struct{}
	submittedChan chan struct{}
	sizeBytes     uint64
	network       NetworkInterface
	nodeID        NodeID
	role          NodeRole
	primaryLeader NodeID
}

// RequestPoolOptions for configuring the request pool
type RequestPoolOptions struct {
	MaxSize       uint64
	Logger        Logger
	insepctor     RequestInspector
	Inspector     RequestInspector
	submittedChan chan struct{}
	SubmittedChan chan struct{}
	Network       NetworkInterface
	NodeID        NodeID
	Role          NodeRole
	PrimaryLeader NodeID
}

// NewRequestPool creates a new request pool
func NewRequestPool() *RequestPool {
	return NewRequestPoolWithOptions(RequestPoolOptions{
		MaxSize: 10000, // Default max size
	})
}

// NewRequestPoolWithOptions creates a new request pool with options
func NewRequestPoolWithOptions(opts RequestPoolOptions) *RequestPool {
	if opts.MaxSize <= 0 {
		opts.MaxSize = 10000
	}

	inspector := opts.Inspector
	if inspector == nil {
		inspector = opts.insepctor
	}
	submittedChan := opts.SubmittedChan
	if submittedChan == nil {
		submittedChan = opts.submittedChan
	}

	return &RequestPool{
		requests:      make(map[string]*Request),
		maxSize:       opts.MaxSize,
		timeouts:      make(map[string]*time.Timer),
		logger:        opts.Logger,
		closed:        false,
		fifo:          list.New(),
		inspector:     inspector,
		submittedChan: submittedChan,
		network:       opts.Network,
		nodeID:        opts.NodeID,
		role:          opts.Role,
		primaryLeader: opts.PrimaryLeader,
		existMap:      make(map[RequestInfo]*list.Element),
		delMap:        make(map[RequestInfo]struct{}),
	}
}

// Submit adds a request to the pool or forwards it to the primary leader
func (rp *RequestPool) Submit(request []byte) error {
	if len(request) == 0 {
		return fmt.Errorf("cannot submit empty request")
	}

	var reqInfo RequestInfo
	if rp.inspector != nil {
		reqInfo = rp.inspector.RequestID(request)
	} else {
		h := sha256.Sum256(request)
		reqInfo = RequestInfo{
			ClientID: "default-client",
			ID:       fmt.Sprintf("%x", h),
		}
	}

	if reqInfo.ClientID == "" || reqInfo.ClientID == "invalid" || reqInfo.ClientID == "invalid-client" || reqInfo.ID == "" {
		return fmt.Errorf("rejected malformed request: invalid client or transaction ID")
	}

	if rp.isClosed() {
		return errors.Errorf("pool closed, request rejected: %s", reqInfo)
	}

	if uint64(len(request)) > rp.maxSize {
		return fmt.Errorf(
			"submitted request (%d) is bigger than request max bytes (%d)",
			len(request),
			rp.maxSize,
		)
	}

	// If this node is not the primary leader, forward the request to the primary leader
	if rp.role != RolePrimaryLeader && rp.network != nil && rp.primaryLeader != "" && rp.primaryLeader != rp.nodeID {
		rp.logInfo("Forwarding request to primary leader",
			"nodeID", rp.nodeID,
			"role", rp.role.String(),
			"primaryLeader", rp.primaryLeader,
			"reqInfo", reqInfo)

		return rp.network.SendTransaction(rp.primaryLeader, request)
	}

	rp.mu.Lock()
	defer rp.mu.Unlock()

	if uint64(len(rp.requests)) >= rp.maxSize {
		return fmt.Errorf(
			"submitted request (%d) is bigger than request max bytes (%d)",
			len(request),
			rp.maxSize,
		)
	}

	_, alreadyExists := rp.existMap[reqInfo]
	_, alreadyDelete := rp.delMap[reqInfo]

	if alreadyExists {
		rp.logDebug("request already exists in the pool", "reqInfo", reqInfo)
		return ErrReqAlreadyExists
	}

	if alreadyDelete {
		rp.logDebug("request %s already processed", "reqInfo", reqInfo)
		return ErrReqAlreadyProcessed
	}

	reqItem := &requestItem{
		request: request,
		// timeout:           to,
		additionTimestamp: time.Now(),
	}

	element := rp.fifo.PushBack(reqItem)
	rp.existMap[reqInfo] = element

	// Verify consistency after adding
	if len(rp.existMap) != rp.fifo.Len() {
		rp.logError("RequestPool map and list are of different length after adding",
			"map", len(rp.existMap),
			"list", rp.fifo.Len(),
			"reqInfo", reqInfo)
		// Try to fix the inconsistency by removing the element we just added
		rp.fifo.Remove(element)
		delete(rp.existMap, reqInfo)
		return fmt.Errorf("internal consistency error in request pool")
	}

	rp.logDebug("Request submitted to local pool", "reqInfo", reqInfo, "nodeID", rp.nodeID)

	// notify that a request was submitted
	if rp.submittedChan != nil {
		select {
		case rp.submittedChan <- struct{}{}:
		default:
		}
	}

	rp.sizeBytes += uint64(len(element.Value.(*requestItem).request))

	return nil
}

// GetRequest retrieves a request from the pool
func (rp *RequestPool) GetRequest(requestID string) (*Request, bool) {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	req, exists := rp.requests[requestID]
	return req, exists
}

// UpdateRequestPhase updates the phase of a request
func (rp *RequestPool) UpdateRequestPhase(requestID string, phase RequestPhase) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if req, exists := rp.requests[requestID]; exists {
		oldPhase := req.Phase
		req.Phase = phase
		req.Timestamp = time.Now()
		rp.logDebug("Updated request phase", "requestID", requestID, "oldPhase", oldPhase.String(), "newPhase", phase.String())
	}
}

// RemoveRequest removes a request from the pool
func (rp *RequestPool) RemoveRequest(requestID string) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if timer, exists := rp.timeouts[requestID]; exists {
		timer.Stop()
		delete(rp.timeouts, requestID)
	}

	if req, exists := rp.requests[requestID]; exists {
		rp.sizeBytes -= uint64(len(req.Data))
		delete(rp.requests, requestID)
		rp.logDebug("Removed request from pool", "requestID", requestID)
		return nil
	}

	return fmt.Errorf("request %s not found in pool", requestID)
}

// Size returns the number of pending requests in the pool
func (rp *RequestPool) Size() int {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	return rp.fifo.Len()
}

// SizeBytes returns the total size of pending requests in bytes
func (rp *RequestPool) SizeBytes() uint64 {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	return rp.sizeBytes
}

// Close closes the request pool and stops all timers
func (rp *RequestPool) Close() {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if rp.closed {
		return
	}

	rp.closed = true

	// Stop all timeout timers
	for _, timer := range rp.timeouts {
		timer.Stop()
	}
	rp.timeouts = make(map[string]*time.Timer)
	rp.logDebug("Request pool closed")
}

// GetRequestsByPhase returns all requests in a specific phase
func (rp *RequestPool) GetRequestsByPhase(phase RequestPhase) []*Request {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	var result []*Request
	for _, req := range rp.requests {
		if req.Phase == phase {
			result = append(result, req)
		}
	}
	return result
}

func (rp *RequestPool) NextRequests(maxCount int, maxSizeBytes uint64, check bool) (batch [][]byte, full bool) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	var totalSize uint64
	var elementsToRemove []*list.Element
	var requestInfosToRemove []RequestInfo

	element := rp.fifo.Front()
	count := rp.fifo.Len()

	for i := 0; i < count && element != nil; i++ {
		req := element.Value.(*requestItem).request
		reqLen := uint64(len(req))
		if totalSize+reqLen > maxSizeBytes {
			rp.logDebug(fmt.Sprintf("Returning batch of %d requests totalling %dB as it exceeds threshold of %dB",
				len(batch), totalSize, maxSizeBytes))
			break
		}
		batch = append(batch, req)
		totalSize += reqLen

		// If this is not just a check, mark element for removal
		if !check {
			elementsToRemove = append(elementsToRemove, element)
			reqInfo := rp.inspector.RequestID(req)
			requestInfosToRemove = append(requestInfosToRemove, reqInfo)
		}

		element = element.Next()
	}

	// Remove elements from FIFO and existMap if this was not just a check
	if !check {
		for i, elem := range elementsToRemove {
			rp.fifo.Remove(elem)
			rp.sizeBytes -= uint64(len(elem.Value.(*requestItem).request))

			// Also remove from existMap and add to delMap to maintain consistency
			reqInfo := requestInfosToRemove[i]
			delete(rp.existMap, reqInfo)
			rp.delMap[reqInfo] = struct{}{}
		}

		// Verify consistency after removal
		if len(rp.existMap) != rp.fifo.Len() {
			rp.logError("RequestPool map and list are of different length after removal",
				"map", len(rp.existMap),
				"list", rp.fifo.Len())
		}
	}

	fullS := totalSize >= maxSizeBytes
	fullC := len(batch) == maxCount
	full = fullS || fullC
	if len(batch) > 0 {
		rp.logDebug(fmt.Sprintf("Returning batch of %d requests totalling %dB",
			len(batch), totalSize))
	}
	return batch, full
}

func (rp *RequestPool) isClosed() bool {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	return rp.closed
}

func (rp *RequestPool) logInfo(msg string, fields ...interface{}) {
	if rp.logger != nil {
		rp.logger.Info(msg, fields...)
	}
}

func (rp *RequestPool) logDebug(msg string, fields ...interface{}) {
	if rp.logger != nil {
		rp.logger.Debug(msg, fields...)
	}
}

func (rp *RequestPool) logError(msg string, fields ...interface{}) {
	if rp.logger != nil {
		rp.logger.Error(msg, fields...)
	}
}
