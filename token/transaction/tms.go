/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/hyperledger/fabric/token/ledger"
)

//go:generate counterfeiter -o mock/tms_tx_processor.go -fake-name TMSTxProcessor . TMSTxProcessor
//go:generate counterfeiter -o mock/tms_manager.go -fake-name TMSManager . TMSManager

// TMSTxProcessor is used to generate the read-dependencies of a token transaction
// (read-set) along with the ledger updates triggered by that transaction
// (write-set); read-write sets are returned implicitly via the simulator object
// that is passed as parameter in the Commit function
type TMSTxProcessor interface {
	// ProcessTx parses ttx to generate a RW set
	ProcessTx(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error
}

type TMSManager interface {
	// GetTxProcessor returns a TxProcessor for TMS transactions for the provided channel
	GetTxProcessor(channel string) (TMSTxProcessor, error)
}

type TxCreatorInfo struct {
	public []byte
}

func (t *TxCreatorInfo) Public() []byte {
	return t.public
}
