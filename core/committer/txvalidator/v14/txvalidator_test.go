/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/semaphore"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	ledger2 "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	"github.com/hyperledger/fabric/core/mocks/validator"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func testValidationWithNTXes(t *testing.T, ledger ledger2.PeerLedger, gbHash []byte, nBlocks int) {
	txid := util2.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()
	pubSimulationResBytes, _ := simRes.GetPubSimulationBytes()
	_, err := testutil.ConstructBytesProposalResponsePayload("v1", pubSimulationResBytes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mockVsccValidator := &validator.MockVsccValidator{}
	mockCapabilities := &mocks.ApplicationCapabilities{}
	mockCapabilities.On("ForbidDuplicateTXIdInBlock").Return(false)
	tValidator := &TxValidator{
		ChannelID:        "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{LedgerVal: ledger, ACVal: mockCapabilities},
		Vscc:             mockVsccValidator,
		CryptoProvider:   cryptoProvider,
	}

	bcInfo, _ := ledger.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	sr := [][]byte{}
	for i := 0; i < nBlocks; i++ {
		sr = append(sr, pubSimulationResBytes)
	}
	block := testutil.ConstructBlock(t, 1, gbHash, sr, true)

	tValidator.Validate(block)

	txsfltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	for i := 0; i < nBlocks; i++ {
		require.True(t, txsfltr.IsSetTo(i, peer.TxValidationCode_VALID))
	}
}

func TestDetectTXIdDuplicates(t *testing.T) {
	txids := []string{"", "1", "2", "3", "", "2", ""}
	txsfltr := txflags.New(len(txids))
	markTXIdDuplicates(txids, txsfltr)
	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(2, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(3, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(4, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(5, peer.TxValidationCode_DUPLICATE_TXID))
	require.True(t, txsfltr.IsSetTo(6, peer.TxValidationCode_NOT_VALIDATED))

	txids = []string{"", "1", "2", "3", "", "21", ""}
	txsfltr = txflags.New(len(txids))
	markTXIdDuplicates(txids, txsfltr)
	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(2, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(3, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(4, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(5, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(6, peer.TxValidationCode_NOT_VALIDATED))
}

func TestBlockValidationDuplicateTXId(t *testing.T) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	defer cleanup()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, _ := ledgerMgr.CreateLedger("TestLedger", gb)
	defer ledger.Close()

	txid := util2.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()
	pubSimulationResBytes, _ := simRes.GetPubSimulationBytes()
	_, err := testutil.ConstructBytesProposalResponsePayload("v1", pubSimulationResBytes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mockVsccValidator := &validator.MockVsccValidator{}
	acv := &mocks.ApplicationCapabilities{}
	tValidator := &TxValidator{
		ChannelID:        "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{LedgerVal: ledger, ACVal: acv},
		Vscc:             mockVsccValidator,
		CryptoProvider:   cryptoProvider,
	}

	bcInfo, _ := ledger.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	envs := []*common.Envelope{}
	env, _, err := testutil.ConstructTransaction(t, pubSimulationResBytes, "", true)
	require.NoError(t, err)
	envs = append(envs, env)
	envs = append(envs, env)
	block := testutil.NewBlock(envs, 1, gbHash)

	acv.On("ForbidDuplicateTXIdInBlock").Return(false).Once()
	tValidator.Validate(block)

	txsfltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_VALID))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_VALID))

	acv.On("ForbidDuplicateTXIdInBlock").Return(true)
	tValidator.Validate(block)

	txsfltr = txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_VALID))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_DUPLICATE_TXID))
}

func TestBlockValidation(t *testing.T) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	defer cleanup()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, _ := ledgerMgr.CreateLedger("TestLedger", gb)
	defer ledger.Close()

	// here we test validation of a block with a single tx
	testValidationWithNTXes(t, ledger, gbHash, 1)
}

func TestParallelBlockValidation(t *testing.T) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	defer cleanup()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, _ := ledgerMgr.CreateLedger("TestLedger", gb)
	defer ledger.Close()

	// here we test validation of a block with 128 txes
	testValidationWithNTXes(t, ledger, gbHash, 128)
}

func TestVeryLargeParallelBlockValidation(t *testing.T) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	defer cleanup()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	gbHash := protoutil.BlockHeaderHash(gb.Header)
	ledger, _ := ledgerMgr.CreateLedger("TestLedger", gb)
	defer ledger.Close()

	// here we test validation of a block with 4096 txes,
	// which is larger than both the number of workers in
	// the pool and the buffer in the channels
	testValidationWithNTXes(t, ledger, gbHash, 4096)
}

func TestTxValidationFailure_InvalidTxid(t *testing.T) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	defer cleanup()

	gb, _ := test.MakeGenesisBlock("TestLedger")
	ledger, _ := ledgerMgr.CreateLedger("TestLedger", gb)

	defer ledger.Close()

	mockCapabilities := &mocks.ApplicationCapabilities{}
	mockCapabilities.On("ForbidDuplicateTXIdInBlock").Return(false)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	tValidator := &TxValidator{
		ChannelID:        "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{LedgerVal: ledger, ACVal: mockCapabilities},
		Vscc:             &validator.MockVsccValidator{},
		CryptoProvider:   cryptoProvider,
	}

	mockSigner, err := mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	require.NoError(t, err)
	mockSignerSerialized, err := mockSigner.Serialize()
	require.NoError(t, err)

	// Create simple endorsement transaction
	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
				TxId:      "INVALID TXID!!!",
				Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
				ChannelId: "testchannelid",
			}),
			SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{
				Nonce:   []byte("nonce"),
				Creator: mockSignerSerialized,
			}),
		},
		Data: []byte("test"),
	}

	payloadBytes, err := proto.Marshal(payload)

	// Check marshaling didn't fail
	require.NoError(t, err)

	sig, err := mockSigner.Sign(payloadBytes)
	require.NoError(t, err)

	// Envelope the payload
	envelope := &common.Envelope{
		Payload:   payloadBytes,
		Signature: sig,
	}

	envelopeBytes, err := proto.Marshal(envelope)

	// Check marshaling didn't fail
	require.NoError(t, err)

	block := &common.Block{
		Data: &common.BlockData{
			// Enconde transactions
			Data: [][]byte{envelopeBytes},
		},
	}

	block.Header = &common.BlockHeader{
		Number:   0,
		DataHash: protoutil.BlockDataHash(block.Data),
	}

	// Initialize metadata
	protoutil.InitBlockMetadata(block)
	txsFilter := txflags.NewWithValues(len(block.Data.Data), peer.TxValidationCode_VALID)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	// Commit block to the ledger
	ledger.CommitLegacy(&ledger2.BlockAndPvtData{Block: block}, &ledger2.CommitOptions{})

	// Validation should invalidate transaction,
	// because it's already committed
	tValidator.Validate(block)

	txsfltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	require.True(t, txsfltr.IsInvalid(0))

	// We expect the tx to be invalid because of a bad txid
	require.True(t, txsfltr.Flag(0) == peer.TxValidationCode_BAD_PROPOSAL_TXID)
}

func createCCUpgradeEnvelope(channelID, chaincodeName, chaincodeVersion string, signer msp.SigningIdentity) (*common.Envelope, error) {
	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	spec := &peer.ChaincodeSpec{
		Type: peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value["GOLANG"]),
		ChaincodeId: &peer.ChaincodeID{
			Path:    "github.com/codePath",
			Name:    chaincodeName,
			Version: chaincodeVersion,
		},
	}

	cds := &peer.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: []byte{}}
	prop, _, err := protoutil.CreateUpgradeProposalFromCDS(channelID, cds, creator, []byte{}, []byte{}, []byte{}, nil)
	if err != nil {
		return nil, err
	}

	proposalResponse := &peer.ProposalResponse{
		Response: &peer.Response{
			Status: 200, // endorsed successfully
		},
		Endorsement: &peer.Endorsement{},
	}

	return protoutil.CreateSignedTx(prop, signer, proposalResponse)
}

func TestGetTxCCInstance(t *testing.T) {
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	signer, err := mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	require.NoError(t, err)

	channelID := "testchannelid"
	upgradeCCName := "mycc"
	upgradeCCVersion := "v1"

	env, err := createCCUpgradeEnvelope(channelID, upgradeCCName, upgradeCCVersion, signer)
	require.NoError(t, err)

	// get the payload from the envelope
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)

	expectInvokeCCIns := &sysccprovider.ChaincodeInstance{
		ChannelID:        channelID,
		ChaincodeName:    "lscc",
		ChaincodeVersion: "",
	}
	expectUpgradeCCIns := &sysccprovider.ChaincodeInstance{
		ChannelID:        channelID,
		ChaincodeName:    upgradeCCName,
		ChaincodeVersion: upgradeCCVersion,
	}

	tValidator := &TxValidator{
		CryptoProvider: cryptoProvider,
	}
	invokeCCIns, upgradeCCIns, err := tValidator.getTxCCInstance(payload)
	if err != nil {
		t.Fatalf("Get chaincode from tx error: %s", err)
	}
	require.EqualValues(t, expectInvokeCCIns, invokeCCIns)
	require.EqualValues(t, expectUpgradeCCIns, upgradeCCIns)
}

func TestInvalidTXsForUpgradeCC(t *testing.T) {
	txsChaincodeNames := map[int]*sysccprovider.ChaincodeInstance{
		0: {ChannelID: "chain0", ChaincodeName: "cc0", ChaincodeVersion: "v0"}, // invoke cc0/chain0:v0, should not be affected by upgrade tx in other chain
		1: {ChannelID: "chain1", ChaincodeName: "cc0", ChaincodeVersion: "v0"}, // invoke cc0/chain1:v0, should be invalided by cc1/chain1 upgrade tx
		2: {ChannelID: "chain1", ChaincodeName: "lscc", ChaincodeVersion: ""},  // upgrade cc0/chain1 to v1, should be invalided by latter cc0/chain1 upgtade tx
		3: {ChannelID: "chain1", ChaincodeName: "cc0", ChaincodeVersion: "v0"}, // invoke cc0/chain1:v0, should be invalided by cc1/chain1 upgrade tx
		4: {ChannelID: "chain1", ChaincodeName: "cc0", ChaincodeVersion: "v1"}, // invoke cc0/chain1:v1, should be invalided by cc1/chain1 upgrade tx
		5: {ChannelID: "chain1", ChaincodeName: "cc1", ChaincodeVersion: "v0"}, // invoke cc1/chain1:v0, should not be affected by other chaincode upgrade tx
		6: {ChannelID: "chain1", ChaincodeName: "lscc", ChaincodeVersion: ""},  // upgrade cc0/chain1 to v2, should be invalided by latter cc0/chain1 upgtade tx
		7: {ChannelID: "chain1", ChaincodeName: "lscc", ChaincodeVersion: ""},  // upgrade cc0/chain1 to v3
	}
	upgradedChaincodes := map[int]*sysccprovider.ChaincodeInstance{
		2: {ChannelID: "chain1", ChaincodeName: "cc0", ChaincodeVersion: "v1"},
		6: {ChannelID: "chain1", ChaincodeName: "cc0", ChaincodeVersion: "v2"},
		7: {ChannelID: "chain1", ChaincodeName: "cc0", ChaincodeVersion: "v3"},
	}

	txsfltr := txflags.New(8)
	txsfltr.SetFlag(0, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(1, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(2, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(3, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(4, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(5, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(6, peer.TxValidationCode_VALID)
	txsfltr.SetFlag(7, peer.TxValidationCode_VALID)

	expectTxsFltr := txflags.New(8)
	expectTxsFltr.SetFlag(0, peer.TxValidationCode_VALID)
	expectTxsFltr.SetFlag(1, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(2, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(3, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(4, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(5, peer.TxValidationCode_VALID)
	expectTxsFltr.SetFlag(6, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
	expectTxsFltr.SetFlag(7, peer.TxValidationCode_VALID)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	tValidator := &TxValidator{
		CryptoProvider: cryptoProvider,
	}
	tValidator.invalidTXsForUpgradeCC(txsChaincodeNames, upgradedChaincodes, txsfltr)

	require.EqualValues(t, expectTxsFltr, txsfltr)
}

func constructLedgerMgrWithTestDefaults(t *testing.T) (*ledgermgmt.LedgerMgr, func()) {
	testDir := t.TempDir()
	initializer := ledgermgmttest.NewInitializer(testDir)
	ledgerMgr := ledgermgmt.NewLedgerMgr(initializer)
	cleanup := func() {
		ledgerMgr.Close()
	}
	return ledgerMgr, cleanup
}
