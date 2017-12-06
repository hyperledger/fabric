/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
)

// kvLedgerStateListeners contains the state listeners for kv ledger implementation
var kvLedgerStateListeners = map[string]ledger.StateListener{
	// lscc namespace listener for chaincode instantiate transactions (which manipulates data in 'lscc' namespace)
	// performs additional tasks such as index creations if couchdb is configured to use as the statedb.
	"lscc": &cceventmgmt.KVLedgerLSCCStateListener{},
}
