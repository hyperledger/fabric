/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package transaction

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabtoken-processor")

// Processor implements the interface 'github.com/hyperledger/fabric/core/ledger/customtx/Processor'
// for FabToken transactions
type Processor struct {
	TMSManager TMSManager
}

func (p *Processor) GenerateSimulationResults(txEnv *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error {
	// Extract channel header and token transaction
	ch, ttx, err := UnmarshalTokenTransaction(txEnv.Payload)
	if err != nil {
		return errors.Errorf("failed unmarshalling token transaction: %s", err)
	}

	// Get a TMSTxProcessor that corresponds to the channel
	verifier, err := p.TMSManager.GetTxProcessor(ch.ChannelId)
	if err != nil {
		return errors.Errorf("failed getting committer for channel %s: %s", ch.ChannelId, err)
	}

	// Extract the read dependencies and leger updates associated to the transaction using simulator
	err = verifier.ProcessTx(ttx, simulator, initializingLedger)
	if err != nil {
		return errors.Errorf("failed committing transaction for channel %s: %s", ch.ChannelId, err)
	}

	return err
}
