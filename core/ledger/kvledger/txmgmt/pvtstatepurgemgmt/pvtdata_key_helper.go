/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

func (pvtdataKeys *PvtdataKeys) addAll(toAdd *PvtdataKeys) {
	for ns, colls := range toAdd.GetMap() {
		for coll, keysAndHashes := range colls.GetMap() {
			for _, k := range keysAndHashes.GetList() {
				pvtdataKeys.add(ns, coll, k.GetKey(), k.GetHash())
			}
		}
	}
}

func (pvtdataKeys *PvtdataKeys) add(ns string, coll string, key string, keyhash []byte) {
	colls := pvtdataKeys.getOrCreateCollections(ns)
	keysAndHashes := colls.getOrCreateKeysAndHashes(coll)
	keysAndHashes.List = append(keysAndHashes.List, &KeyAndHash{Key: key, Hash: keyhash})
}

func (pvtdataKeys *PvtdataKeys) getOrCreateCollections(ns string) *Collections {
	colls, ok := pvtdataKeys.GetMap()[ns]
	if !ok {
		colls = newCollections()
		pvtdataKeys.Map[ns] = colls
	}
	return colls
}

func (colls *Collections) getOrCreateKeysAndHashes(coll string) *KeysAndHashes {
	keysAndHashes, ok := colls.GetMap()[coll]
	if !ok {
		keysAndHashes = &KeysAndHashes{}
		colls.Map[coll] = keysAndHashes
	}
	return keysAndHashes
}

func newPvtdataKeys() *PvtdataKeys {
	return &PvtdataKeys{Map: make(map[string]*Collections)}
}

func newCollections() *Collections {
	return &Collections{Map: make(map[string]*KeysAndHashes)}
}
