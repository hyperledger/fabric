/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"errors"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

// TxProcessor implements the interface 'github.com/hyperledger/fabric/core/ledger/customtx/Processor'
// for FabToken transactions
type TxProcessor struct {
}

func (tp *TxProcessor) GenerateSimulationResults(txEnv *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error {
	return errors.New("implement me")
}
