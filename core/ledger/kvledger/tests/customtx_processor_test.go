/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	protopeer "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestReadWriteCustomTxProcessor(t *testing.T) {
	fakeTxProcessor := &mock.CustomTxProcessor{}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	env := newEnvWithInitializer(
		t,
		&ledgermgmt.Initializer{
			CustomTxProcessors: map[common.HeaderType]ledger.CustomTxProcessor{
				100: fakeTxProcessor,
			},
			HashProvider: cryptoProvider,
		},
	)
	defer env.cleanup()
	env.initLedgerMgmt()

	l := env.createTestLedgerFromGenesisBlk("ledger1")
	l.simulateDataTx("tx0", func(s *simulator) {
		s.setState("ns", "key1", "value1")
		s.setState("ns", "key2", "value2")
		s.setState("ns", "key3", "value3")
	})
	l.cutBlockAndCommitLegacy() // commit block-1 to populate initial state

	valueCounter := 0
	fakeTxProcessor.GenerateSimulationResultsStub =
		// tx processor reads and modifies key1
		func(txEnvelop *common.Envelope, s ledger.TxSimulator, initializingLedger bool) error {
			valKey1, err := s.GetState("ns", "key1")
			require.NoError(t, err)
			require.Equal(t, []byte("value1"), valKey1)
			valueCounter++
			return s.SetState("ns", "key1", []byte(fmt.Sprintf("value1_%d", valueCounter)))
		}

	// block-2 with two post order transactions
	l.addPostOrderTx("tx1", 100)
	l.addPostOrderTx("tx2", 100)
	l.cutBlockAndCommitLegacy()

	// Tx1 should be valid and tx2 should be marked as invalid because, tx1 has already modified key1
	// in the same block
	l.verifyTxValidationCode("tx1", protopeer.TxValidationCode_VALID)
	l.verifyTxValidationCode("tx2", protopeer.TxValidationCode_MVCC_READ_CONFLICT)
	l.verifyPubState("ns", "key1", "value1_1")
}

func TestRangeReadAndWriteCustomTxProcessor(t *testing.T) {
	fakeTxProcessor1 := &mock.CustomTxProcessor{}
	fakeTxProcessor2 := &mock.CustomTxProcessor{}
	fakeTxProcessor3 := &mock.CustomTxProcessor{}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	env := newEnvWithInitializer(
		t,
		&ledgermgmt.Initializer{
			CustomTxProcessors: map[common.HeaderType]ledger.CustomTxProcessor{
				101: fakeTxProcessor1,
				102: fakeTxProcessor2,
				103: fakeTxProcessor3,
			},
			HashProvider: cryptoProvider,
		},
	)
	defer env.cleanup()
	env.initLedgerMgmt()

	l := env.createTestLedgerFromGenesisBlk("ledger1")
	l.simulateDataTx("tx0", func(s *simulator) {
		s.setState("ns", "key1", "value1")
		s.setState("ns", "key2", "value2")
		s.setState("ns", "key3", "value3")
	})
	l.cutBlockAndCommitLegacy() // commit block-1 to populate initial state

	fakeTxProcessor1.GenerateSimulationResultsStub =
		// tx processor for txtype 101 sets key1
		func(txEnvelop *common.Envelope, s ledger.TxSimulator, initializingLedger bool) error {
			return s.SetState("ns", "key1", []byte("value1_new"))
		}

	fakeTxProcessor2.GenerateSimulationResultsStub =
		// tx processor for txtype 102 reads a range (that covers key1) and sets key2
		func(txEnvelop *common.Envelope, s ledger.TxSimulator, initializingLedger bool) error {
			itr, err := s.GetStateRangeScanIterator("ns", "key1", "key2")
			require.NoError(t, err)
			for {
				res, err := itr.Next()
				require.NoError(t, err)
				if res == nil {
					break
				}
			}
			return s.SetState("ns", "key2", []byte("value2_new"))
		}

	fakeTxProcessor3.GenerateSimulationResultsStub =
		// tx processor for txtype 103 reads a range (that does not include key1) and sets key2
		func(txEnvelop *common.Envelope, s ledger.TxSimulator, initializingLedger bool) error {
			itr, err := s.GetStateRangeScanIterator("ns", "key2", "key3")
			require.NoError(t, err)
			for {
				res, err := itr.Next()
				require.NoError(t, err)
				if res == nil {
					break
				}
			}
			return s.SetState("ns", "key3", []byte("value3_new"))
		}

	// block-2 with three post order transactions
	l.addPostOrderTx("tx1", 101)
	l.addPostOrderTx("tx2", 102)
	l.addPostOrderTx("tx3", 103)
	l.cutBlockAndCommitLegacy()

	// Tx1 should be valid and tx2 should be marked as invalid because, tx1 has already modified key1
	// in the same block (and tx2 does a range iteration that includes key1)
	// However, tx3 should be fine as this performs a range itertaes that does not include key1
	l.verifyTxValidationCode("tx1", protopeer.TxValidationCode_VALID)
	l.verifyTxValidationCode("tx2", protopeer.TxValidationCode_PHANTOM_READ_CONFLICT)
	l.verifyTxValidationCode("tx3", protopeer.TxValidationCode_VALID)
	l.verifyPubState("ns", "key1", "value1_new")
	l.verifyPubState("ns", "key2", "value2")
	l.verifyPubState("ns", "key3", "value3_new")
}
