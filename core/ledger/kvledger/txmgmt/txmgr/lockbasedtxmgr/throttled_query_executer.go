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

// throttledQueryExecutor is a query executor used in `LockBasedTxMgr`
type throttledQueryExecutor struct {
	*lockBasedQueryExecutor
	throttleSemaphore semaphore.Semaphore
}

func newThrottledQueryExecutor(txmgr *LockBasedTxMgr, txid string, performCollCheck bool, hasher ledger.Hasher, throttleSemaphore semaphore.Semaphore) (*throttledQueryExecutor, error) {
	// the semaphore will be released by ThrottledQueryExecutor.Done
	if err := throttleSemaphore.Acquire(context.Background()); err != nil {
		return nil, err
	}
	return &throttledQueryExecutor{
		lockBasedQueryExecutor: newQueryExecutor(txmgr, txid, performCollCheck, hasher),
		throttleSemaphore:      throttleSemaphore,
	}, nil
}

// Done implements method in interface `ledger.QueryExecutor`
func (tq *throttledQueryExecutor) Done() {
	logger.Debugf("Done with throttled transaction query execution [%s]", tq.txid)
	tq.helper.done(tq.throttleSemaphore)
}
