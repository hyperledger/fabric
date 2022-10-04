/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"strings"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

// combinedIterator implements the interface statedb.ResultsIterator.
// Internally, it maintains two iterators
//   - (1) dbItr - an iterator that iterates over keys present in the db
//   - (2) updatesItr - an iterator that iterates over keys present in the update batch
//     (i.e, the keys that are inserted/updated/deleted by preceding valid transactions
//     in the block and to be committed to the db as a part of block commit operation)
//
// This can be used where the caller wants to see what would be the final results of
// iterating over a key range if the modifications of the preceding valid transactions
// were to be applied to the db
//
// This can be used to perform validation for phantom reads in a transactions rwset
type combinedIterator struct {
	// input
	ns            string
	db            statedb.VersionedDB
	updates       *statedb.UpdateBatch
	endKey        string
	includeEndKey bool

	// internal state
	dbItr        statedb.ResultsIterator
	updatesItr   statedb.ResultsIterator
	dbItem       *statedb.VersionedKV
	updatesItem  *statedb.VersionedKV
	endKeyServed bool
}

func newCombinedIterator(db statedb.VersionedDB, updates *statedb.UpdateBatch,
	ns string, startKey string, endKey string, includeEndKey bool) (*combinedIterator, error) {
	var dbItr statedb.ResultsIterator
	var updatesItr statedb.ResultsIterator
	var err error
	if dbItr, err = db.GetStateRangeScanIterator(ns, startKey, endKey); err != nil {
		return nil, err
	}
	updatesItr = updates.GetRangeScanIterator(ns, startKey, endKey)
	var dbItem, updatesItem *statedb.VersionedKV
	if dbItem, err = dbItr.Next(); err != nil {
		return nil, err
	}
	if updatesItem, err = updatesItr.Next(); err != nil {
		return nil, err
	}
	logger.Debugf("Combined iterator initialized. dbItem=%#v, updatesItem=%#v", dbItem, updatesItem)
	return &combinedIterator{
		ns, db, updates, endKey, includeEndKey,
		dbItr, updatesItr, dbItem, updatesItem, false,
	}, nil
}

// Next returns the KV from either dbItr or updatesItr that gives the next smaller key
// If both gives the same keys, then it returns the KV from updatesItr.
func (itr *combinedIterator) Next() (*statedb.VersionedKV, error) {
	if itr.dbItem == nil && itr.updatesItem == nil {
		logger.Debugf("dbItem and updatesItem both are nil.")
		return itr.serveEndKeyIfNeeded()
	}
	var moveDBItr bool
	var moveUpdatesItr bool
	var selectedItem *statedb.VersionedKV
	compResult := compareKeys(itr.dbItem, itr.updatesItem)
	logger.Debugf("compResult=%d", compResult)
	switch compResult {
	case -1:
		// dbItem is smaller
		selectedItem = itr.dbItem
		moveDBItr = true
	case 0:
		// both items are same so, choose the updatesItem (latest)
		selectedItem = itr.updatesItem
		moveUpdatesItr = true
		moveDBItr = true
	case 1:
		// updatesItem is smaller
		selectedItem = itr.updatesItem
		moveUpdatesItr = true
	}
	var err error
	if moveDBItr {
		if itr.dbItem, err = itr.dbItr.Next(); err != nil {
			return nil, err
		}
	}

	if moveUpdatesItr {
		if itr.updatesItem, err = itr.updatesItr.Next(); err != nil {
			return nil, err
		}
	}
	if isDelete(selectedItem) {
		return itr.Next()
	}
	logger.Debugf("Returning item=%#v. Next dbItem=%#v, Next updatesItem=%#v", selectedItem, itr.dbItem, itr.updatesItem)
	return selectedItem, nil
}

func (itr *combinedIterator) Close() {
	itr.dbItr.Close()
}

func (itr *combinedIterator) GetBookmarkAndClose() string {
	itr.Close()
	return ""
}

// serveEndKeyIfNeeded returns the endKey only once and only if includeEndKey was set to true
// in the constructor of combinedIterator.
func (itr *combinedIterator) serveEndKeyIfNeeded() (*statedb.VersionedKV, error) {
	if !itr.includeEndKey || itr.endKeyServed {
		logger.Debugf("Endkey not to be served. Returning nil... [toInclude=%t, alreadyServed=%t]",
			itr.includeEndKey, itr.endKeyServed)
		return nil, nil
	}
	logger.Debug("Serving the endKey")
	var vv *statedb.VersionedValue
	var err error
	vv = itr.updates.Get(itr.ns, itr.endKey)
	logger.Debugf("endKey value from updates:%s", vv)
	if vv == nil {
		if vv, err = itr.db.GetState(itr.ns, itr.endKey); err != nil {
			return nil, err
		}
		logger.Debugf("endKey value from stateDB:%s", vv)
	}
	itr.endKeyServed = true
	if vv == nil {
		return nil, nil
	}
	vkv := &statedb.VersionedKV{
		CompositeKey: &statedb.CompositeKey{
			Namespace: itr.ns,
			Key:       itr.endKey,
		},
		VersionedValue: &statedb.VersionedValue{
			Value:   vv.Value,
			Version: vv.Version,
		},
	}

	if isDelete(vkv) {
		return nil, nil
	}
	return vkv, nil
}

func compareKeys(item1 *statedb.VersionedKV, item2 *statedb.VersionedKV) int {
	if item1 == nil {
		if item2 == nil {
			return 0
		}
		return 1
	}
	if item2 == nil {
		return -1
	}
	// at this stage both items are not nil
	return strings.Compare(item1.Key, item2.Key)
}

func isDelete(item *statedb.VersionedKV) bool {
	return item.Value == nil
}
