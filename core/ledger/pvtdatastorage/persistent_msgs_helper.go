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
	} else {
		// due to protobuf encoding/decoding, the previously
		// initialized map could be a nil now due to 0 length.
		// Hence, we need to reinitialize the map.
		if collections.Map == nil {
			collections.Map = make(map[string]*TxNums)
		}
		if collections.MissingDataMap == nil {
			collections.MissingDataMap = make(map[string]bool)
		}
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

func newCollElgInfo(nsCollMap map[string][]string) *CollElgInfo {
	m := &CollElgInfo{NsCollMap: map[string]*CollNames{}}
	for ns, colls := range nsCollMap {
		collNames, ok := m.NsCollMap[ns]
		if !ok {
			collNames = &CollNames{}
			m.NsCollMap[ns] = collNames
		}
		collNames.Entries = colls
	}
	return m
}
