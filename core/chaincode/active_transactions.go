/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import "sync"

func NewTxKey(channelID, txID string) string { return channelID + txID }

type ActiveTransactions struct {
	mutex sync.Mutex
	ids   map[string]struct{}
}

func NewActiveTransactions() *ActiveTransactions {
	return &ActiveTransactions{
		ids: map[string]struct{}{},
	}
}

func (a *ActiveTransactions) Add(channelID, txID string) bool {
	key := NewTxKey(channelID, txID)
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if _, ok := a.ids[key]; ok {
		return false
	}

	a.ids[key] = struct{}{}
	return true
}

func (a *ActiveTransactions) Remove(channelID, txID string) {
	key := NewTxKey(channelID, txID)
	a.mutex.Lock()
	delete(a.ids, key)
	a.mutex.Unlock()
}
