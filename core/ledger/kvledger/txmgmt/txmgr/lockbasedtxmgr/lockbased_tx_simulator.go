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

package lockbasedtxmgr

import (
	"errors"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
)

// LockBasedTxSimulator is a transaction simulator used in `LockBasedTxMgr`
type lockBasedTxSimulator struct {
	lockBasedQueryExecutor
	rwset *rwset.RWSet
}

func newLockBasedTxSimulator(txmgr *LockBasedTxMgr) *lockBasedTxSimulator {
	rwset := rwset.NewRWSet()
	helper := &queryHelper{txmgr: txmgr, rwset: rwset}
	id := util.GenerateUUID()
	logger.Debugf("constructing new tx simulator [%s]", id)
	return &lockBasedTxSimulator{lockBasedQueryExecutor{helper, id}, rwset}
}

// GetState implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) GetState(ns string, key string) ([]byte, error) {
	return s.helper.getState(ns, key)
}

// SetState implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetState(ns string, key string, value []byte) error {
	s.helper.checkDone()
	s.rwset.AddToWriteSet(ns, key, value)
	return nil
}

// DeleteState implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) DeleteState(ns string, key string) error {
	return s.SetState(ns, key, nil)
}

// SetStateMultipleKeys implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	for k, v := range kvs {
		if err := s.SetState(namespace, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetTxSimulationResults implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) GetTxSimulationResults() ([]byte, error) {
	logger.Debugf("Simulation completed, getting simulation results")
	s.Done()
	return s.rwset.GetTxReadWriteSet().Marshal()
}

// ExecuteUpdate implements method in interface `ledger.TxSimulator`
func (s *lockBasedTxSimulator) ExecuteUpdate(query string) error {
	return errors.New("Not supported")
}
