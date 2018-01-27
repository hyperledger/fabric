/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package txvalidator

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
	"github.com/hyperledger/fabric/core/chaincode/shim"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/mocks/ccprovider"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
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

func setupLedgerAndValidator(t *testing.T) (ledger.PeerLedger, Validator) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/validatortest")
	ledgermgmt.InitializeTestEnv()
	gb, err := ctxt.MakeGenesisBlock("TestLedger")
	assert.NoError(t, err)
	theLedger, err := ledgermgmt.CreateLedger(gb)
	assert.NoError(t, err)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}
	theValidator := NewTxValidator(vcs)

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

func getEnvWithType(ccID string, res []byte, pType common.HeaderType, t *testing.T) *common.Envelope {
	// get a toy proposal
	prop, err := getProposalWithType(ccID, pType)
	assert.NoError(t, err)

	response := &peer.Response{Status: 200}

	// endorse it to get a proposal response
	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, response, res, nil, &peer.ChaincodeID{Name: ccID, Version: ccVersion}, nil, signer)
	assert.NoError(t, err)

	// assemble a transaction from that proposal and endorsement
	tx, err := utils.CreateSignedTx(prop, signer, presp)
	assert.NoError(t, err)

	return tx
}

func getEnv(ccID string, res []byte, t *testing.T) *common.Envelope {
	return getEnvWithType(ccID, res, common.HeaderType_ENDORSER_TRANSACTION, t)
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
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	tx := getEnv(ccID, []byte("barf"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNoPolicy(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfo(l, ccID, nil, t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeOK(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeOKPvtDataOnly(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	v.(*txValidator).support.(struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}).ACVal = &mockconfig.MockApplicationCapabilities{PrivateChannelDataRv: true}

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToPvtAndHashedWriteSet(ccID, "mycollection", "somekey", nil)
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	assert.NoError(t, err)

	tx := getEnv(ccID, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	v.(*txValidator).vscc.(*vsccValidatorImpl).ccprovider.(*ccprovider.MockCcProviderImpl).ExecuteResultProvider = nil
	v.(*txValidator).vscc.(*vsccValidatorImpl).ccprovider.(*ccprovider.MockCcProviderImpl).ExecuteChaincodeResponse = &peer.Response{Status: shim.ERROR}

	err = v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeOKSCC(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "lscc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeNOKWritesToLSCC(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID, "lscc"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKWritesToESCC(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID, "escc"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKWritesToNotExt(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID, "notext"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKInvokesNotExt(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "notext"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKInvokesEmptyCCName(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := ""

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNOKExpiredCC(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "vscc", "badversion", signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_EXPIRED_CHAINCODE)
}

func TestInvokeNOKBogusActions(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, []byte("barf"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNOKCCDoesntExist(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNOKVSCCUnspecified(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "", ccVersion, signedByAnyMember([]string{"DEFAULT"}), t)

	tx := getEnv(ccID, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	err := v.Validate(b)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNoBlock(t *testing.T) {
	l, v := setupLedgerAndValidator(t)
	defer ledgermgmt.CleanupTestEnv()
	defer l.Close()

	err := v.Validate(&common.Block{Data: &common.BlockData{Data: [][]byte{}}})
	assert.NoError(t, err)
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

func (exec *mockQueryExecutor) ExecuteQuery(namespace, query string) (ledger2.ResultsIterator, error) {
	args := exec.Called(namespace)
	return args.Get(0).(ledger2.ResultsIterator), args.Error(1)
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
	validator := NewTxValidator(vcs)

	ccID := "mycc"
	tx := getEnv(ccID, createRWset(t, ccID), t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, errors.New("Cannot find the transaction"))

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

func TestValidationInvalidEndorsing(t *testing.T) {
	theLedger := new(mockLedger)
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}, semaphore.NewWeighted(10)}
	validator := NewTxValidator(vcs)

	ccID := "mycc"
	tx := getEnv(ccID, createRWset(t, ccID), t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, errors.New("Cannot find the transaction"))

	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"DEFAULT"}),
	}

	cdbytes := utils.MarshalOrPanic(cd)

	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", "lscc", ccID).Return(cdbytes, nil)
	theLedger.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	// Keep default callback
	c := executeChaincodeProvider.getCallback()
	executeChaincodeProvider.setCallback(func() (*peer.Response, *peer.ChaincodeEvent, error) {
		return &peer.Response{Status: shim.ERROR}, nil, nil
	})
	err := validator.Validate(b)
	// Restore default callback
	executeChaincodeProvider.setCallback(c)
	assert.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestValidationResourceUpdate(t *testing.T) {
	theLedger := new(mockLedger)
	sup := &mocktxvalidator.Support{LedgerVal: theLedger, ACVal: &mockconfig.MockApplicationCapabilities{}}
	vcs := struct {
		*mocktxvalidator.Support
		*semaphore.Weighted
	}{sup, semaphore.NewWeighted(10)}
	validator := NewTxValidator(vcs)

	ccID := "mycc"
	tx := getEnvWithType(ccID, createRWset(t, ccID), common.HeaderType_PEER_RESOURCE_UPDATE, t)

	theLedger.On("GetTransactionByID", mock.Anything).Return(&peer.ProcessedTransaction{}, errors.New("Cannot find the transaction"))

	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"DEFAULT"}),
	}

	cdbytes := utils.MarshalOrPanic(cd)

	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", "lscc", ccID).Return(cdbytes, nil)
	theLedger.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)

	b1 := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}
	b2 := &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(tx)}}}

	// Keep default callback
	c := executeChaincodeProvider.getCallback()
	executeChaincodeProvider.setCallback(func() (*peer.Response, *peer.ChaincodeEvent, error) {
		return &peer.Response{Status: shim.ERROR}, nil, nil
	})
	err := validator.Validate(b1)
	assert.NoError(t, err)
	sup.ACVal = &mockconfig.MockApplicationCapabilities{ResourcesTreeRv: true}
	err = validator.Validate(b2)
	assert.NoError(t, err)
	// Restore default callback
	executeChaincodeProvider.setCallback(c)
	assertInvalid(b1, t, peer.TxValidationCode_UNSUPPORTED_TX_PAYLOAD)
	assertValid(b2, t)
}

type ccResultCallback func() (*peer.Response, *peer.ChaincodeEvent, error)

type ccExecuteChaincode struct {
	executeChaincodeCalback ccResultCallback
}

func (cc *ccExecuteChaincode) ExecuteChaincodeResult() (*peer.Response, *peer.ChaincodeEvent, error) {
	return cc.executeChaincodeCalback()
}

func (cc *ccExecuteChaincode) getCallback() ccResultCallback {
	return cc.executeChaincodeCalback
}

func (cc *ccExecuteChaincode) setCallback(calback ccResultCallback) {
	cc.executeChaincodeCalback = calback
}

var signer msp.SigningIdentity

var signerSerialized []byte

var executeChaincodeProvider = &ccExecuteChaincode{
	executeChaincodeCalback: func() (*peer.Response, *peer.ChaincodeEvent, error) {
		return &peer.Response{Status: shim.OK}, nil, nil
	},
}

func TestMain(m *testing.M) {
	sysccprovider.RegisterSystemChaincodeProviderFactory(&scc.MocksccProviderFactory{})
	ccp.RegisterChaincodeProviderFactory(&ccprovider.MockCcProviderFactory{executeChaincodeProvider})

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
