/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"context"

	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/core/ledger"
)

// throttledTxSimulator is a query executor used in `LockBasedTxMgr`
type throttledTxSimulator struct {
	*lockBasedTxSimulator
	throttleSemaphore semaphore.Semaphore
}

func newThrottledTxSimulator(txmgr *LockBasedTxMgr, txid string, hasher ledger.Hasher, throttleSemaphore semaphore.Semaphore) (*throttledTxSimulator, error) {
	txSimulator, err := newLockBasedTxSimulator(txmgr, txid, hasher)
	if err != nil {
		return nil, err
	}

	// the semaphore will be released by ThrottledTxSimulator.Done
	if err := throttleSemaphore.Acquire(context.Background()); err != nil {
		return nil, err
	}
	return &throttledTxSimulator{
		lockBasedTxSimulator: txSimulator,
		throttleSemaphore:    throttleSemaphore,
	}, nil
}

// Done implements method in interface `ledger.QueryExecutor`
func (ts *throttledTxSimulator) Done() {
	logger.Debugf("Done with throttled transaction simulation [%s]", ts.txid)
	ts.helper.done(ts.throttleSemaphore)
}
