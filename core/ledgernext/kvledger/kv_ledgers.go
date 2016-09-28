/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kvledger

import (
	"fmt"
	"strings"
	"sync"
)

//--!!!!IMPORTANT!!!--!!!IMPORTANT!!!---!!!IMPORTANT!!!-----------
//
//Three things about this code
//
//  1. This code is TEMPORARY - it can go away when (sub) ledgers
//     management is implemented
//  2. This code is PLACEHOLDER - in lieu of proper (sub) ledgers
//     management, we need a mechanism to hold onto ledger handles
//     This merely does that
//  3. Do NOT add any function related to subledgers till it has
//     been agreed upon
//-----------------------------------------------------------------

//-------- initialization --------
var lManager *ledgerManager

//Initialize kv ledgers
func Initialize(lpath string) {
	if lpath == "" {
		panic("DB path not specified")
	}
	if !strings.HasSuffix(lpath, "/") {
		lpath = lpath + "/"
	}

	//TODO - when we don't need 0.5 DB, we can remove this
	lpath = lpath + "ledgernext/"

	lManager = &ledgerManager{ledgerPath: lpath, ledgers: make(map[string]*KVLedger)}
}

//--------- errors -----------

//LedgerNotInitializedErr exists error
type LedgerNotInitializedErr string

func (l LedgerNotInitializedErr) Error() string {
	return fmt.Sprintf("ledger manager not inialized")
}

//LedgerExistsErr exists error
type LedgerExistsErr string

func (l LedgerExistsErr) Error() string {
	return fmt.Sprintf("ledger exists %s", string(l))
}

//LedgerCreateErr exists error
type LedgerCreateErr string

func (l LedgerCreateErr) Error() string {
	return fmt.Sprintf("ledger creation failed %s", string(l))
}

//--------- ledger manager ---------
// just a container for ledgers
type ledgerManager struct {
	sync.RWMutex
	ledgerPath string
	ledgers    map[string]*KVLedger
}

//create a ledger if one does not exist
func (lMgr *ledgerManager) create(name string) (*KVLedger, error) {
	lMgr.Lock()
	defer lMgr.Unlock()

	lPath := lMgr.ledgerPath + name
	lgr, _ := lMgr.ledgers[lPath]
	if lgr != nil {
		return lgr, LedgerExistsErr(name)
	}

	var err error

	ledgerConf := NewConf(lPath, 0)
	if lgr, err = NewKVLedger(ledgerConf); err != nil || lgr == nil {
		return nil, LedgerCreateErr(name)
	}

	lMgr.ledgers[lPath] = lgr

	return lgr, nil
}

//GetLedger returns a kvledger, creating one if necessary
//the call will panic if it cannot create a ledger
func GetLedger(name string) *KVLedger {
	lgr, err := lManager.create(name)

	if lgr == nil {
		panic("Cannot get ledger " + name + "(" + err.Error() + ")")
	}
	return lgr
}
