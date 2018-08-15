/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package transaction

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/token"
)

//go:generate mockery -dir . -name Committer -case underscore -output ./mocks/
//go:generate mockery -dir . -name TMSManager -case underscore -output ./mocks/

// TMSTxProcessor is used to generate the read-dependencies of a token transaction
// (read-set) along with the ledger updates triggered by that transaction
// (write-set); read-write sets are returned implicitly via the simulator object
// that is passed as parameter in the Commit function
type TMSTxProcessor interface {
	// ProcessTx parses ttx to generate a RW set
	ProcessTx(ttx *token.TokenTransaction, simulator ledger.TxSimulator, initializingLedger bool) error
}

type TMSManager interface {
	// GetTxProcessor returns a TxProcessor for TMS transactions for the provided channel
	GetTxProcessor(channel string) (TMSTxProcessor, error)
}
