/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tx

import (
	"fmt"
	"io"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/pkg/statedata"
)

// ProcessorCreator creates a new instance of a processor of a particular transaction type.
// In addition, this function also returns zero or more simulated readwrite sets that may
// be present in the transaction if the transaction type supports enclosing of these.
// There is expected to be one to one mapping between a supported transaction type and the
// corresponding implementation of this interface. The transaction envelope passed to the
// function `NewProcessor` is guaranteed to be of the associated transaction type.
// If the ProcessCreator finds the transaction envelop to be invalid the err returned by
// this function should be of type `InvalidErr`
type ProcessorCreator interface {
	NewProcessor(txenv *Envelope) (processor Processor, simulatedRWSet [][]byte, err error)
}

// Processor contains logic for processing a transaction on the committing peer.
// One instance of a Processor is created for each transaction via `NewProcessor` function on the
// corresponding `ProcessorCreator`.
//
// On a Processor, the first a `Preprocess` function is invoked and then the `Process` function is invoked.
// The `Preprocess` function is invoked exactly once however, this may be invoked in parallel on different instances of Processors.
// The function `Process` is invoked one by one on the `Processors` in the order in which the associated transactions
// appear in the block. The `State` passed to the `Process` function represents the world state for the channel as of preceding valid
// transaction in the block. For the purpose of efficiency (e.g., speculative execution), this function can be invoked more
// than once and hence this function should not preserve anything in the form of instance variables.
// Eventually, the function `Done` is invoked when the `Processor` has been used so as to indicate that the `Processor` can
// release any resources held. This invocation could be because of a successful invocation to the `Process` function or
// because the associated transaction is found to be invalid at any stage during the process
// (e.g., concurrency conflict with other transactions). If the `Processor` finds the transaction to be invalid at any stage,
// the error returned by the functions `PreProcess` and `Process` should be of type `InvalidErr`
//
// The intent is to support different transaction types via interface Processor such as pure endorser transactions,
// pure post-order transactions, and a mixed transaction - e.g., a transaction that combines an endorser transaction and
// a post-order transaction (say, a token transaction).
//
// Below is the detail description of the semantics of the function `Process`
// In order to process a transaction on a committing peer, we first evaluate the simulated readwrite set of the transaction
// (returned by the function `NewProcessor` on the corresponding `ProcessorCreator`).
// If the simulated part is found to have a concurrency conflict with one or more preceding valid transactions
// (either a preceding transaction in the same block or in a preceding block), we mark the transaction invalid.
// However, if simulated part of the transaction is found to be conflict free,
// this is assumed that the transaction has logically started executing during commit time and has produced the
// writes present in the simulated part of the transaction. In this case, the transaction processing is
// continued from this point on and the `Process` function gives a chance to the `Processor` to complete the
// transaction processing. Via the `Process` function, the transaction processor can perform reads/writes to the
// state passed to this function.
//
// Following is an illustration how the Processor is potentially expected be implemented for the transaction type "ENDORSER_TRANSACTION".
//  1. Preprocess function - Verifies the signatures and keeps the identities in internal state
//  2. Process function - Reads and evaluates endorsement policies that are applicable to the transactions writes.
//     The parameter "proposedWrites" to the Process function, contains the data items are intended writes by the processing of
//     the simulatedRWSet. The endorser transaction processor can load the applicable endorsement policies (such as chaincode or key-based)
//     and returns an error of type InvalidErr if the endorsement policy is not satisfied.
type Processor interface {
	Preprocess(latestChannelConfig *ChannelConfig) error
	Process(state *State, proposedWrites *statedata.ProposedWrites) error
	Done()
}

// InvalidErr is intended to be used by a ProcessorCreator or a Processor to indicate that the transaction is found to be invalid
type InvalidErr struct {
	ActualErr      error
	ValidationCode peer.TxValidationCode
}

func (e *InvalidErr) msgWithoutStack() string {
	return fmt.Sprintf("ValidationCode = %s, ActualErr = %s", e.ValidationCode.String(), e.ActualErr)
}

func (e *InvalidErr) msgWithStack() string {
	return fmt.Sprintf("ValidationCode = %s, ActualErr = %+v", e.ValidationCode.String(), e.ActualErr)
}

func (e *InvalidErr) Error() string {
	return e.msgWithoutStack()
}

// Format implements interface fmt.Formatter
func (e *InvalidErr) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			io.WriteString(s, e.msgWithStack())
			return
		}
		fallthrough
	case 's':
		io.WriteString(s, e.msgWithoutStack())
	case 'q':
		fmt.Fprintf(s, "%q", e.msgWithoutStack())
	}
}

// ReadHinter is an optional interface that a `Processor` implementation is encouraged to
// implement if the `Processor` can give any hints about what data items it would potentially read
// during its processing. This helps in pre-fetching/bulkloading the data in order to boost the performance
// For instance, the `Processor` implementation for the endorser transactions is expected to give hint
// about the endorsement policies based on the chaincode/collection/keys present in the "potentialWrites" parameter
// (which in turn are derived from the simulated readwrite set present in the transaction envelope)
// If a `Processor` implements this interface, the function `ReadHint` also gets the same treatment as the function `PreProcess`
// i.e., this is invoked before invoking function `Process` and exactly once and may be invoked in parallel on different
// instances of Processors. Note that the Preprocess and ReadHint functions on a Processor can get invoked in parallel
type ReadHinter interface {
	ReadHint(potentialWrites *statedata.WriteHint) *statedata.ReadHint
}

// Reprocessor is an optional interface that a `Processor` is encouraged to implement if a
// significant large number of transactions of the corresponding type are expected to be present and
// validation of the transaction is significantly resource consuming (e.g., signature matching/crypto operations)
// as compare to manipulating the state.
// The main context in which the function in this interface is to be invoked is to rebuild the ledger constructs such as
// statedb and historydb from the blocks that has already been processed in the past.
// For instance, if the statedb is dropped and it is to be rebuilt, fabric will only use function in this interface (if implemented)
// instead of the function in the Processor interface.
// The function in this interface can safely assume that only transaction that are processed using this
// were found to be valid earlier (when Processor interface was used for processing the transaction for the first time)
// and hence, this function can skip any validation step and can just manipulate the state. However, if there is
// no significant difference in the resource consumption, there is no value in implementing this interface and
// the regular functions in the Processor interface would be invoked
type Reprocessor interface {
	Reprocess(state *State, latestChannelConfig *ChannelConfig, proposedWrites *statedata.ProposedWrites)
}

// ReprocessReadHinter is an optional interface that a `Processor` may choose to implement if it implements Reprocessor.
// This is similar to as a processor may implement the ReadHinter interface albeit this gets invoked only if Reprocessor
// is used for processing the transaction
type ReprocessReadHinter interface {
	ReprocessReadHint(potentialWrites *statedata.WriteHint) *statedata.ReadHint
}

// PvtdataSourceHinter is an optional interface that a `Processor` implements to return the peers
// (identity bytes, e.g., certs) that could be the potential source for the private data associated
// with the transaction
type PvtdataSourceHinter interface {
	PvtdataSource() [][]byte
}

// ChannelConfig gives handle to the channel config
type ChannelConfig struct{}

// State exposes functions that helps a `Processor` in retrieving the latest state
// The `State` passed to the `Process` function represents the world state for the channel as of
// commit of the preceding valid transaction in the block
type State struct {
	BackingState state
}

// GetState returns value associated with a tuple <namespace, key>
func (s *State) GetState(ns, key string) ([]byte, error) {
	return s.BackingState.GetState(ns, key)
}

// GetStateMetadata returns a map containing the metadata associated with a tuple <namespace, key>
func (s *State) GetStateMetadata(ns, key string) (map[string][]byte, error) {
	return s.BackingState.GetStateMetadata(ns, key)
}

// GetStateRangeScanIterator returns an iterator that can be used to iterate over all the keys
// present in the range startKey (inclusive) and endKey (exclusive) for the a namespace.
func (s *State) GetStateRangeScanIterator(ns, startKey, endKey string) (*KeyValueItr, error) {
	itr, err := s.BackingState.GetStateRangeScanIterator(ns, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &KeyValueItr{
		BackingItr: itr,
	}, nil
}

// SetState sets the value associated with a tuple <namespace, key>
// A nil value implies the delete of the key
func (s *State) SetState(ns, key string, value []byte) error {
	return s.BackingState.SetState(ns, key, value)
}

// SetStateMetadata sets the metadata associated with a tuple <namespace, key> for an existing key
// This function is a noop for a non existing key. A nil metadata implied the delete of the metadata
func (s *State) SetStateMetadata(ns, key string, metadata map[string][]byte) error {
	return s.BackingState.SetStateMetadata(ns, key, metadata)
}

// GetPrivateDataMetadataByHash returns the metadata associated with a tuple <namespace, collection, keyhash>
func (s *State) GetPrivateDataMetadataByHash(ns, coll string, keyHash []byte) (map[string][]byte, error) {
	return s.BackingState.GetPrivateDataMetadataByHash(ns, coll, keyHash)
}

// KeyValueItr helps iterates over the results of a range scan query
type KeyValueItr struct {
	BackingItr keyValueItr
}

// Next returns the next result. A nil in the return value implies that no more results are available
func (i *KeyValueItr) Next() (*statedata.KeyValue, error) {
	return i.BackingItr.Next()
}

// Close closes the iterator
func (i *KeyValueItr) Close() {
	i.BackingItr.Close()
}

// Envelope contains data of the common.Envelope; some byte fields are already
// unmarshalled to structs and we preserve the unmarshalled version so as to not
// duplicate the unmarshalling work. Still, given the non-deterministic nature of
// protobufs, we preserve their original byte representation so that the tx processor
// may for instance verify signatures or perform bitwise operations on their original
// representation.
type Envelope struct {
	// SignedBytes contains the marshalled common.Payload in the envelope
	SignedBytes []byte
	// Signature contains the creator's signature over the SignedBytes
	Signature []byte
	// Data contains the opaque Data bytes in the common.Payload
	Data []byte
	// ChannelHeaderBytes contains the marshalled ChannelHeader of the common.Header
	ChannelHeaderBytes []byte
	// ChannelHeaderBytes contains the marshalled SignatureHeader of the common.Header
	SignatureHeaderBytes []byte
	// ChannelHeader contains the ChannelHeader of this envelope
	ChannelHeader *common.ChannelHeader
	// SignatureHeader contains the SignatureHeader of this envelope
	SignatureHeader *common.SignatureHeader
}

// **********************    Unexported types ***********************************************//
// state represents the latest state that is passed to the "Process" function of the TxProcessor
type state interface {
	GetState(ns, key string) ([]byte, error)
	GetStateMetadata(ns, key string) (map[string][]byte, error)
	GetStateRangeScanIterator(ns, startKey, endKey string) (keyValueItr, error)
	SetState(ns, key string, value []byte) error
	SetStateMetadata(ns, key string, metadata map[string][]byte) error
	GetPrivateDataMetadataByHash(ns, coll string, keyHash []byte) (map[string][]byte, error)
}

// keyValueItr iterates over a range of key-values
type keyValueItr interface {
	Next() (*statedata.KeyValue, error)
	Close()
}
