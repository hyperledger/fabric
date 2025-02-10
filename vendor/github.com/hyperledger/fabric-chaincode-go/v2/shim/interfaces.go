// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package shim

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Chaincode interface must be implemented by all chaincodes. The fabric runs
// the transactions by calling these functions as specified.
type Chaincode interface {
	// Init is called during Instantiate transaction after the chaincode container
	// has been established for the first time, allowing the chaincode to
	// initialize its internal data
	Init(stub ChaincodeStubInterface) *peer.Response

	// Invoke is called to update or query the ledger in a proposal transaction.
	// Updated state variables are not committed to the ledger until the
	// transaction is committed.
	Invoke(stub ChaincodeStubInterface) *peer.Response
}

// ChaincodeStubInterface is used by deployable chaincode apps to access and
// modify their ledgers
type ChaincodeStubInterface interface {
	// GetArgs returns the arguments intended for the chaincode Init and Invoke
	// as an array of byte arrays.
	GetArgs() [][]byte

	// GetStringArgs returns the arguments intended for the chaincode Init and
	// Invoke as a string array. Only use GetStringArgs if the client passes
	// arguments intended to be used as strings.
	GetStringArgs() []string

	// GetFunctionAndParameters returns the first argument as the function
	// name and the rest of the arguments as parameters in a string array.
	// Only use GetFunctionAndParameters if the client passes arguments intended
	// to be used as strings.
	GetFunctionAndParameters() (string, []string)

	// GetArgsSlice returns the arguments intended for the chaincode Init and
	// Invoke as a byte array
	GetArgsSlice() ([]byte, error)

	// GetTxID returns the tx_id of the transaction proposal, which is unique per
	// transaction and per client. See
	// https://godoc.org/github.com/hyperledger/fabric-protos-go-apiv2/common#ChannelHeader
	// for further details.
	GetTxID() string

	// GetChannelID returns the channel the proposal is sent to for chaincode to process.
	// This would be the channel_id of the transaction proposal (see
	// https://godoc.org/github.com/hyperledger/fabric-protos-go-apiv2/common#ChannelHeader )
	// except where the chaincode is calling another on a different channel.
	GetChannelID() string

	// InvokeChaincode locally calls the specified chaincode `Invoke` using the
	// same transaction context; that is, chaincode calling chaincode doesn't
	// create a new transaction message.
	// If the called chaincode is on the same channel, it simply adds the called
	// chaincode read set and write set to the calling transaction.
	// If the called chaincode is on a different channel,
	// only the Response is returned to the calling chaincode; any PutState calls
	// from the called chaincode will not have any effect on the ledger; that is,
	// the called chaincode on a different channel will not have its read set
	// and write set applied to the transaction. Only the calling chaincode's
	// read set and write set will be applied to the transaction. Effectively
	// the called chaincode on a different channel is a `Query`, which does not
	// participate in state validation checks in subsequent commit phase.
	// If `channel` is empty, the caller's channel is assumed.
	InvokeChaincode(chaincodeName string, args [][]byte, channel string) *peer.Response

	// GetState returns the value of the specified `key` from the
	// ledger. Note that GetState doesn't read data from the writeset, which
	// has not been committed to the ledger. In other words, GetState doesn't
	// consider data modified by PutState that has not been committed.
	// If the key does not exist in the state database, (nil, nil) is returned.
	GetState(key string) ([]byte, error)

	// GetMultipleStates retrieves the values of the specified keys from the ledger.
	// It has similar semantics to the GetState function regarding read-your-own-writes behavior,
	// meaning that updates made by a previous PutState operation within the same chaincode
	// session are not visible to this function.
	// If no key is passed, the function will return (nil, nil).
	// GetMultipleStates returns values in the same order in which the keys are provided,
	// and any keys missing from the ledger return a nil.
	GetMultipleStates(keys ...string) ([][]byte, error)

	// PutState puts the specified `key` and `value` into the transaction's
	// writeset as a data-write proposal. PutState doesn't effect the ledger
	// until the transaction is validated and successfully committed.
	// Simple keys must not be an empty string and must not start with a
	// null character (0x00) in order to avoid range query collisions with
	// composite keys, which internally get prefixed with 0x00 as composite
	// key namespace. In addition, if using CouchDB, keys can only contain
	// valid UTF-8 strings and cannot begin with an underscore ("_").
	PutState(key string, value []byte) error

	// DelState records the specified `key` to be deleted in the writeset of
	// the transaction proposal. The `key` and its value will be deleted from
	// the ledger when the transaction is validated and successfully committed.
	DelState(key string) error

	// SetStateValidationParameter sets the key-level endorsement policy for `key`.
	SetStateValidationParameter(key string, ep []byte) error

	// GetStateValidationParameter retrieves the key-level endorsement policy
	// for `key`. Note that this will introduce a read dependency on `key` in
	// the transaction's readset.
	GetStateValidationParameter(key string) ([]byte, error)

	// GetStateByRange returns a range iterator over a set of keys in the
	// ledger. The iterator can be used to iterate over all keys
	// between the startKey (inclusive) and endKey (exclusive).
	// However, if the number of keys between startKey and endKey is greater than the
	// totalQueryLimit (defined in core.yaml), this iterator cannot be used
	// to fetch all keys (results will be capped by the totalQueryLimit).
	// The keys are returned by the iterator in lexical order. Note
	// that startKey and endKey can be empty string, which implies unbounded range
	// query on start or end.
	// Call Close() on the returned StateQueryIteratorInterface object when done.
	// The query is re-executed during validation phase to ensure result set
	// has not changed since transaction endorsement (phantom reads detected).
	GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error)

	// GetStateByRangeWithPagination returns a range iterator over a set of keys in the
	// ledger. The iterator can be used to fetch keys between the startKey (inclusive)
	// and endKey (exclusive).
	// When an empty string is passed as a value to the bookmark argument, the returned
	// iterator can be used to fetch the first `pageSize` keys between the startKey
	// (inclusive) and endKey (exclusive).
	// When the bookmark is a non-emptry string, the iterator can be used to fetch
	// the first `pageSize` keys between the bookmark (inclusive) and endKey (exclusive).
	// Note that only the bookmark present in a prior page of query results (ResponseMetadata)
	// can be used as a value to the bookmark argument. Otherwise, an empty string must
	// be passed as bookmark.
	// The keys are returned by the iterator in lexical order. Note
	// that startKey and endKey can be empty string, which implies unbounded range
	// query on start or end.
	// Call Close() on the returned StateQueryIteratorInterface object when done.
	// This call is only supported in a read only transaction.
	GetStateByRangeWithPagination(startKey, endKey string, pageSize int32,
		bookmark string) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error)

	// GetStateByPartialCompositeKey queries the state in the ledger based on
	// a given partial composite key. This function returns an iterator
	// which can be used to iterate over all composite keys whose prefix matches
	// the given partial composite key. However, if the number of matching composite
	// keys is greater than the totalQueryLimit (defined in core.yaml), this iterator
	// cannot be used to fetch all matching keys (results will be limited by the totalQueryLimit).
	// The `objectType` and attributes are expected to have only valid utf8 strings and
	// should not contain U+0000 (nil byte) and U+10FFFF (biggest and unallocated code point).
	// See related functions SplitCompositeKey and CreateCompositeKey.
	// Call Close() on the returned StateQueryIteratorInterface object when done.
	// The query is re-executed during validation phase to ensure result set
	// has not changed since transaction endorsement (phantom reads detected). This function should be used only for
	// a partial composite key. For a full composite key, an iter with empty response
	// would be returned.
	GetStateByPartialCompositeKey(objectType string, keys []string) (StateQueryIteratorInterface, error)

	// GetStateByPartialCompositeKeyWithPagination queries the state in the ledger based on
	// a given partial composite key. This function returns an iterator
	// which can be used to iterate over the composite keys whose
	// prefix matches the given partial composite key.
	// When an empty string is passed as a value to the bookmark argument, the returned
	// iterator can be used to fetch the first `pageSize` composite keys whose prefix
	// matches the given partial composite key.
	// When the bookmark is a non-emptry string, the iterator can be used to fetch
	// the first `pageSize` keys between the bookmark (inclusive) and the last matching
	// composite key.
	// Note that only the bookmark present in a prior page of query result (ResponseMetadata)
	// can be used as a value to the bookmark argument. Otherwise, an empty string must
	// be passed as bookmark.
	// The `objectType` and attributes are expected to have only valid utf8 strings
	// and should not contain U+0000 (nil byte) and U+10FFFF (biggest and unallocated
	// code point). See related functions SplitCompositeKey and CreateCompositeKey.
	// Call Close() on the returned StateQueryIteratorInterface object when done.
	// This call is only supported in a read only transaction. This function should be used only for
	// a partial composite key. For a full composite key, an iter with empty response
	// would be returned.
	GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string,
		pageSize int32, bookmark string) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error)

	// CreateCompositeKey combines the given `attributes` to form a composite
	// key. The objectType and attributes are expected to have only valid utf8
	// strings and should not contain U+0000 (nil byte) and U+10FFFF
	// (biggest and unallocated code point).
	// The resulting composite key can be used as the key in PutState().
	CreateCompositeKey(objectType string, attributes []string) (string, error)

	// SplitCompositeKey splits the specified key into attributes on which the
	// composite key was formed. Composite keys found during range queries
	// or partial composite key queries can therefore be split into their
	// composite parts.
	SplitCompositeKey(compositeKey string) (string, []string, error)

	// GetQueryResult performs a "rich" query against a state database. It is
	// only supported for state databases that support rich query,
	// e.g.CouchDB. The query string is in the native syntax
	// of the underlying state database. An iterator is returned
	// which can be used to iterate over all keys in the query result set.
	// However, if the number of keys in the query result set is greater than the
	// totalQueryLimit (defined in core.yaml), this iterator cannot be used
	// to fetch all keys in the query result set (results will be limited by
	// the totalQueryLimit).
	// The query is NOT re-executed during validation phase, phantom reads are
	// not detected. That is, other committed transactions may have added,
	// updated, or removed keys that impact the result set, and this would not
	// be detected at validation/commit time.  Applications susceptible to this
	// should therefore not use GetQueryResult as part of transactions that update
	// ledger, and should limit use to read-only chaincode operations.
	GetQueryResult(query string) (StateQueryIteratorInterface, error)

	// GetQueryResultWithPagination performs a "rich" query against a state database.
	// It is only supported for state databases that support rich query,
	// e.g., CouchDB. The query string is in the native syntax
	// of the underlying state database. An iterator is returned
	// which can be used to iterate over keys in the query result set.
	// When an empty string is passed as a value to the bookmark argument, the returned
	// iterator can be used to fetch the first `pageSize` of query results.
	// When the bookmark is a non-emptry string, the iterator can be used to fetch
	// the first `pageSize` keys between the bookmark and the last key in the query result.
	// Note that only the bookmark present in a prior page of query results (ResponseMetadata)
	// can be used as a value to the bookmark argument. Otherwise, an empty string
	// must be passed as bookmark.
	// This call is only supported in a read only transaction.
	GetQueryResultWithPagination(query string, pageSize int32,
		bookmark string) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error)

	// GetHistoryForKey returns a history of key values across time.
	// For each historic key update, the historic value and associated
	// transaction id and timestamp are returned. The timestamp is the
	// timestamp provided by the client in the proposal header.
	// GetHistoryForKey requires peer configuration
	// core.ledger.history.enableHistoryDatabase to be true.
	// The query is NOT re-executed during validation phase, phantom reads are
	// not detected. That is, other committed transactions may have updated
	// the key concurrently, impacting the result set, and this would not be
	// detected at validation/commit time. Applications susceptible to this
	// should therefore not use GetHistoryForKey as part of transactions that
	// update ledger, and should limit use to read-only chaincode operations.
	// Starting in Fabric v2.0, the GetHistoryForKey chaincode API
	// will return results from newest to oldest in terms of ordered transaction
	// height (block height and transaction height within block).
	// This will allow applications to efficiently iterate through the top results
	// to understand recent changes to a key.
	GetHistoryForKey(key string) (HistoryQueryIteratorInterface, error)

	// GetPrivateData returns the value of the specified `key` from the specified
	// `collection`. Note that GetPrivateData doesn't read data from the
	// private writeset, which has not been committed to the `collection`. In
	// other words, GetPrivateData doesn't consider data modified by PutPrivateData
	// that has not been committed.
	GetPrivateData(collection, key string) ([]byte, error)

	// GetMultiplePrivateData retrieves the values of the specified keys from the  specified collection.
	// It has similar semantics to the GetePrivateData function regarding read-your-own-writes behavior,
	// meaning that updates made by a previous PutPrivateData operation within the same chaincode
	// session are not visible to this function.
	// If no key is passed, the function will return (nil, nil).
	// GetMultiplePrivateData returns values in the same order in which the keys are provided,
	// and any keys missing from the ledger return a nil.
	GetMultiplePrivateData(collection string, keys ...string) ([][]byte, error)

	// GetPrivateDataHash returns the hash of the value of the specified `key` from the specified
	// `collection`
	GetPrivateDataHash(collection, key string) ([]byte, error)

	// PutPrivateData puts the specified `key` and `value` into the transaction's
	// private writeset. Note that only hash of the private writeset goes into the
	// transaction proposal response (which is sent to the client who issued the
	// transaction) and the actual private writeset gets temporarily stored in a
	// transient store. PutPrivateData doesn't effect the `collection` until the
	// transaction is validated and successfully committed. Simple keys must not
	// be an empty string and must not start with a null character (0x00) in order
	// to avoid range query collisions with composite keys, which internally get
	// prefixed with 0x00 as composite key namespace. In addition, if using
	// CouchDB, keys can only contain valid UTF-8 strings and cannot begin with an
	// an underscore ("_").
	PutPrivateData(collection string, key string, value []byte) error

	// DelPrivateData records the specified `key` to be deleted in the private writeset
	// of the transaction. Note that only hash of the private writeset goes into the
	// transaction proposal response (which is sent to the client who issued the
	// transaction) and the actual private writeset gets temporarily stored in a
	// transient store. The `key` and its value will be deleted from the collection
	// when the transaction is validated and successfully committed.
	DelPrivateData(collection, key string) error

	// PurgePrivateData records the specified `key` to be purged in the private writeset
	// of the transaction. Note that only hash of the private writeset goes into the
	// transaction proposal response (which is sent to the client who issued the
	// transaction) and the actual private writeset gets temporarily stored in a
	// transient store. The `key` and its value will be deleted from the collection
	// when the transaction is validated and successfully committed, and will
	// subsequently be completely removed from the private data store (that maintains
	// the historical versions of private writesets) as a background operation.
	PurgePrivateData(collection, key string) error

	// SetPrivateDataValidationParameter sets the key-level endorsement policy
	// for the private data specified by `key`.
	SetPrivateDataValidationParameter(collection, key string, ep []byte) error

	// GetPrivateDataValidationParameter retrieves the key-level endorsement
	// policy for the private data specified by `key`. Note that this introduces
	// a read dependency on `key` in the transaction's readset.
	GetPrivateDataValidationParameter(collection, key string) ([]byte, error)

	// GetPrivateDataByRange returns a range iterator over a set of keys in a
	// given private collection. The iterator can be used to iterate over all keys
	// between the startKey (inclusive) and endKey (exclusive).
	// The keys are returned by the iterator in lexical order. Note
	// that startKey and endKey can be empty string, which implies unbounded range
	// query on start or end.
	// Call Close() on the returned StateQueryIteratorInterface object when done.
	// The query is re-executed during validation phase to ensure result set
	// has not changed since transaction endorsement (phantom reads detected).
	GetPrivateDataByRange(collection, startKey, endKey string) (StateQueryIteratorInterface, error)

	// GetPrivateDataByPartialCompositeKey queries the state in a given private
	// collection based on a given partial composite key. This function returns
	// an iterator which can be used to iterate over all composite keys whose prefix
	// matches the given partial composite key. The `objectType` and attributes are
	// expected to have only valid utf8 strings and should not contain
	// U+0000 (nil byte) and U+10FFFF (biggest and unallocated code point).
	// See related functions SplitCompositeKey and CreateCompositeKey.
	// Call Close() on the returned StateQueryIteratorInterface object when done.
	// The query is re-executed during validation phase to ensure result set
	// has not changed since transaction endorsement (phantom reads detected). This function should be used only for
	// a partial composite key. For a full composite key, an iter with empty response
	// would be returned.
	GetPrivateDataByPartialCompositeKey(collection, objectType string, keys []string) (StateQueryIteratorInterface, error)

	// GetPrivateDataQueryResult performs a "rich" query against a given private
	// collection. It is only supported for state databases that support rich query,
	// e.g.CouchDB. The query string is in the native syntax
	// of the underlying state database. An iterator is returned
	// which can be used to iterate (next) over the query result set.
	// The query is NOT re-executed during validation phase, phantom reads are
	// not detected. That is, other committed transactions may have added,
	// updated, or removed keys that impact the result set, and this would not
	// be detected at validation/commit time.  Applications susceptible to this
	// should therefore not use GetPrivateDataQueryResult as part of transactions that update
	// ledger, and should limit use to read-only chaincode operations.
	GetPrivateDataQueryResult(collection, query string) (StateQueryIteratorInterface, error)

	// GetCreator returns `SignatureHeader.Creator` (e.g. an identity)
	// of the `SignedProposal`. This is the identity of the agent (or user)
	// submitting the transaction.
	GetCreator() ([]byte, error)

	// GetTransient returns the `ChaincodeProposalPayload.Transient` field.
	// It is a map that contains data (e.g. cryptographic material)
	// that might be used to implement some form of application-level
	// confidentiality. The contents of this field, as prescribed by
	// `ChaincodeProposalPayload`, are supposed to always
	// be omitted from the transaction and excluded from the ledger.
	GetTransient() (map[string][]byte, error)

	// GetBinding returns the transaction binding, which is used to enforce a
	// link between application data (like those stored in the transient field
	// above) to the proposal itself. This is useful to avoid possible replay
	// attacks.
	GetBinding() ([]byte, error)

	// GetDecorations returns additional data (if applicable) about the proposal
	// that originated from the peer. This data is set by the decorators of the
	// peer, which append or mutate the chaincode input passed to the chaincode.
	GetDecorations() map[string][]byte

	// GetSignedProposal returns the SignedProposal object, which contains all
	// data elements part of a transaction proposal.
	GetSignedProposal() (*peer.SignedProposal, error)

	// GetTxTimestamp returns the timestamp when the transaction was created. This
	// is taken from the transaction ChannelHeader, therefore it will indicate the
	// client's timestamp and will have the same value across all endorsers.
	GetTxTimestamp() (*timestamppb.Timestamp, error)

	// SetEvent allows the chaincode to set an event on the response to the
	// proposal to be included as part of a transaction. The event will be
	// available within the transaction in the committed block regardless of the
	// validity of the transaction.
	// Only a single event can be included in a transaction, and must originate
	// from the outer-most invoked chaincode in chaincode-to-chaincode scenarios.
	// The marshaled ChaincodeEvent will be available in the transaction's ChaincodeAction.events field.
	SetEvent(name string, payload []byte) error

	// StartWriteBatch enables a mode where all changes are not immediately forwarded to the peer,
	// but accumulate in the cache. The cache is sent in large batches either at the end of transaction
	// execution or after the FinishWriteBatch call.
	// IMPORTANT: in this mode, the expected order of transaction execution and expected errors can be changed.
	//
	// If write batching is not supported by the peer, this method has no effect
	// and writes to the ledger continue to be processed immediately.
	StartWriteBatch()

	// FinishWriteBatch sends accumulated changes in large batches to the peer
	// if StartWriteBatch has been called before it.
	//
	// If write batching is not supported by the peer or no write batch has been
	// started, this method has no effect and returns nil.
	FinishWriteBatch() error
}

// CommonIteratorInterface allows a chaincode to check whether any more result
// to be fetched from an iterator and close it when done.
type CommonIteratorInterface interface {
	// HasNext returns true if the range query iterator contains additional keys
	// and values.
	HasNext() bool

	// Close closes the iterator. This should be called when done
	// reading from the iterator to free up resources.
	Close() error
}

// StateQueryIteratorInterface allows a chaincode to iterate over a set of
// key/value pairs returned by range and execute query.
type StateQueryIteratorInterface interface {
	// Inherit HasNext() and Close()
	CommonIteratorInterface

	// Next returns the next key and value in the range and execute query iterator.
	Next() (*queryresult.KV, error)
}

// HistoryQueryIteratorInterface allows a chaincode to iterate over a set of
// key/value pairs returned by a history query.
type HistoryQueryIteratorInterface interface {
	// Inherit HasNext() and Close()
	CommonIteratorInterface

	// Next returns the next key and value in the history query iterator.
	Next() (*queryresult.KeyModification, error)
}
