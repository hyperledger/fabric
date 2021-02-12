/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator_test

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	protosmsp "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	tmocks "github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	txvalidatorplugin "github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	txvalidatorv20 "github.com/hyperledger/fabric/core/committer/txvalidator/v20"
	txvalidatormocks "github.com/hyperledger/fabric/core/committer/txvalidator/v20/mocks"
	plugindispatchermocks "github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher/mocks"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	"github.com/hyperledger/fabric/core/scc/lscc"
	supportmocks "github.com/hyperledger/fabric/discovery/support/mocks"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func signedByAnyMember(ids []string) []byte {
	p := policydsl.SignedByAnyMember(ids)
	return protoutil.MarshalOrPanic(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: p}})
}

func v20Capabilities() *tmocks.ApplicationCapabilities {
	ac := &tmocks.ApplicationCapabilities{}
	ac.On("V1_2Validation").Return(true)
	ac.On("V1_3Validation").Return(true)
	ac.On("V2_0Validation").Return(true)
	ac.On("PrivateChannelData").Return(true)
	ac.On("KeyLevelEndorsement").Return(true)
	return ac
}

func createRWset(t *testing.T, ccnames ...string) []byte {
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	for _, ccname := range ccnames {
		rwsetBuilder.AddToWriteSet(ccname, "key", []byte("value"))
	}
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	require.NoError(t, err)
	return rwsetBytes
}

func getProposalWithType(ccID string, pType common.HeaderType) (*peer.Proposal, error) {
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: ccID, Version: ccVersion},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("func")}},
			Type:        peer.ChaincodeSpec_GOLANG,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(pType, "testchannelid", cis, signerSerialized)
	return proposal, err
}

const ccVersion = "1.0"

func getEnvWithType(ccID string, event []byte, res []byte, pType common.HeaderType, t *testing.T) *common.Envelope {
	// get a toy proposal
	prop, err := getProposalWithType(ccID, pType)
	require.NoError(t, err)

	response := &peer.Response{Status: 200}

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, res, event, &peer.ChaincodeID{Name: ccID, Version: ccVersion}, signer)
	require.NoError(t, err)

	// assemble a transaction from that proposal and endorsement
	tx, err := protoutil.CreateSignedTx(prop, signer, presp)
	require.NoError(t, err)

	return tx
}

func getEnv(ccID string, event []byte, res []byte, t *testing.T) *common.Envelope {
	return getEnvWithType(ccID, event, res, common.HeaderType_ENDORSER_TRANSACTION, t)
}

func getEnvWithSigner(ccID string, event []byte, res []byte, sig msp.SigningIdentity, t *testing.T) *common.Envelope {
	// get a toy proposal
	pType := common.HeaderType_ENDORSER_TRANSACTION
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: ccID, Version: ccVersion},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("func")}},
			Type:        peer.ChaincodeSpec_GOLANG,
		},
	}

	sID, err := sig.Serialize()
	require.NoError(t, err)
	prop, _, err := protoutil.CreateProposalFromCIS(pType, "foochain", cis, sID)
	require.NoError(t, err)

	response := &peer.Response{Status: 200}

	// endorse it to get a proposal response
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, response, res, event, &peer.ChaincodeID{Name: ccID, Version: ccVersion}, sig)
	require.NoError(t, err)

	// assemble a transaction from that proposal and endorsement
	tx, err := protoutil.CreateSignedTx(prop, sig, presp)
	require.NoError(t, err)

	return tx
}

func assertInvalid(block *common.Block, t *testing.T, code peer.TxValidationCode) {
	txsFilter := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	require.True(t, txsFilter.IsInvalid(0))
	require.True(t, txsFilter.IsSetTo(0, code))
}

func assertValid(block *common.Block, t *testing.T) {
	txsFilter := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	require.False(t, txsFilter.IsInvalid(0))
}

func setupValidator() (*txvalidatorv20.TxValidator, *txvalidatormocks.QueryExecutor, *supportmocks.Identity, *txvalidatormocks.CollectionResources) {
	mspmgr := &supportmocks.MSPManager{}
	mockID := &supportmocks.Identity{}
	mockID.SatisfiesPrincipalReturns(nil)
	mockID.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(mockID, nil)

	return setupValidatorWithMspMgr(mspmgr, mockID)
}

func setupValidatorWithMspMgr(mspmgr msp.MSPManager, mockID *supportmocks.Identity) (*txvalidatorv20.TxValidator, *txvalidatormocks.QueryExecutor, *supportmocks.Identity, *txvalidatormocks.CollectionResources) {
	pm := &plugindispatchermocks.Mapper{}
	factory := &plugindispatchermocks.PluginFactory{}
	pm.On("FactoryByName", txvalidatorplugin.Name("vscc")).Return(factory)
	factory.On("New").Return(&builtin.DefaultValidation{})

	mockQE := &txvalidatormocks.QueryExecutor{}
	mockQE.On("Done").Return(nil)
	mockQE.On("GetState", "lscc", "lscc").Return(nil, nil)
	mockQE.On("GetState", "lscc", "escc").Return(nil, nil)

	mockLedger := &txvalidatormocks.LedgerResources{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
	mockLedger.On("NewQueryExecutor").Return(mockQE, nil)

	mockCpmg := &plugindispatchermocks.ChannelPolicyManagerGetter{}
	mockCpmg.On("Manager", mock.Anything).Return(&txvalidatormocks.PolicyManager{})

	mockCR := &txvalidatormocks.CollectionResources{}

	cryptoProvider, _ := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	v := txvalidatorv20.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{ACVal: v20Capabilities(), MSPManagerVal: mspmgr},
		mockLedger,
		&lscc.SCC{BCCSP: cryptoProvider},
		mockCR,
		pm,
		mockCpmg,
		cryptoProvider,
	)

	return v, mockQE, mockID, mockCR
}

func TestInvokeBadRWSet(t *testing.T) {
	ccID := "mycc"

	v, _, _, _ := setupValidator()

	tx := getEnv(ccID, nil, []byte("barf"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 1}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNoPolicy(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  nil,
	}), nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeOK(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeNOKDuplicateNs(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

	// note that this read-write set has two read-write sets for the same namespace and key
	txrws := &rwset.TxReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsRwset: []*rwset.NsReadWriteSet{
			{
				Namespace: "mycc",
				Rwset: protoutil.MarshalOrPanic(&kvrwset.KVRWSet{
					Writes: []*kvrwset.KVWrite{
						{
							Key:   "foo",
							Value: []byte("bar1"),
						},
					},
				}),
			},
			{
				Namespace: "mycc",
				Rwset: protoutil.MarshalOrPanic(&kvrwset.KVRWSet{
					Writes: []*kvrwset.KVWrite{
						{
							Key:   "foo",
							Value: []byte("bar2"),
						},
					},
				}),
			},
		},
	}

	tx := getEnv(ccID, nil, protoutil.MarshalOrPanic(txrws), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNoRWSet(t *testing.T) {
	ccID := "mycc"

	v, mockQE, mockID, _ := setupValidator()
	mockID.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

	tx := getEnv(ccID, nil, createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

// SerializedIdentity mock for the parallel validation test
type mockSI struct {
	SerializedID []byte
	MspID        string
	SatPrinError error
}

func (msi *mockSI) ExpiresAt() time.Time {
	return time.Now()
}

func (msi *mockSI) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{
		Mspid: msi.MspID,
		Id:    "",
	}
}

func (msi *mockSI) GetMSPIdentifier() string {
	return msi.MspID
}

func (msi *mockSI) Validate() error {
	return nil
}

func (msi *mockSI) GetOrganizationalUnits() []*msp.OUIdentifier {
	return nil
}

func (msi *mockSI) Anonymous() bool {
	return false
}

func (msi *mockSI) Verify(msg []byte, sig []byte) error {
	return nil
}

func (msi *mockSI) Serialize() ([]byte, error) {
	sid := &protosmsp.SerializedIdentity{
		Mspid:   msi.MspID,
		IdBytes: msi.SerializedID,
	}
	sidBytes := protoutil.MarshalOrPanic(sid)
	return sidBytes, nil
}

func (msi *mockSI) SatisfiesPrincipal(principal *protosmsp.MSPPrincipal) error {
	return msi.SatPrinError
}

func (msi *mockSI) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (msi *mockSI) GetPublicVersion() msp.Identity {
	return msi
}

// MSP mock for the parallel validation test
type mockMSP struct {
	ID           msp.Identity
	SatPrinError error
	MspID        string
}

func (fake *mockMSP) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	return fake.ID, nil
}

func (fake *mockMSP) IsWellFormed(identity *protosmsp.SerializedIdentity) error {
	return nil
}

func (fake *mockMSP) Setup(config *protosmsp.MSPConfig) error {
	return nil
}

func (fake *mockMSP) GetVersion() msp.MSPVersion {
	return msp.MSPv1_3
}

func (fake *mockMSP) GetType() msp.ProviderType {
	return msp.FABRIC
}

func (fake *mockMSP) GetIdentifier() (string, error) {
	return fake.MspID, nil
}

func (fake *mockMSP) GetDefaultSigningIdentity() (msp.SigningIdentity, error) {
	return nil, nil
}

func (fake *mockMSP) GetTLSRootCerts() [][]byte {
	return nil
}

func (fake *mockMSP) GetTLSIntermediateCerts() [][]byte {
	return nil
}

func (fake *mockMSP) Validate(id msp.Identity) error {
	return nil
}

func (fake *mockMSP) SatisfiesPrincipal(id msp.Identity, principal *protosmsp.MSPPrincipal) error {
	return fake.SatPrinError
}

// parallel validation on a block with a high number of transactions and sbe dependencies among those
func TestParallelValidation(t *testing.T) {
	// number of transactions in the block
	txCnt := 100

	// create two MSPs to control the policy evaluation result, one of them returning an error on SatisfiesPrincipal()
	msp1 := &mockMSP{
		ID: &mockSI{
			MspID:        "Org1",
			SerializedID: []byte("signer0"),
			SatPrinError: nil,
		},
		SatPrinError: nil,
		MspID:        "Org1",
	}
	msp2 := &mockMSP{
		ID: &mockSI{
			MspID:        "Org2",
			SerializedID: []byte("signer1"),
			SatPrinError: errors.New("nope"),
		},
		SatPrinError: errors.New("nope"),
		MspID:        "Org2",
	}
	mgmt.GetManagerForChain("foochain")
	mgr := mgmt.GetManagerForChain("foochain")
	mgr.Setup([]msp.MSP{msp1, msp2})

	vpKey := peer.MetaDataKeys_VALIDATION_PARAMETER.String()
	ccID := "mycc"

	v, mockQE, _, mockCR := setupValidatorWithMspMgr(mgr, nil)

	mockCR.On("CollectionValidationInfo", ccID, "col1", mock.Anything).Return(nil, nil, nil)

	policy := policydsl.SignedByMspPeer("Org1")
	polBytes := protoutil.MarshalOrPanic(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: policy}})
	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  polBytes,
	}), nil)
	mockQE.On("GetStateMetadata", "mycc", mock.Anything).Return(nil, nil)
	mockQE.On("GetPrivateDataMetadataByHash", "mycc", "col1", mock.Anything).Return(nil, nil)

	// create a number of txes
	blockData := make([][]byte, 0, txCnt)
	col := "col1"
	sigID0 := &mockSI{
		SerializedID: []byte("signer0"),
		MspID:        "Org1",
	}
	sigID1 := &mockSI{
		SerializedID: []byte("signer1"),
		MspID:        "Org2",
	}
	for txNum := 0; txNum < txCnt; txNum++ {
		var sig msp.SigningIdentity
		// create rwset for the tx - KVS key depends on the txnum
		key := strconv.Itoa(txNum % 10)
		rwsetBuilder := rwsetutil.NewRWSetBuilder()
		// pick action that we want to do: read / modify the value or the ep
		switch uint(txNum / 10) {
		case 0:
			// set the value of the key (valid)
			rwsetBuilder.AddToWriteSet(ccID, key, []byte("value1"))
			sig = sigID0
		case 1:
			// set the ep of the key (invalid, because Org2's MSP returns principal not satisfied)
			metadata := make(map[string][]byte)
			metadata[vpKey] = signedByAnyMember([]string{"SampleOrg"})
			rwsetBuilder.AddToMetadataWriteSet(ccID, key, metadata)
			sig = sigID1
		case 2:
			// set the value of the key (valid, because the ep change before was invalid)
			rwsetBuilder.AddToWriteSet(ccID, key, []byte("value2"))
			sig = sigID0
		case 3:
			// set the ep of the key (valid)
			metadata := make(map[string][]byte)
			metadata[vpKey] = signedByAnyMember([]string{"Org2"})
			rwsetBuilder.AddToMetadataWriteSet(ccID, key, metadata)
			sig = sigID0
		case 4:
			// set the value of the key (invalid, because the ep change before was valid)
			rwsetBuilder.AddToWriteSet(ccID, key, []byte("value3"))
			sig = &mockSI{
				SerializedID: []byte("signer0"),
				MspID:        "Org1",
			}
		// do the same txes for private data
		case 5:
			// set the value of the key (valid)
			rwsetBuilder.AddToPvtAndHashedWriteSet(ccID, col, key, []byte("value1"))
			sig = sigID0
		case 6:
			// set the ep of the key (invalid, because Org2's MSP returns principal not satisfied)
			metadata := make(map[string][]byte)
			metadata[vpKey] = signedByAnyMember([]string{"SampleOrg"})
			rwsetBuilder.AddToHashedMetadataWriteSet(ccID, col, key, metadata)
			sig = sigID1
		case 7:
			// set the value of the key (valid, because the ep change before was invalid)
			rwsetBuilder.AddToPvtAndHashedWriteSet(ccID, col, key, []byte("value2"))
			sig = sigID0
		case 8:
			// set the ep of the key (valid)
			metadata := make(map[string][]byte)
			metadata[vpKey] = signedByAnyMember([]string{"Org2"})
			rwsetBuilder.AddToHashedMetadataWriteSet(ccID, col, key, metadata)
			sig = sigID0
		case 9:
			// set the value of the key (invalid, because the ep change before was valid)
			rwsetBuilder.AddToPvtAndHashedWriteSet(ccID, col, key, []byte("value3"))
			sig = sigID0
		}
		rwset, err := rwsetBuilder.GetTxSimulationResults()
		require.NoError(t, err)
		rwsetBytes, err := rwset.GetPubSimulationBytes()
		require.NoError(t, err)
		tx := getEnvWithSigner(ccID, nil, rwsetBytes, sig, t)
		blockData = append(blockData, protoutil.MarshalOrPanic(tx))
	}

	// assemble block from all those txes
	b := &common.Block{Data: &common.BlockData{Data: blockData}, Header: &common.BlockHeader{Number: uint64(txCnt)}}

	// validate the block
	err := v.Validate(b)
	require.NoError(t, err)

	// Block metadata array position to store serialized bit array filter of invalid transactions
	txsFilter := txflags.ValidationFlags(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	// tx validity
	for txNum := 0; txNum < txCnt; txNum += 1 {
		switch uint(txNum / 10) {
		case 1:
			fallthrough
		case 4:
			fallthrough
		case 6:
			fallthrough
		case 9:
			require.True(t, txsFilter.IsInvalid(txNum))
		default:
			require.False(t, txsFilter.IsInvalid(txNum))
		}
	}
}

func TestChaincodeEvent(t *testing.T) {
	ccID := "mycc"

	t.Run("MisMatchedName", func(t *testing.T) {
		v, mockQE, _, _ := setupValidator()

		mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
			Name:    ccID,
			Version: ccVersion,
			Vscc:    "vscc",
			Policy:  signedByAnyMember([]string{"SampleOrg"}),
		}), nil)
		mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

		testCCEventMismatchedName(t, v, ccID)
	})

	t.Run("BadBytes", func(t *testing.T) {
		v, mockQE, _, _ := setupValidator()

		mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
			Name:    ccID,
			Version: ccVersion,
			Vscc:    "vscc",
			Policy:  signedByAnyMember([]string{"SampleOrg"}),
		}), nil)
		mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

		testCCEventBadBytes(t, v, ccID)
	})

	t.Run("GoodPath", func(t *testing.T) {
		v, mockQE, _, _ := setupValidator()

		mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
			Name:    ccID,
			Version: ccVersion,
			Vscc:    "vscc",
			Policy:  signedByAnyMember([]string{"SampleOrg"}),
		}), nil)
		mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

		testCCEventGoodPath(t, v, ccID)
	})
}

func testCCEventMismatchedName(t *testing.T, v txvalidator.Validator, ccID string) {
	tx := getEnv(ccID, protoutil.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: "wrong"}), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err) // TODO, convert test so it can check the error text for INVALID_OTHER_REASON
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func testCCEventBadBytes(t *testing.T, v txvalidator.Validator, ccID string) {
	tx := getEnv(ccID, []byte("garbage"), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err) // TODO, convert test so it can check the error text for INVALID_OTHER_REASON
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func testCCEventGoodPath(t *testing.T, v txvalidator.Validator, ccID string) {
	tx := getEnv(ccID, protoutil.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: ccID}), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeOKPvtDataOnly(t *testing.T) {
	ccID := "mycc"

	v, mockQE, mockID, mockCR := setupValidator()
	mockID.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetPrivateDataMetadataByHash", ccID, "mycollection", mock.Anything).Return(nil, nil)

	mockCR.On("CollectionValidationInfo", ccID, "mycollection", mock.Anything).Return(nil, nil, nil)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToPvtAndHashedWriteSet(ccID, "mycollection", "somekey", nil)
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	require.NoError(t, err)

	tx := getEnv(ccID, nil, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err = v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeOKMetaUpdateOnly(t *testing.T) {
	ccID := "mycc"

	v, mockQE, mockID, _ := setupValidator()
	mockID.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", ccID, "somekey").Return(nil, nil)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToMetadataWriteSet(ccID, "somekey", map[string][]byte{})
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	require.NoError(t, err)

	tx := getEnv(ccID, nil, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err = v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeOKPvtMetaUpdateOnly(t *testing.T) {
	ccID := "mycc"

	v, mockQE, mockID, mockCR := setupValidator()
	mockID.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetPrivateDataMetadataByHash", ccID, "mycollection", mock.Anything).Return(nil, nil)

	mockCR.On("CollectionValidationInfo", ccID, "mycollection", mock.Anything).Return(nil, nil, nil)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToHashedMetadataWriteSet(ccID, "mycollection", "somekey", map[string][]byte{})
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	require.NoError(t, err)

	tx := getEnv(ccID, nil, rwsetBytes, t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err = v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestInvokeNOKWritesToLSCC(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)
	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "lscc"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNOKWritesToESCC(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)
	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "escc"), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{Number: 35},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNOKWritesToNotExt(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)
	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetState", "lscc", "notext").Return(nil, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "notext"), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{Number: 35},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNOKInvokesNotExt(t *testing.T) {
	ccID := "notext"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", "notext").Return(nil, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNOKInvokesEmptyCCName(t *testing.T) {
	ccID := ""

	v, _, _, _ := setupValidator()

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNOKBogusActions(t *testing.T) {
	ccID := "ccid"

	v, _, _, _ := setupValidator()

	tx := getEnv(ccID, nil, []byte("barf"), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNOKCCDoesntExist(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()
	mockQE.On("GetState", "lscc", ccID).Return(nil, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNOKVSCCUnspecified(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_CHAINCODE)
}

func TestInvokeNoBlock(t *testing.T) {
	v, _, _, _ := setupValidator()
	err := v.Validate(&common.Block{
		Data:   &common.BlockData{Data: [][]byte{}},
		Header: &common.BlockHeader{},
	})
	require.NoError(t, err)
}

func TestValidateTxWithStateBasedEndorsement(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", ccID, "key").Return(map[string][]byte{peer.MetaDataKeys_VALIDATION_PARAMETER.String(): protoutil.MarshalOrPanic(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: policydsl.RejectAllPolicy}})}, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 3}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestDynamicCapabilitiesAndMSP(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()

	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)
	mockQE.On("GetStateMetadata", ccID, "key").Return(nil, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{Number: 1},
	}

	// Perform a validation of a block
	err := v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
	// Record the number of times the capabilities and the MSP Manager were invoked
	capabilityInvokeCount := v.ChannelResources.(*mocktxvalidator.Support).CapabilitiesInvokeCount()
	mspManagerInvokeCount := v.ChannelResources.(*mocktxvalidator.Support).MSPManagerInvokeCount()

	// Perform another validation pass, and ensure it is valid
	err = v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)

	// Ensure that the capabilities were retrieved from the support twice,
	// which proves that the capabilities are dynamically retrieved from the support each time
	require.Equal(t, 2*capabilityInvokeCount, v.ChannelResources.(*mocktxvalidator.Support).CapabilitiesInvokeCount())
	// Ensure that the MSP Manager was retrieved from the support twice,
	// which proves that the MSP Manager is dynamically retrieved from the support each time
	require.Equal(t, 2*mspManagerInvokeCount, v.ChannelResources.(*mocktxvalidator.Support).MSPManagerInvokeCount())
}

// TestLedgerIsNoAvailable simulates and provides a test for following scenario,
// which is based on FAB-535. Test checks the validation path which expects that
// DB won't available while trying to lookup for VSCC from LCCC and therefore
// transaction validation will have to fail. In such case the outcome should be
// the error return from validate block method and processing of transactions
// has to stop. There is suppose to be clear indication of the failure with error
// returned from the function call.
func TestLedgerIsNotAvailable(t *testing.T) {
	ccID := "mycc"

	v, mockQE, _, _ := setupValidator()
	mockQE.On("GetState", "lscc", ccID).Return(nil, errors.New("Detroit rock city"))

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)

	assertion := require.New(t)
	// We suppose to get the error which indicates we cannot commit the block
	assertion.Error(err)
	// The error exptected to be of type VSCCInfoLookupFailureError
	assertion.NotNil(err.(*commonerrors.VSCCInfoLookupFailureError))
}

func TestLedgerIsNotAvailableForCheckingTxidDuplicate(t *testing.T) {
	ccID := "mycc"

	v, _, _, _ := setupValidator()

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	mockLedger := &txvalidatormocks.LedgerResources{}
	v.LedgerResources = mockLedger
	mockLedger.On("TxIDExists", mock.Anything).Return(false, errors.New("uh, oh"))

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{Number: 1},
	}

	err := v.Validate(b)

	assertion := require.New(t)
	// We expect a validation error because the ledger wasn't ready to tell us whether there was a tx with that ID or not
	assertion.Error(err)
}

func TestDuplicateTxId(t *testing.T) {
	ccID := "mycc"

	v, _, _, _ := setupValidator()

	mockLedger := &txvalidatormocks.LedgerResources{}
	v.LedgerResources = mockLedger
	mockLedger.On("TxIDExists", mock.Anything).Return(true, nil)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)

	assertion := require.New(t)
	// We expect no validation error because we simply mark the tx as invalid
	assertion.NoError(err)

	// We expect the tx to be invalid because of a duplicate txid
	txsfltr := txflags.ValidationFlags(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assertion.True(txsfltr.IsInvalid(0))
	assertion.True(txsfltr.Flag(0) == peer.TxValidationCode_DUPLICATE_TXID)
}

func TestValidationInvalidEndorsing(t *testing.T) {
	ccID := "mycc"

	mspmgr := &supportmocks.MSPManager{}
	mockID := &supportmocks.Identity{}
	mockID.SatisfiesPrincipalReturns(nil)
	mockID.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(mockID, nil)

	pm := &plugindispatchermocks.Mapper{}
	factory := &plugindispatchermocks.PluginFactory{}
	pm.On("FactoryByName", txvalidatorplugin.Name("vscc")).Return(factory)
	plugin := &plugindispatchermocks.Plugin{}
	factory.On("New").Return(plugin)
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("invalid tx"))

	mockQE := &txvalidatormocks.QueryExecutor{}
	mockQE.On("Done").Return(nil)

	mockLedger := &txvalidatormocks.LedgerResources{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
	mockLedger.On("NewQueryExecutor").Return(mockQE, nil)

	mockCpmg := &plugindispatchermocks.ChannelPolicyManagerGetter{}
	mockCpmg.On("Manager", mock.Anything).Return(&txvalidatormocks.PolicyManager{})

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	v := txvalidatorv20.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{ACVal: v20Capabilities(), MSPManagerVal: mspmgr},
		mockLedger,
		&lscc.SCC{BCCSP: cryptoProvider},
		&txvalidatormocks.CollectionResources{},
		pm,
		mockCpmg,
		cryptoProvider,
	)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}

	cdbytes := protoutil.MarshalOrPanic(cd)

	mockQE.On("GetState", "lscc", ccID).Return(cdbytes, nil)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	// Keep default callback
	err = v.Validate(b)
	// Restore default callback
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func TestValidationPluginExecutionError(t *testing.T) {
	ccID := "mycc"

	mspmgr := &supportmocks.MSPManager{}
	mockID := &supportmocks.Identity{}
	mockID.SatisfiesPrincipalReturns(nil)
	mockID.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(mockID, nil)

	pm := &plugindispatchermocks.Mapper{}
	factory := &plugindispatchermocks.PluginFactory{}
	pm.On("FactoryByName", txvalidatorplugin.Name("vscc")).Return(factory)
	plugin := &plugindispatchermocks.Plugin{}
	factory.On("New").Return(plugin)
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&validation.ExecutionFailureError{
		Reason: "I/O error",
	})

	mockQE := &txvalidatormocks.QueryExecutor{}
	mockQE.On("Done").Return(nil)
	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)

	mockLedger := &txvalidatormocks.LedgerResources{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
	mockLedger.On("NewQueryExecutor").Return(mockQE, nil)

	mockCpmg := &plugindispatchermocks.ChannelPolicyManagerGetter{}
	mockCpmg.On("Manager", mock.Anything).Return(&txvalidatormocks.PolicyManager{})

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	v := txvalidatorv20.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{ACVal: v20Capabilities(), MSPManagerVal: mspmgr},
		mockLedger,
		&lscc.SCC{BCCSP: cryptoProvider},
		&txvalidatormocks.CollectionResources{},
		pm,
		mockCpmg,
		cryptoProvider,
	)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err = v.Validate(b)
	executionErr := err.(*commonerrors.VSCCExecutionFailureError)
	require.Contains(t, executionErr.Error(), "I/O error")
}

func TestValidationPluginNotFound(t *testing.T) {
	ccID := "mycc"

	mspmgr := &supportmocks.MSPManager{}
	mockID := &supportmocks.Identity{}
	mockID.SatisfiesPrincipalReturns(nil)
	mockID.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(mockID, nil)

	pm := &plugindispatchermocks.Mapper{}
	pm.On("FactoryByName", txvalidatorplugin.Name("vscc")).Return(nil)

	mockQE := &txvalidatormocks.QueryExecutor{}
	mockQE.On("Done").Return(nil)
	mockQE.On("GetState", "lscc", ccID).Return(protoutil.MarshalOrPanic(&ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}), nil)

	mockLedger := &txvalidatormocks.LedgerResources{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
	mockLedger.On("NewQueryExecutor").Return(mockQE, nil)

	mockCpmg := &plugindispatchermocks.ChannelPolicyManagerGetter{}
	mockCpmg.On("Manager", mock.Anything).Return(&txvalidatormocks.PolicyManager{})

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	v := txvalidatorv20.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{ACVal: v20Capabilities(), MSPManagerVal: mspmgr},
		mockLedger,
		&lscc.SCC{BCCSP: cryptoProvider},
		&txvalidatormocks.CollectionResources{},
		pm,
		mockCpmg,
		cryptoProvider,
	)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err = v.Validate(b)
	executionErr := err.(*commonerrors.VSCCExecutionFailureError)
	require.Contains(t, executionErr.Error(), "plugin with name vscc wasn't found")
}

var signer msp.SigningIdentity

var signerSerialized []byte

func TestMain(m *testing.M) {
	msptesttools.LoadMSPSetupForTesting()

	var err error
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		fmt.Printf("Initialize cryptoProvider bccsp failed: %s", err)
		os.Exit(-1)
		return
	}

	signer, err = mgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
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
