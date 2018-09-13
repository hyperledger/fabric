/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	ctxt "github.com/hyperledger/fabric/common/configtx/test"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	ledger2 "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/testdata"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	mocks2 "github.com/hyperledger/fabric/discovery/support/mocks"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sync/semaphore"
)

func signedByAnyMember(ids []string) []byte {
	p := cauthdsl.SignedByAnyMember(ids)
	return utils.MarshalOrPanic(p)
}

func setupLedgerAndValidator(t *testing.T) (ledger.PeerLedger, txvalidator.Validator) {
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	return setupLedgerAndValidatorExplicit(t, &mockconfig.MockApplicationCapabilities{}, plugin)
}

func preV12Capabilities() *mockconfig.MockApplicationCapabilities {
	return &mockconfig.MockApplicationCapabilities{}
}

func v12Capabilities() *mockconfig.MockApplicationCapabilities {
	return &mockconfig.MockApplicationCapabilities{V1_2ValidationRv: true, PrivateChannelDataRv: true}
}

func v13Capabilities() *mockconfig.MockApplicationCapabilities {
	return &mockconfig.MockApplicationCapabilities{V1_2ValidationRv: true, PrivateChannelDataRv: true, V1_3ValidationRv: true, KeyLevelEndorsementRv: true}
}

func setupLedgerAndValidatorExplicit(t *testing.T, cpb *mockconfig.MockApplicationCapabilities, plugin validation.Plugin) (ledger.PeerLedger, txvalidator.Validator) {
	return setupLedgerAndValidatorExplicitWithMSP(t, cpb, plugin, nil)
}

func setupLedgerAndValidatorWithPreV12Capabilities(t *testing.T) (ledger.PeerLedger, txvalidator.Validator) {
	return setupLedgerAndValidatorWithCapabilities(t, preV12Capabilities())
}

func setupLedgerAndValidatorWithV12Capabilities(t *testing.T) (ledger.PeerLedger, txvalidator.Validator) {
	return setupLedgerAndValidatorWithCapabilities(t, v12Capabilities())
}

func setupLedgerAndValidatorWithV13Capabilities(t *testing.T) (ledger.PeerLedger, txvalidator.Validator) {
	return setupLedgerAndValidatorWithCapabilities(t, v13Capabilities())
}

func setupLedgerAndValidatorWithCapabilities(t *testing.T, c *mockconfig.MockApplicationCapabilities) (ledger.PeerLedger, txvalidator.Validator) {
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(nil)
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	return setupLedgerAndValidatorExplicitWithMSP(t, c, &builtin.DefaultValidation{}, mspmgr)
}

func setupLedgerAndValidatorExplicitWithMSP(t *testing.T, cpb *mockconfig.MockApplicationCapabilities, plugin validation.Plugin, mspMgr msp.MSPManager) (ledger.PeerLedger, txvalidator.Validator) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/validatortest")
	ledgermgmt.InitializeTestEnv()
	gb, err := ctxt.MakeGenesisBlock("TestLedger")
	assert.NoError(t, err)
	theLedger, err := ledgermgmt.CreateLedger(gb)
	assert.NoError(t, err)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: cpb, MSPManagerVal: mspMgr}, semaphore.NewWeighted(10)}
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()
	pm := &mocks.PluginMapper{}
	factory := &mocks.PluginFactory{}
	pm.On("PluginFactoryByName", txvalidator.PluginName("vscc")).Return(factory)
	factory.On("New").Return(plugin)

	theValidator := txvalidator.NewTxValidator(vcs, mp, pm)

	return theLedger, theValidator
}

func createRWset(t *testing.T, ccnames ...string) []byte {
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	for _, ccname := range ccnames {
		rwsetBuilder.AddToWriteSet(ccname, "key", []byte("value"))
	}
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	return rwsetBytes
}

func getProposalWithType(ccID string, pType common.HeaderType) (*peer.Proposal, error) {
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: ccID, Version: ccVersion},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("func")}},
			Type:        peer.ChaincodeSpec_GOLANG}}

	proposal, _, err := utils.CreateProposalFromCIS(pType, util.GetTestChainID(), cis, signerSerialized)
	return proposal, err
}

const ccVersion = "1.0"

func getEnvWithType(ccID string, event []byte, res []byte, pType common.HeaderType, t *testing.T) *common.Envelope {
	// get a toy proposal
	prop, err := getProposalWithType(ccID, pType)
	assert.NoError(t, err)

	response := &peer.Response{Status: 200}

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response, res, event, &peer.ChaincodeID{Name: ccID, Version: ccVersion}, nil, signer)
	assert.NoError(t, err)

	// assemble a transaction from that proposal and endorsement
	tx, err := utils.CreateSignedTx(prop, signer, presp)
	assert.NoError(t, err)

	return tx
}

func getEnv(ccID string, event []byte, res []byte, t *testing.T) *common.Envelope {
	return getEnvWithType(ccID, event, res, common.HeaderType_ENDORSER_TRANSACTION, t)
}

func putCCInfoWithVSCCAndVer(theLedger ledger.PeerLedger, ccname, vscc, ver string, policy []byte, t *testing.T) {
	cd := &ccp.ChaincodeData{
		Name:    ccname,
		Version: ver,
		Vscc:    vscc,
		Policy:  policy,
	}

	cdbytes := utils.MarshalOrPanic(cd)

	txid := util.GenerateUUID()
	simulator, err := theLedger.NewTxSimulator(txid)
	assert.NoError(t, err)
	simulator.SetState("lscc", ccname, cdbytes)
	simulator.Done()

	simRes, err := simulator.GetTxSimulationResults()
	assert.NoError(t, err)
	pubSimulationBytes, err := simRes.GetPubSimulationBytes()
	assert.NoError(t, err)
	block0 := testutil.ConstructBlock(t, 1, []byte("hash"), [][]byte{pubSimulationBytes}, true)
	err = theLedger.CommitWithPvtData(&ledger.BlockAndPvtData{
		Block: block0,
	})
	assert.NoError(t, err)
}

func putSBEP(theLedger ledger.PeerLedger, cc, key string, policy []byte, t *testing.T) {
	vpMetadataKey := peer.MetaDataKeys_VALIDATION_PARAMETER.String()
	txid := util.GenerateUUID()
	simulator, err := theLedger.NewTxSimulator(txid)
	assert.NoError(t, err)
	simulator.SetStateMetadata(cc, key, map[string][]byte{vpMetadataKey: policy})
	simulator.SetState(cc, key, []byte("I am a man who walks alone"))
	simulator.Done()

	simRes, err := simulator.GetTxSimulationResults()
	assert.NoError(t, err)
	pubSimulationBytes, err := simRes.GetPubSimulationBytes()
	assert.NoError(t, err)
	block0 := testutil.ConstructBlock(t, 2, []byte("hash"), [][]byte{pubSimulationBytes}, true)
	err = theLedger.CommitWithPvtData(&ledger.BlockAndPvtData{
		Block: block0,
	})
	assert.NoError(t, err)
}

func putCCInfo(theLedger ledger.PeerLedger, ccname string, policy []byte, t *testing.T) {
	putCCInfoWithVSCCAndVer(theLedger, ccname, "vscc", ccVersion, policy, t)
}

func assertInvalid(block *common.Block, t *testing.T, code peer.TxValidationCode) {
	txsFilter := lutils.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assert.True(t, txsFilter.IsInvalid(0))
	assert.True(t, txsFilter.IsSetTo(0, code))
}

func assertValid(block *common.Block, t *testing.T) {
	txsFilter := lutils.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assert.False(t, txsFilter.IsInvalid(0))
}

func TestInvokeBadRWSet(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeBadRWSet(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeBadRWSet(t, l, v)
	})
}

func testInvokeBadRWSet(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	tx := getEnv(ccID, nil, []byte("barf"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 1}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNoPolicy(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNoPolicy(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNoPolicy(t, l, v)
	})
}

func testInvokeNoPolicy(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, nil, t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeOK(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOK(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOK(t, l, v)
	})
}

func testInvokeOK(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeNoRWSet(t *testing.T) {
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	t.Run("Pre-1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicit(t, preV12Capabilities(), plugin)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		ccID := "mycc"

		putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

		tx := getEnv(ccID, nil, createRWset(t), t)
		b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

		err := v.Validate(b)
		assert.NoError(t, err)
		assertValid(b, t)
	})

	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	// No need to define validation behavior for the previous test case because pre 1.2 we don't validate transactions
	// that have no write set.
	t.Run("Post-1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, v12Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNoRWSet(t, l, v)
	})

	// Here we test that if we have the 1.3 capability, we still reject a transaction that only contains
	// reads if it doesn't comply with the endorsement policy
	t.Run("Post-1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, v13Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNoRWSet(t, l, v)
	})
}

func testInvokeNoRWSet(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestChaincodeEvent(t *testing.T) {
	t.Run("PreV1.2", func(t *testing.T) {
		t.Run("MisMatchedName", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithPreV12Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			ccID := "mycc"

			putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

			tx := getEnv(ccID, utils.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: "wrong"}), createRWset(t), t)
			b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

			err := v.Validate(b)
			assert.NoError(t, err)
			assertValid(b, t)
		})

		t.Run("BadBytes", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithPreV12Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			ccID := "mycc"

			putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

			tx := getEnv(ccID, []byte("garbage"), createRWset(t), t)
			b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

			err := v.Validate(b)
			assert.NoError(t, err)
			assertValid(b, t)
		})

		t.Run("GoodPath", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithPreV12Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			ccID := "mycc"

			putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

			tx := getEnv(ccID, utils.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: ccID}), createRWset(t), t)
			b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

			err := v.Validate(b)
			assert.NoError(t, err)
			assertValid(b, t)
		})
	})

	t.Run("PostV1.2", func(t *testing.T) {
		t.Run("MisMatchedName", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithV12Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			testCCEventMismatchedName(t, l, v)
		})

		t.Run("BadBytes", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithV12Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			testCCEventBadBytes(t, l, v)
		})

		t.Run("GoodPath", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithV12Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			testCCEventGoodPath(t, l, v)
		})
	})

	t.Run("V1.3", func(t *testing.T) {
		t.Run("MisMatchedName", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithV13Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			testCCEventMismatchedName(t, l, v)
		})

		t.Run("BadBytes", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithV13Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			testCCEventBadBytes(t, l, v)
		})

		t.Run("GoodPath", func(t *testing.T) {
			l, v := setupLedgerAndValidatorWithV13Capabilities(t)
			defer ledgermgmt.CleanupTestEnv()
			defer l.Close()

			testCCEventGoodPath(t, l, v)
		})
	})
}

func testCCEventMismatchedName(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, utils.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: "wrong"}), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err) // TODO, convert test so it can check the error text for INVALID_OTHER_REASON
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func testCCEventBadBytes(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, []byte("garbage"), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err) // TODO, convert test so it can check the error text for INVALID_OTHER_REASON
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func testCCEventGoodPath(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, utils.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: ccID}), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeOKPvtDataOnly(t *testing.T) {
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	t.Run("V1.2", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, v12Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKPvtDataOnly(t, l, v)
	})

	t.Run("V1.3", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, v13Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKPvtDataOnly(t, l, v)
	})
}

func testInvokeOKPvtDataOnly(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToPvtAndHashedWriteSet(ccID, "mycollection", "somekey", nil)
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	assert.NoError(t, err)

	tx := getEnv(ccID, nil, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err = v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeOKMetaUpdateOnly(t *testing.T) {
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	t.Run("V1.2", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, &mockconfig.MockApplicationCapabilities{V1_2ValidationRv: true, PrivateChannelDataRv: true}, &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKMetaUpdateOnly(t, l, v)
	})

	t.Run("V1.3", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, &mockconfig.MockApplicationCapabilities{V1_3ValidationRv: true, V1_2ValidationRv: true, PrivateChannelDataRv: true}, &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKMetaUpdateOnly(t, l, v)
	})
}

func testInvokeOKMetaUpdateOnly(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToMetadataWriteSet(ccID, "somekey", map[string][]byte{})
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	assert.NoError(t, err)

	tx := getEnv(ccID, nil, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err = v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeOKPvtMetaUpdateOnly(t *testing.T) {
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	t.Run("V1.2", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, &mockconfig.MockApplicationCapabilities{V1_2ValidationRv: true, PrivateChannelDataRv: true}, &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKPvtMetaUpdateOnly(t, l, v)
	})

	t.Run("V1.3", func(t *testing.T) {
		l, v := setupLedgerAndValidatorExplicitWithMSP(t, &mockconfig.MockApplicationCapabilities{V1_3ValidationRv: true, V1_2ValidationRv: true, PrivateChannelDataRv: true}, &builtin.DefaultValidation{}, mspmgr)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKPvtMetaUpdateOnly(t, l, v)
	})
}

func testInvokeOKPvtMetaUpdateOnly(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToHashedMetadataWriteSet(ccID, "mycollection", "somekey", map[string][]byte{})
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	assert.NoError(t, err)

	tx := getEnv(ccID, nil, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err = v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeOKSCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKSCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeOKSCC(t, l, v)
	})
}

func testInvokeOKSCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	cds := utils.MarshalOrPanic(&peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "cc", Version: "ver"},
			Input:       &peer.ChaincodeInput{},
		},
	})
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: "lscc", Version: ccVersion},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte(util.GetTestChainID()), cds}},
			Type:        peer.ChaincodeSpec_GOLANG}}

	prop, _, err := utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, signerSerialized)
	assert.NoError(t, err)
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", "cc", utils.MarshalOrPanic(&ccp.ChaincodeData{Name: "cc", Version: "ver", InstantiationPolicy: cauthdsl.MarshaledAcceptAllPolicy}))
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	assert.NoError(t, err)
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, rwsetBytes, nil, &peer.ChaincodeID{Name: "lscc", Version: ccVersion}, nil, signer)
	assert.NoError(t, err)
	tx, err := utils.CreateSignedTx(prop, signer, presp)
	assert.NoError(t, err)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 1}}

	err = v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeNOKWritesToLSCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKWritesToLSCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKWritesToLSCC(t, l, v)
	})
}

func testInvokeNOKWritesToLSCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "lscc"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKWritesToESCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKWritesToESCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKWritesToESCC(t, l, v)
	})
}

func testInvokeNOKWritesToESCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "escc"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKWritesToNotExt(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKWritesToNotExt(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKWritesToNotExt(t, l, v)
	})
}

func testInvokeNOKWritesToNotExt(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "notext"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKInvokesNotExt(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKInvokesNotExt(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKInvokesNotExt(t, l, v)
	})
}

func testInvokeNOKInvokesNotExt(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "notext"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKInvokesEmptyCCName(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKInvokesEmptyCCName(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKInvokesEmptyCCName(t, l, v)
	})
}

func testInvokeNOKInvokesEmptyCCName(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := ""

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNOKExpiredCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKExpiredCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKExpiredCC(t, l, v)
	})
}

func testInvokeNOKExpiredCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "vscc", "badversion", signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_EXPIRED_CHAINCODE)
}

func TestInvokeNOKBogusActions(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKBogusActions(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKBogusActions(t, l, v)
	})
}

func testInvokeNOKBogusActions(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, []byte("barf"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNOKCCDoesntExist(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKCCDoesntExist(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKCCDoesntExist(t, l, v)
	})
}

func testInvokeNOKCCDoesntExist(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNOKVSCCUnspecified(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKVSCCUnspecified(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNOKVSCCUnspecified(t, l, v)
	})
}

func testInvokeNOKVSCCUnspecified(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "", ccVersion, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNoBlock(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNoBlock(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		testInvokeNoBlock(t, l, v)
	})
}

func testInvokeNoBlock(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	err := v.Validate(&common.Block{Data: &common.BlockData{Data: [][]byte{}}})
	assert.NoError(t, err)
}

func TestValidateTxWithStateBasedEndorsement(t *testing.T) {

	// SCENARIO: we validate a transaction that writes to key "key". This key
	// has a state-based endorsement policy that cannot be satisfied, while
	// the chaincode endorseemnt policy is satisfied by this transaction.
	// When we run with the 1.2 capability we expect the transaction to be
	// successfully validated, while when we run with the 1.3 capability,
	// validation is expected to fail owing to the unsatisfiable SBEP.
	// Notice that this corner case should never occur in practice, since
	// capabilities also determine whether honest peers will endorse
	// chaincodes that set state-based endorsement policies. Still, the test
	// is valuable as it shows how transactions that are affected by state-based
	// endorsement policies are handled by the 1.3 validation, and simply
	// ignored by 1.2.

	t.Run("1.2Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV12Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		err, b := validateTxWithStateBasedEndorsement(t, l, v)

		assert.NoError(t, err)
		assertValid(b, t)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v := setupLedgerAndValidatorWithV13Capabilities(t)
		defer ledgermgmt.CleanupTestEnv()
		defer l.Close()

		err, b := validateTxWithStateBasedEndorsement(t, l, v)

		assert.NoError(t, err)
		assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
	})
}

func validateTxWithStateBasedEndorsement(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) (error, *common.Block) {
	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "vscc", ccVersion, signedByAnyMember([]string{"SampleOrg"}), t)
	putSBEP(l, ccID, "key", cauthdsl.MarshaledRejectAllPolicy, t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 3}}

	err := v.Validate(b)

	return err, b
}

// mockLedger structure used to test ledger
// failure, therefore leveraging mocking
// library as need to simulate ledger which not
// able to get access to state db
type mockLedger struct {
	mock.Mock
}

// GetTransactionByID returns transaction by ud
func (m *mockLedger) GetTransactionByID(txID string) (*peer.ProcessedTransaction, error) {
	args := m.Called(txID)
	return args.Get(0).(*peer.ProcessedTransaction), args.Error(1)
}

// GetBlockByHash returns block using its hash value
func (m *mockLedger) GetBlockByHash(blockHash []byte) (*common.Block, error) {
	args := m.Called(blockHash)
	return args.Get(0).(*common.Block), nil
}

// GetBlockByTxID given transaction id return block transaction was committed with
func (m *mockLedger) GetBlockByTxID(txID string) (*common.Block, error) {
	args := m.Called(txID)
	return args.Get(0).(*common.Block), nil
}

// GetTxValidationCodeByTxID returns validation code of give tx
func (m *mockLedger) GetTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	args := m.Called(txID)
	return args.Get(0).(peer.TxValidationCode), nil
}

// NewTxSimulator creates new transaction simulator
func (m *mockLedger) NewTxSimulator(txid string) (ledger.TxSimulator, error) {
	args := m.Called()
	return args.Get(0).(ledger.TxSimulator), nil
}

// NewQueryExecutor creates query executor
func (m *mockLedger) NewQueryExecutor() (ledger.QueryExecutor, error) {
	args := m.Called()
	return args.Get(0).(ledger.QueryExecutor), nil
}

// NewHistoryQueryExecutor history query executor
func (m *mockLedger) NewHistoryQueryExecutor() (ledger.HistoryQueryExecutor, error) {
	args := m.Called()
	return args.Get(0).(ledger.HistoryQueryExecutor), nil
}

// GetPvtDataAndBlockByNum retrieves pvt data and block
func (m *mockLedger) GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error) {
	args := m.Called()
	return args.Get(0).(*ledger.BlockAndPvtData), nil
}

// GetPvtDataByNum retrieves the pvt data
func (m *mockLedger) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	args := m.Called()
	return args.Get(0).([]*ledger.TxPvtData), nil
}

// CommitWithPvtData commits the block and the corresponding pvt data in an atomic operation
func (m *mockLedger) CommitWithPvtData(pvtDataAndBlock *ledger.BlockAndPvtData) error {
	return nil
}

// PurgePrivateData purges the private data
func (m *mockLedger) PurgePrivateData(maxBlockNumToRetain uint64) error {
	return nil
}

// PrivateDataMinBlockNum returns the lowest retained endorsement block height
func (m *mockLedger) PrivateDataMinBlockNum() (uint64, error) {
	return 0, nil
}

// Prune prune using policy
func (m *mockLedger) Prune(policy ledger2.PrunePolicy) error {
	return nil
}

func (m *mockLedger) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	args := m.Called()
	return args.Get(0).(*common.BlockchainInfo), nil
}

func (m *mockLedger) GetBlockByNumber(blockNumber uint64) (*common.Block, error) {
	args := m.Called(blockNumber)
	return args.Get(0).(*common.Block), nil
}

func (m *mockLedger) GetBlocksIterator(startBlockNumber uint64) (ledger2.ResultsIterator, error) {
	args := m.Called(startBlockNumber)
	return args.Get(0).(ledger2.ResultsIterator), nil
}

func (m *mockLedger) Close() {

}

func (m *mockLedger) Commit(block *common.Block) error {
	return nil
}

// GetConfigHistoryRetriever returns the ConfigHistoryRetriever
func (m *mockLedger) GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error) {
	args := m.Called()
	return args.Get(0).(ledger.ConfigHistoryRetriever), nil
}

func (m *mockLedger) CommitPvtData(blockPvtData []*ledger.BlockPvtData) ([]*ledger.PvtdataHashMismatch, error) {
	return nil, nil
}

func (m *mockLedger) GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error) {
	args := m.Called()
	return args.Get(0).(ledger.MissingPvtDataTracker), nil
}

// mockQueryExecutor mock of the query executor,
// needed to simulate inability to access state db, e.g.
// the case where due to db failure it's not possible to
// query for state, for example if we would like to query
// the lccc for VSCC info and db is not avaible we expect
// to stop validating block and fail commit procedure with
// an error.
type mockQueryExecutor struct {
	mock.Mock
}

func (exec *mockQueryExecutor) GetState(namespace string, key string) ([]byte, error) {
	args := exec.Called(namespace, key)
	return args.Get(0).([]byte), args.Error(1)
}

func (exec *mockQueryExecutor) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	args := exec.Called(namespace, keys)
	return args.Get(0).([][]byte), args.Error(1)
}

func (exec *mockQueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ledger2.ResultsIterator, error) {
	args := exec.Called(namespace, startKey, endKey)
	return args.Get(0).(ledger2.ResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) GetStateRangeScanIteratorWithMetadata(namespace, startKey, endKey string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	args := exec.Called(namespace, startKey, endKey, metadata)
	return args.Get(0).(ledger.QueryResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) ExecuteQuery(namespace, query string) (ledger2.ResultsIterator, error) {
	args := exec.Called(namespace)
	return args.Get(0).(ledger2.ResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	args := exec.Called(namespace, query, metadata)
	return args.Get(0).(ledger.QueryResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) GetPrivateData(namespace, collection, key string) ([]byte, error) {
	args := exec.Called(namespace, collection, key)
	return args.Get(0).([]byte), args.Error(1)
}

func (exec *mockQueryExecutor) GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([][]byte, error) {
	args := exec.Called(namespace, collection, keys)
	return args.Get(0).([][]byte), args.Error(1)
}

func (exec *mockQueryExecutor) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (ledger2.ResultsIterator, error) {
	args := exec.Called(namespace, collection, startKey, endKey)
	return args.Get(0).(ledger2.ResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) ExecuteQueryOnPrivateData(namespace, collection, query string) (ledger2.ResultsIterator, error) {
	args := exec.Called(namespace, collection, query)
	return args.Get(0).(ledger2.ResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) Done() {
}

func (exec *mockQueryExecutor) GetStateMetadata(namespace, key string) (map[string][]byte, error) {
	return nil, nil
}

func (exec *mockQueryExecutor) GetPrivateDataMetadata(namespace, collection, key string) (map[string][]byte, error) {
	return nil, nil
}

func createCustomSupportAndLedger(t *testing.T) (*mocktxvalidator.Support, ledger.PeerLedger) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/validatortest")
	ledgermgmt.InitializeTestEnv()
	gb, err := ctxt.MakeGenesisBlock("TestLedger")
	assert.NoError(t, err)
	l, err := ledgermgmt.CreateLedger(gb)
	assert.NoError(t, err)

	identity := &mocks2.Identity{}
	identity.GetIdentifierReturns(&msp.IdentityIdentifier{
		Mspid: "SampleOrg",
		Id:    "foo",
	})
	mspManager := &mocks2.MSPManager{}
	mspManager.DeserializeIdentityReturns(identity, nil)
	support := &mocktxvalidator.Support{LedgerVal: l, ACVal: &mockconfig.MockApplicationCapabilities{}, MSPManagerVal: mspManager}
	return support, l
}

func TestDynamicCapabilitiesAndMSP(t *testing.T) {
	factory := &mocks.PluginFactory{}
	factory.On("New").Return(&testdata.SampleValidationPlugin{})
	pm := &mocks.PluginMapper{}
	pm.On("PluginFactoryByName", txvalidator.PluginName("vscc")).Return(factory)

	support, l := createCustomSupportAndLedger(t)
	defer l.Close()

	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{support, semaphore.NewWeighted(10)}
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()

	v := txvalidator.NewTxValidator(vcs, mp, pm)

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	// Perform a validation of a block
	err := v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)
	// Record the number of times the capabilities and the MSP Manager were invoked
	capabilityInvokeCount := support.CapabilitiesInvokeCount()
	mspManagerInvokeCount := support.MSPManagerInvokeCount()

	// Perform another validation pass, and ensure it is valid
	err = v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)

	// Ensure that the capabilities were retrieved from the support twice,
	// which proves that the capabilities are dynamically retrieved from the support each time
	assert.Equal(t, 2*capabilityInvokeCount, support.CapabilitiesInvokeCount())
	// Ensure that the MSP Manager was retrieved from the support twice,
	// which proves that the MSP Manager is dynamically retrieved from the support each time
	assert.Equal(t, 2*mspManagerInvokeCount, support.MSPManagerInvokeCount())
}

// TestLedgerIsNoAvailable simulates and provides a test for following scenario,
// which is based on FAB-535. Test checks the validation path which expects that
// DB won't available while trying to lookup for VSCC from LCCC and therefore
// transaction validation will have to fail. In such case the outcome should be
// the error return from validate block method and proccessing of transactions
// has to stop. There is suppose to be clear indication of the failure with error
// returned from the function call.
func TestLedgerIsNoAvailable(t *testing.T) {
	theLedger := new(mockLedger)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()
	pm := &mocks.PluginMapper{}
	validator := txvalidator.NewTxValidator(vcs, mp, pm)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, ledger.NotFoundInIndexErr(""))

	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", mock.Anything, mock.Anything).Return([]byte{}, errors.New("Unable to connect to DB"))
	theLedger.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := validator.Validate(b)

	assertion := assert.New(t)
	// We suppose to get the error which indicates we cannot commit the block
	assertion.Error(err)
	// The error exptected to be of type VSCCInfoLookupFailureError
	assertion.NotNil(err.(*commonerrors.VSCCInfoLookupFailureError))
}

func TestLedgerIsNotAvailableForCheckingTxidDuplicate(t *testing.T) {
	theLedger := new(mockLedger)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()
	pm := &mocks.PluginMapper{}
	validator := txvalidator.NewTxValidator(vcs, mp, pm)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, errors.New("Unable to connect to DB"))

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := validator.Validate(b)

	assertion := assert.New(t)
	// We expect a validation error because the ledger wasn't ready to tell us whether there was a tx with that ID or not
	assertion.Error(err)
}

func TestDuplicateTxId(t *testing.T) {
	theLedger := new(mockLedger)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()
	pm := &mocks.PluginMapper{}
	validator := txvalidator.NewTxValidator(vcs, mp, pm)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, nil)

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := validator.Validate(b)

	assertion := assert.New(t)
	// We expect no validation error because we simply mark the tx as invalid
	assertion.NoError(err)

	// We expect the tx to be invalid because of a duplicate txid
	txsfltr := lutils.TxValidationFlags(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assertion.True(txsfltr.IsInvalid(0))
	assertion.True(txsfltr.Flag(0) == peer.TxValidationCode_DUPLICATE_TXID)
}

func TestValidationInvalidEndorsing(t *testing.T) {
	theLedger := new(mockLedger)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()
	pm := &mocks.PluginMapper{}
	factory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	factory.On("New").Return(plugin)
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("invalid tx"))
	pm.On("PluginFactoryByName", txvalidator.PluginName("vscc")).Return(factory)
	validator := txvalidator.NewTxValidator(vcs, mp, pm)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, ledger.NotFoundInIndexErr(""))

	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}

	cdbytes := utils.MarshalOrPanic(cd)

	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", "lscc", ccID).Return(cdbytes, nil)
	theLedger.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	// Keep default callback
	err := validator.Validate(b)
	// Restore default callback
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func createMockLedger(t *testing.T, ccID string) *mockLedger {
	l := new(mockLedger)
	l.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, ledger.NotFoundInIndexErr(""))
	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}

	cdbytes := utils.MarshalOrPanic(cd)
	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", "lscc", ccID).Return(cdbytes, nil)
	l.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)
	return l
}

func TestValidationPluginExecutionError(t *testing.T) {
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	l, v := setupLedgerAndValidatorExplicit(t, &mockconfig.MockApplicationCapabilities{}, plugin)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"
	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&validation.ExecutionFailureError{
		Reason: "I/O error",
	})

	err := v.Validate(b)
	executionErr := err.(*commonerrors.VSCCExecutionFailureError)
	assert.Contains(t, executionErr.Error(), "I/O error")
}

func TestValidationPluginNotFound(t *testing.T) {
	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	l := createMockLedger(t, ccID)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: l, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	pm := &mocks.PluginMapper{}
	pm.On("PluginFactoryByName", txvalidator.PluginName("vscc")).Return(nil)
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()
	validator := txvalidator.NewTxValidator(vcs, mp, pm)
	err := validator.Validate(b)
	executionErr := err.(*commonerrors.VSCCExecutionFailureError)
	assert.Contains(t, executionErr.Error(), "plugin with name vscc wasn't found")
}

var signer msp.SigningIdentity

var signerSerialized []byte

func TestMain(m *testing.M) {
	msptesttools.LoadMSPSetupForTesting()

	var err error
	signer, err = mgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Could not get signer")
		os.Exit(-1)
		return
	}

	signerSerialized, err = signer.Serialize()
	if err != nil {
		fmt.Println("Could not serialize identity")
		os.Exit(-1)
		return
	}

	os.Exit(m.Run())
}
