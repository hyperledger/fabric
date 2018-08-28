/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

func newExpiryData() *ExpiryData {
	return &ExpiryData{Map: make(map[string]*Collections)}
}

func (e *ExpiryData) getOrCreateCollections(ns string) *Collections {
	collections, ok := e.Map[ns]
	if !ok {
		collections = &Collections{
			Map:            make(map[string]*TxNums),
			MissingDataMap: make(map[string]bool)}
		e.Map[ns] = collections
	}

	return collections
}

func (e *ExpiryData) addPresentData(ns, coll string, txNum uint64) {
	collections := e.getOrCreateCollections(ns)

	txNums, ok := collections.Map[coll]
	if !ok {
		txNums = &TxNums{}
		collections.Map[coll] = txNums
	}
	txNums.List = append(txNums.List, txNum)
}

func (e *ExpiryData) addMissingData(ns, coll string) {
	collections := e.getOrCreateCollections(ns)
	collections.MissingDataMap[coll] = true
}
