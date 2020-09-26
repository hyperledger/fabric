/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

func newExpiryData() *ExpiryData {
	return &ExpiryData{Map: make(map[string]*NamespaceExpiryData)}
}

func (e *ExpiryData) getOrCreateCollections(ns string) *NamespaceExpiryData {
	nsExpiryData, ok := e.Map[ns]
	if !ok {
		nsExpiryData = &NamespaceExpiryData{
			PresentData:  make(map[string]*TxNums),
			MissingData:  make(map[string]bool),
			BootKVHashes: make(map[string]*TxNums),
		}
		e.Map[ns] = nsExpiryData
	} else {
		// due to protobuf encoding/decoding, the previously
		// initialized map could be a nil now due to 0 length.
		// Hence, we need to reinitialize the map.
		if nsExpiryData.PresentData == nil {
			nsExpiryData.PresentData = make(map[string]*TxNums)
		}
		if nsExpiryData.MissingData == nil {
			nsExpiryData.MissingData = make(map[string]bool)
		}
		if nsExpiryData.BootKVHashes == nil {
			nsExpiryData.BootKVHashes = make(map[string]*TxNums)
		}
	}
	return nsExpiryData
}

func (e *ExpiryData) addPresentData(ns, coll string, txNum uint64) {
	nsExpiryData := e.getOrCreateCollections(ns)

	txNums, ok := nsExpiryData.PresentData[coll]
	if !ok {
		txNums = &TxNums{}
		nsExpiryData.PresentData[coll] = txNums
	}
	txNums.List = append(txNums.List, txNum)
}

func (e *ExpiryData) addMissingData(ns, coll string) {
	nsExpiryData := e.getOrCreateCollections(ns)
	nsExpiryData.MissingData[coll] = true
}

func (e *ExpiryData) addBootKVHash(ns, coll string, txNum uint64) {
	nsExpiryData := e.getOrCreateCollections(ns)

	txNums, ok := nsExpiryData.BootKVHashes[coll]
	if !ok {
		txNums = &TxNums{}
		nsExpiryData.BootKVHashes[coll] = txNums
	}
	txNums.List = append(txNums.List, txNum)
}

func (h *BootKVHashes) toMap() map[string][]byte {
	m := make(map[string][]byte, len(h.List))
	for _, kv := range h.List {
		m[string(kv.KeyHash)] = kv.ValueHash
	}
	return m
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
