/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mirbft

type Config struct {
	// Logger provides the logging functions.
	Logger Logger

	// BatchSize determines how large a batch may grow (in number of request)
	// before it is cut. (Note, batches may be cut earlier, so this is a max size).
	BatchSize uint32

	// HeartbeatTicks is the number of ticks before a heartbeat is emitted
	// by a leader.
	HeartbeatTicks uint32

	// SuspectTicks is the number of ticks a bucket may not progress before
	// the node suspects the epoch has gone bad.
	SuspectTicks uint32

	// NewEpochTimeoutTicks is the number of ticks a replica will wait until
	// it suspects the epoch leader has failed.  This value must be greater
	// than 1, as rebroadcast ticks are computed as half this value.
	NewEpochTimeoutTicks uint32

	// BufferSize is the total size of messages which can be held by the state
	// machine, pending application, for each node.  This is necessary because
	// there may be dependencies between messages (for instance, until a checkpoint
	// result is computed, watermarks cannot advance).  This should be set
	// to a minimum of a few MB.
	BufferSize uint32
}
