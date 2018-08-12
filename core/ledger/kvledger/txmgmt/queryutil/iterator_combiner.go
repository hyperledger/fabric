/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryutil

import (
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
)

type itrCombiner struct {
	namespace string
	holders   []*itrHolder
}

func newItrCombiner(namespace string, baseIterators []statedb.ResultsIterator) (*itrCombiner, error) {
	var holders []*itrHolder
	for _, itr := range baseIterators {
		res, err := itr.Next()
		if err != nil {
			for _, holder := range holders {
				holder.itr.Close()
			}
			return nil, err
		}
		if res != nil {
			holders = append(holders, &itrHolder{itr, res.(*statedb.VersionedKV)})
		}
	}
	return &itrCombiner{namespace, holders}, nil
}

// Next returns the next eligible item from the underlying iterators.
// This function evaluates the underlying iterators, and picks the one which is
// gives the lexicographically smallest key. Then, it saves that value, and advances the chosen iterator.
// If the chosen iterator is out of elements, then that iterator is closed, and removed from the list of iterators.
func (combiner *itrCombiner) Next() (commonledger.QueryResult, error) {
	logger.Debugf("Iterators position at beginning: %s", combiner.holders)
	if len(combiner.holders) == 0 {
		return nil, nil
	}
	smallestHolderIndex := 0
	for i := 1; i < len(combiner.holders); i++ {
		smallestKey, holderKey := combiner.keyAt(smallestHolderIndex), combiner.keyAt(i)
		switch {
		case holderKey == smallestKey: // we found the same key in the lower order iterator (stale value of the key);
			// we already have the latest value for this key (in smallestHolder). Ignore this value and move the iterator
			// to next item (to a greater key) so that for next round of key selection, we do not consider this key again
			removed, err := combiner.moveItrAndRemoveIfExhausted(i)
			if err != nil {
				return nil, err
			}
			if removed { // if the current iterator is exhaused and hence removed, decrement the index
				// because indexes of the remaining iterators are decremented by one
				i--
			}
		case holderKey < smallestKey:
			smallestHolderIndex = i
		default:
			// the current key under evaluation is greater than the smallestKey - do nothing
		}
	}
	kv := combiner.kvAt(smallestHolderIndex)
	combiner.moveItrAndRemoveIfExhausted(smallestHolderIndex)
	if kv.IsDelete() {
		return combiner.Next()
	}
	logger.Debugf("Key [%s] selected from iterator at index [%d]", kv.Key, smallestHolderIndex)
	logger.Debugf("Iterators position at end: %s", combiner.holders)
	return &queryresult.KV{Namespace: combiner.namespace, Key: kv.Key, Value: kv.Value}, nil
}

// moveItrAndRemoveIfExhausted moves the iterator at index i to the next item. If the iterator gets exhausted
// then the iterator is removed from the underlying slice
func (combiner *itrCombiner) moveItrAndRemoveIfExhausted(i int) (removed bool, err error) {
	holder := combiner.holders[i]
	exhausted, err := holder.moveToNext()
	if err != nil {
		return false, err
	}
	if exhausted {
		combiner.holders[i].itr.Close()
		combiner.holders = append(combiner.holders[:i], combiner.holders[i+1:]...)

	}
	return exhausted, nil
}

// kvAt returns the kv available from iterator at index i
func (combiner *itrCombiner) kvAt(i int) *statedb.VersionedKV {
	return combiner.holders[i].kv
}

// keyAt returns the key available from iterator at index i
func (combiner *itrCombiner) keyAt(i int) string {
	return combiner.kvAt(i).Key
}

// Close closes all the underlying iterators
func (combiner *itrCombiner) Close() {
	for _, holder := range combiner.holders {
		holder.itr.Close()
	}
}

// itrHolder encloses an iterator and keeps the next item available from the iterator in the buffer
type itrHolder struct {
	itr statedb.ResultsIterator
	kv  *statedb.VersionedKV
}

// moveToNext fetches the next item to keep in buffer and returns true if the iterator is exhausted
func (holder *itrHolder) moveToNext() (exhausted bool, err error) {
	var res statedb.QueryResult
	if res, err = holder.itr.Next(); err != nil {
		return false, err
	}
	if res != nil {
		holder.kv = res.(*statedb.VersionedKV)
	}
	return res == nil, nil
}

// String returns the key that the holder has in the buffer for serving as a next key
func (holder *itrHolder) String() string {
	return fmt.Sprintf("{%s}", holder.kv.Key)
}
