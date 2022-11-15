// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"time"

	"github.com/pkg/errors"
)

// Configuration defines the parameters needed in order to create an instance of Consensus.
type Configuration struct {
	// SelfID is the identifier of the node.
	SelfID uint64

	// RequestBatchMaxCount is the maximal number of requests in a batch.
	// A request batch that reaches this count is proposed immediately.
	RequestBatchMaxCount uint64
	// RequestBatchMaxBytes is the maximal total size of requests in a batch, in bytes.
	// This is also the maximal size of a request. A request batch that reaches this size is proposed immediately.
	RequestBatchMaxBytes uint64
	// RequestBatchMaxInterval is the maximal time interval a request batch is waiting before it is proposed.
	// A request batch is accumulating requests until RequestBatchMaxInterval had elapsed from the time the batch was
	// first created (i.e. the time the first request was added to it), or until it is of count RequestBatchMaxCount,
	// or total size RequestBatchMaxBytes, which ever happens first.
	RequestBatchMaxInterval time.Duration

	// IncomingMessageBufferSize is the size of the buffer holding incoming messages before they are processed.
	IncomingMessageBufferSize uint64
	// RequestPoolSize is the number of pending requests retained by the node.
	// The RequestPoolSize is recommended to be at least double (x2) the RequestBatchMaxCount.
	RequestPoolSize uint64

	// RequestForwardTimeout is started from the moment a request is submitted, and defines the interval after which a
	// request is forwarded to the leader.
	RequestForwardTimeout time.Duration
	// RequestComplainTimeout is started when RequestForwardTimeout expires, and defines the interval after which the
	// node complains about the view leader.
	RequestComplainTimeout time.Duration
	// RequestAutoRemoveTimeout is started when RequestComplainTimeout expires, and defines the interval after which
	// a request is removed (dropped) from the request pool.
	RequestAutoRemoveTimeout time.Duration

	// ViewChangeResendInterval defined the interval in which the ViewChange message is resent.
	ViewChangeResendInterval time.Duration
	// ViewChangeTimeout is started when a node first receives a quorum of ViewChange messages, and defines the
	// interval after which the node will try to initiate a view change with a higher view number.
	ViewChangeTimeout time.Duration

	// LeaderHeartbeatTimeout is the interval after which, if nodes do not receive a "sign of life" from the leader,
	// they complain on the current leader and try to initiate a view change. A sign of life is either a heartbeat
	// or a message from the leader.
	LeaderHeartbeatTimeout time.Duration
	// LeaderHeartbeatCount is the number of heartbeats per LeaderHeartbeatTimeout that the leader should emit.
	// The heartbeat-interval is equal to: LeaderHeartbeatTimeout/LeaderHeartbeatCount.
	LeaderHeartbeatCount uint64
	// NumOfTicksBehindBeforeSyncing is the number of follower ticks where the follower is behind the leader
	// by one sequence before starting a sync
	NumOfTicksBehindBeforeSyncing uint64

	// CollectTimeout is the interval after which the node stops listening to StateTransferResponse messages,
	// stops collecting information about view metadata from remote nodes.
	CollectTimeout time.Duration

	// SyncOnStart is a flag indicating whether a sync is required on startup.
	SyncOnStart bool

	// SpeedUpViewChange is a flag indicating whether a node waits for only f+1 view change messages to join
	// the view change (hence speeds up the view change process), or it waits for a quorum before joining.
	// Waiting only for f+1 is considered less safe.
	SpeedUpViewChange bool

	// LeaderRotation is a flag indicating whether leader rotation is active.
	LeaderRotation bool
	// DecisionsPerLeader is the number of decisions reached by a leader before there is a leader rotation.
	DecisionsPerLeader uint64

	// RequestMaxBytes total allowed size of the single request
	RequestMaxBytes uint64

	// RequestPoolSubmitTimeout the total amount of time client can wait for submission of single
	// request into request pool
	RequestPoolSubmitTimeout time.Duration
}

// DefaultConfig contains reasonable values for a small cluster that resides on the same geography (or "Region"), but
// possibly on different availability zones within the geography. It is assumed that the typical latency between nodes,
// and between clients to nodes, is approximately 10ms.
// Set the SelfID.
var DefaultConfig = Configuration{
	RequestBatchMaxCount:          100,
	RequestBatchMaxBytes:          10 * 1024 * 1024,
	RequestBatchMaxInterval:       50 * time.Millisecond,
	IncomingMessageBufferSize:     200,
	RequestPoolSize:               400,
	RequestForwardTimeout:         2 * time.Second,
	RequestComplainTimeout:        20 * time.Second,
	RequestAutoRemoveTimeout:      3 * time.Minute,
	ViewChangeResendInterval:      5 * time.Second,
	ViewChangeTimeout:             20 * time.Second,
	LeaderHeartbeatTimeout:        time.Minute,
	LeaderHeartbeatCount:          10,
	NumOfTicksBehindBeforeSyncing: 10,
	CollectTimeout:                time.Second,
	SyncOnStart:                   false,
	SpeedUpViewChange:             false,
	LeaderRotation:                true,
	DecisionsPerLeader:            3,
	RequestMaxBytes:               10 * 1024,
	RequestPoolSubmitTimeout:      5 * time.Second,
}

func (c Configuration) Validate() error {
	if !(c.SelfID > 0) {
		return errors.Errorf("SelfID is lower than or equal to zero")
	}

	if !(c.RequestBatchMaxCount > 0) {
		return errors.Errorf("RequestBatchMaxCount should be greater than zero")
	}
	if !(c.RequestBatchMaxBytes > 0) {
		return errors.Errorf("RequestBatchMaxBytes should be greater than zero")
	}
	if !(c.RequestBatchMaxInterval > 0) {
		return errors.Errorf("RequestBatchMaxInterval should be greater than zero")
	}
	if !(c.IncomingMessageBufferSize > 0) {
		return errors.Errorf("IncomingMessageBufferSize should be greater than zero")
	}
	if !(c.RequestPoolSize > 0) {
		return errors.Errorf("RequestPoolSize should be greater than zero")
	}
	if !(c.RequestForwardTimeout > 0) {
		return errors.Errorf("RequestForwardTimeout should be greater than zero")
	}
	if !(c.RequestComplainTimeout > 0) {
		return errors.Errorf("RequestComplainTimeout should be greater than zero")
	}
	if !(c.RequestAutoRemoveTimeout > 0) {
		return errors.Errorf("RequestAutoRemoveTimeout should be greater than zero")
	}
	if !(c.ViewChangeResendInterval > 0) {
		return errors.Errorf("ViewChangeResendInterval should be greater than zero")
	}
	if !(c.ViewChangeTimeout > 0) {
		return errors.Errorf("ViewChangeTimeout should be greater than zero")
	}
	if !(c.LeaderHeartbeatTimeout > 0) {
		return errors.Errorf("LeaderHeartbeatTimeout should be greater than zero")
	}
	if !(c.LeaderHeartbeatCount > 0) {
		return errors.Errorf("LeaderHeartbeatCount should be greater than zero")
	}
	if !(c.NumOfTicksBehindBeforeSyncing > 0) {
		return errors.Errorf("NumOfTicksBehindBeforeSyncing should be greater than zero")
	}
	if !(c.CollectTimeout > 0) {
		return errors.Errorf("CollectTimeout should be greater than zero")
	}
	if c.RequestBatchMaxCount > c.RequestBatchMaxBytes {
		return errors.Errorf("RequestBatchMaxCount is bigger than RequestBatchMaxBytes")
	}
	if c.RequestForwardTimeout > c.RequestComplainTimeout {
		return errors.Errorf("RequestForwardTimeout is bigger than RequestComplainTimeout")
	}
	if c.RequestComplainTimeout > c.RequestAutoRemoveTimeout {
		return errors.Errorf("RequestComplainTimeout is bigger than RequestAutoRemoveTimeout")
	}
	if c.ViewChangeResendInterval > c.ViewChangeTimeout {
		return errors.Errorf("ViewChangeResendInterval is bigger than ViewChangeTimeout")
	}
	if c.LeaderRotation && c.DecisionsPerLeader <= 0 {
		return errors.Errorf("DecisionsPerLeader should be greater than zero when leader rotation is active")
	}

	if !(c.RequestMaxBytes > 0) {
		return errors.Errorf("RequestMaxBytes should be greater than zero")
	}

	if !(c.RequestPoolSubmitTimeout > 0) {
		return errors.Errorf("RequestPoolSubmitTimeout should be greater than zero")
	}

	return nil
}
