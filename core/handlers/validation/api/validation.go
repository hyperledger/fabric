/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import "github.com/hyperledger/fabric-protos-go-apiv2/common"

// Argument defines the argument for validation
type Argument interface {
	Dependency
	// Arg returns the bytes of the argument
	Arg() []byte
}

// Dependency marks a dependency passed to the Init() method
type Dependency any

// ContextDatum defines additional data that is passed from the validator
// into the Validate() invocation
type ContextDatum any

// Plugin validates transactions
type Plugin interface {
	// Validate returns nil if the action at the given position inside the transaction
	// at the given position in the given block is valid, or an error if not.
	Validate(block *common.Block, namespace string, txPosition int, actionPosition int, contextData ...ContextDatum) error

	// Init injects dependencies into the instance of the Plugin
	Init(dependencies ...Dependency) error
}

// PluginFactory creates a new instance of a Plugin
type PluginFactory interface {
	New() Plugin
}

// InvalidTxSignaler is an optional interface a Plugin may implement to be
// informed about transactions that were rejected before or during plugin
// dispatch, and therefore never produced a plugin validation result
type InvalidTxSignaler interface {
	// SignalInvalidTx signals that the transaction at height `blockNum, txNum`
	// was invalidated without a plugin validation result, so that bookkeeping
	// kept by the plugin (e.g. state-based endorsement dependencies) can
	// release transactions waiting on it. Implementations must tolerate the
	// same transaction being signaled more than once: a signal may be
	// delivered directly and then replayed to a lazily created plugin
	// instance, and repeated signals must not change the outcome
	SignalInvalidTx(blockNum, txNum uint64, err error)
}

// ExecutionFailureError indicates that the validation
// failed because of an execution problem, and thus
// the transaction validation status could not be computed
type ExecutionFailureError struct {
	Reason string
}

// Error conveys this is an error, and also contains
// the reason for the error
func (e *ExecutionFailureError) Error() string {
	return e.Reason
}
