/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

func newExpiryData() *ExpiryData {
	return &ExpiryData{Map: make(map[string]*Collections)}
}

func newCollections() *Collections {
	return &Collections{Map: make(map[string]*TxNums)}
}

func (e *ExpiryData) add(ns, coll string, txNum uint64) {
	collections, ok := e.Map[ns]
	if !ok {
		collections = newCollections()
		e.Map[ns] = collections
	}
	txNums, ok := collections.Map[coll]
	if !ok {
		txNums = &TxNums{}
		collections.Map[coll] = txNums
	}
	txNums.List = append(txNums.List, txNum)
}
