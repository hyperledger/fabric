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
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	ctxt "github.com/hyperledger/fabric/common/configtx/test"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	ledger2 "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	tmocks "github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	vp "github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	txvalidatorv14 "github.com/hyperledger/fabric/core/committer/txvalidator/v14"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v14/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v14/testdata"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	mocks2 "github.com/hyperledger/fabric/discovery/support/mocks"
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
	return protoutil.MarshalOrPanic(p)
}

func preV12Capabilities() *tmocks.ApplicationCapabilities {
	ac := &tmocks.ApplicationCapabilities{}
	ac.On("V1_2Validation").Return(false)
	ac.On("V1_3Validation").Return(false)
	ac.On("V2_0Validation").Return(false)
	ac.On("PrivateChannelData").Return(false)
	ac.On("ForbidDuplicateTXIdInBlock").Return(false)
	ac.On("KeyLevelEndorsement").Return(false)
	return ac
}

func v12Capabilities() *tmocks.ApplicationCapabilities {
	ac := &tmocks.ApplicationCapabilities{}
	ac.On("V1_2Validation").Return(true)
	ac.On("V1_3Validation").Return(false)
	ac.On("V2_0Validation").Return(false)
	ac.On("PrivateChannelData").Return(true)
	ac.On("ForbidDuplicateTXIdInBlock").Return(false)
	ac.On("KeyLevelEndorsement").Return(false)
	return ac
}

func v13Capabilities() *tmocks.ApplicationCapabilities {
	ac := &tmocks.ApplicationCapabilities{}
	ac.On("V1_2Validation").Return(true)
	ac.On("V1_3Validation").Return(true)
	ac.On("V2_0Validation").Return(false)
	ac.On("PrivateChannelData").Return(true)
	ac.On("ForbidDuplicateTXIdInBlock").Return(true)
	ac.On("KeyLevelEndorsement").Return(true)
	return ac
}

func setupLedgerAndValidatorExplicit(t *testing.T, cpb *tmocks.ApplicationCapabilities, plugin validation.Plugin) (ledger.PeerLedger, txvalidator.Validator, func()) {
	return setupLedgerAndValidatorExplicitWithMSP(t, cpb, plugin, nil)
}

func setupLedgerAndValidatorWithPreV12Capabilities(t *testing.T) (ledger.PeerLedger, txvalidator.Validator, func()) {
	return setupLedgerAndValidatorWithCapabilities(t, preV12Capabilities())
}

func setupLedgerAndValidatorWithV12Capabilities(t *testing.T) (ledger.PeerLedger, txvalidator.Validator, func()) {
	return setupLedgerAndValidatorWithCapabilities(t, v12Capabilities())
}

func setupLedgerAndValidatorWithV13Capabilities(t *testing.T) (ledger.PeerLedger, txvalidator.Validator, func()) {
	return setupLedgerAndValidatorWithCapabilities(t, v13Capabilities())
}

func setupLedgerAndValidatorWithCapabilities(t *testing.T, c *tmocks.ApplicationCapabilities) (ledger.PeerLedger, txvalidator.Validator, func()) {
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(nil)
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	return setupLedgerAndValidatorExplicitWithMSP(t, c, &builtin.DefaultValidation{}, mspmgr)
}

func setupLedgerAndValidatorExplicitWithMSP(t *testing.T, cpb *tmocks.ApplicationCapabilities, plugin validation.Plugin, mspMgr msp.MSPManager) (ledger.PeerLedger, txvalidator.Validator, func()) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	gb, err := ctxt.MakeGenesisBlock("TestLedger")
	require.NoError(t, err)
	theLedger, err := ledgerMgr.CreateLedger("TestLedger", gb)
	require.NoError(t, err)
	pm := &mocks.Mapper{}
	factory := &mocks.PluginFactory{}
	pm.On("FactoryByName", vp.Name("vscc")).Return(factory)
	factory.On("New").Return(plugin)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	theValidator := txvalidatorv14.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: cpb, MSPManagerVal: mspMgr},
		pm,
		cryptoProvider,
	)

	return theLedger,
		theValidator,
		func() {
			theLedger.Close()
			cleanup()
		}
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

func putCCInfoWithVSCCAndVer(theLedger ledger.PeerLedger, ccname, vscc, ver string, policy []byte, t *testing.T) {
	cd := &ccp.ChaincodeData{
		Name:    ccname,
		Version: ver,
		Vscc:    vscc,
		Policy:  policy,
	}

	cdbytes := protoutil.MarshalOrPanic(cd)

	txid := util.GenerateUUID()
	simulator, err := theLedger.NewTxSimulator(txid)
	require.NoError(t, err)
	simulator.SetState("lscc", ccname, cdbytes)
	simulator.Done()

	simRes, err := simulator.GetTxSimulationResults()
	require.NoError(t, err)
	pubSimulationBytes, err := simRes.GetPubSimulationBytes()
	require.NoError(t, err)
	bcInfo, err := theLedger.GetBlockchainInfo()
	require.NoError(t, err)
	block0 := testutil.ConstructBlock(t, 1, bcInfo.CurrentBlockHash, [][]byte{pubSimulationBytes}, true)
	err = theLedger.CommitLegacy(&ledger.BlockAndPvtData{Block: block0}, &ledger.CommitOptions{})
	require.NoError(t, err)
}

func putSBEP(theLedger ledger.PeerLedger, cc, key string, policy []byte, t *testing.T) {
	vpMetadataKey := peer.MetaDataKeys_VALIDATION_PARAMETER.String()
	txid := util.GenerateUUID()
	simulator, err := theLedger.NewTxSimulator(txid)
	require.NoError(t, err)
	simulator.SetStateMetadata(cc, key, map[string][]byte{vpMetadataKey: policy})
	simulator.SetState(cc, key, []byte("I am a man who walks alone"))
	simulator.Done()

	simRes, err := simulator.GetTxSimulationResults()
	require.NoError(t, err)
	pubSimulationBytes, err := simRes.GetPubSimulationBytes()
	require.NoError(t, err)
	bcInfo, err := theLedger.GetBlockchainInfo()
	require.NoError(t, err)
	block0 := testutil.ConstructBlock(t, 2, bcInfo.CurrentBlockHash, [][]byte{pubSimulationBytes}, true)
	err = theLedger.CommitLegacy(&ledger.BlockAndPvtData{Block: block0}, &ledger.CommitOptions{})
	require.NoError(t, err)
}

func putCCInfo(theLedger ledger.PeerLedger, ccname string, policy []byte, t *testing.T) {
	putCCInfoWithVSCCAndVer(theLedger, ccname, "vscc", ccVersion, policy, t)
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

func TestInvokeBadRWSet(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeBadRWSet(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeBadRWSet(t, l, v)
	})
}

func testInvokeBadRWSet(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	tx := getEnv(ccID, nil, []byte("barf"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 1}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_BAD_RWSET)
}

func TestInvokeNoPolicy(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNoPolicy(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNoPolicy(t, l, v)
	})
}

func testInvokeNoPolicy(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, nil, t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeOK(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeOK(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeOK(t, l, v)
	})
}

func testInvokeOK(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeNOKDuplicateNs(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKDuplicateNs(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKDuplicateNs(t, l, v)
	})
}

func testInvokeNOKDuplicateNs(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

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
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	t.Run("Pre-1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicit(t, preV12Capabilities(), plugin)
		defer cleanup()

		ccID := "mycc"

		putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

		tx := getEnv(ccID, nil, createRWset(t), t)
		b := &common.Block{
			Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
			Header: &common.BlockHeader{},
		}

		err := v.Validate(b)
		require.NoError(t, err)
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
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v12Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeNoRWSet(t, l, v)
	})

	// Here we test that if we have the 1.3 capability, we still reject a transaction that only contains
	// reads if it doesn't comply with the endorsement policy
	t.Run("Post-1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v13Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeNoRWSet(t, l, v)
	})
}

func testInvokeNoRWSet(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

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
	sid := &mb.SerializedIdentity{
		Mspid:   msi.MspID,
		IdBytes: msi.SerializedID,
	}
	sidBytes := protoutil.MarshalOrPanic(sid)
	return sidBytes, nil
}

func (msi *mockSI) SatisfiesPrincipal(principal *mb.MSPPrincipal) error {
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

func (fake *mockMSP) IsWellFormed(identity *mb.SerializedIdentity) error {
	return nil
}

func (fake *mockMSP) Setup(config *mb.MSPConfig) error {
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

func (fake *mockMSP) SatisfiesPrincipal(id msp.Identity, principal *mb.MSPPrincipal) error {
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
	l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(
		t,
		v13Capabilities(),
		&builtin.DefaultValidation{},
		mgr,
	)
	defer cleanup()

	ccID := "mycc"

	policy := policydsl.SignedByMspPeer("Org1")
	polBytes := protoutil.MarshalOrPanic(policy)
	putCCInfo(l, ccID, polBytes, t)

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
	t.Run("PreV1.2", func(t *testing.T) {
		t.Run("MisMatchedName", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithPreV12Capabilities(t)
			defer cleanup()

			ccID := "mycc"

			putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

			tx := getEnv(ccID, protoutil.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: "wrong"}), createRWset(t), t)
			b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

			err := v.Validate(b)
			require.NoError(t, err)
			assertValid(b, t)
		})

		t.Run("BadBytes", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithPreV12Capabilities(t)
			defer cleanup()

			ccID := "mycc"

			putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

			tx := getEnv(ccID, []byte("garbage"), createRWset(t), t)
			b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

			err := v.Validate(b)
			require.NoError(t, err)
			assertValid(b, t)
		})

		t.Run("GoodPath", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithPreV12Capabilities(t)
			defer cleanup()

			ccID := "mycc"

			putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

			tx := getEnv(ccID, protoutil.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: ccID}), createRWset(t), t)
			b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

			err := v.Validate(b)
			require.NoError(t, err)
			assertValid(b, t)
		})
	})

	t.Run("PostV1.2", func(t *testing.T) {
		t.Run("MisMatchedName", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
			defer cleanup()

			testCCEventMismatchedName(t, l, v)
		})

		t.Run("BadBytes", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
			defer cleanup()

			testCCEventBadBytes(t, l, v)
		})

		t.Run("GoodPath", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
			defer cleanup()

			testCCEventGoodPath(t, l, v)
		})
	})

	t.Run("V1.3", func(t *testing.T) {
		t.Run("MisMatchedName", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
			defer cleanup()

			testCCEventMismatchedName(t, l, v)
		})

		t.Run("BadBytes", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
			defer cleanup()

			testCCEventBadBytes(t, l, v)
		})

		t.Run("GoodPath", func(t *testing.T) {
			l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
			defer cleanup()

			testCCEventGoodPath(t, l, v)
		})
	})
}

func testCCEventMismatchedName(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, protoutil.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: "wrong"}), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err) // TODO, convert test so it can check the error text for INVALID_OTHER_REASON
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func testCCEventBadBytes(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, []byte("garbage"), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err) // TODO, convert test so it can check the error text for INVALID_OTHER_REASON
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func testCCEventGoodPath(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, protoutil.MarshalOrPanic(&peer.ChaincodeEvent{ChaincodeId: ccID}), createRWset(t), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeOKPvtDataOnly(t *testing.T) {
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	t.Run("V1.2", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v12Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeOKPvtDataOnly(t, l, v)
	})

	t.Run("V1.3", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v13Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeOKPvtDataOnly(t, l, v)
	})
}

func testInvokeOKPvtDataOnly(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

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
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	t.Run("V1.2", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v12Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeOKMetaUpdateOnly(t, l, v)
	})

	t.Run("V1.3", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v13Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeOKMetaUpdateOnly(t, l, v)
	})
}

func testInvokeOKMetaUpdateOnly(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

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
	mspmgr := &mocks2.MSPManager{}
	idThatSatisfiesPrincipal := &mocks2.Identity{}
	idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(errors.New("principal not satisfied"))
	idThatSatisfiesPrincipal.GetIdentifierReturns(&msp.IdentityIdentifier{})
	mspmgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)

	t.Run("V1.2", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v12Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeOKPvtMetaUpdateOnly(t, l, v)
	})

	t.Run("V1.3", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorExplicitWithMSP(t, v13Capabilities(), &builtin.DefaultValidation{}, mspmgr)
		defer cleanup()

		testInvokeOKPvtMetaUpdateOnly(t, l, v)
	})
}

func testInvokeOKPvtMetaUpdateOnly(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

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

func TestInvokeOKSCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeOKSCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeOKSCC(t, l, v)
	})
}

func testInvokeOKSCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	cds := protoutil.MarshalOrPanic(&peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "cc", Version: "ver"},
			Input:       &peer.ChaincodeInput{},
		},
	})
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: "lscc", Version: ccVersion},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte("testchannelid"), cds}},
			Type:        peer.ChaincodeSpec_GOLANG,
		},
	}

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "testchannelid", cis, signerSerialized)
	require.NoError(t, err)
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", "cc", protoutil.MarshalOrPanic(&ccp.ChaincodeData{Name: "cc", Version: "ver", InstantiationPolicy: policydsl.MarshaledAcceptAllPolicy}))
	rwset, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	rwsetBytes, err := rwset.GetPubSimulationBytes()
	require.NoError(t, err)
	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, rwsetBytes, nil, &peer.ChaincodeID{Name: "lscc", Version: ccVersion}, signer)
	require.NoError(t, err)
	tx, err := protoutil.CreateSignedTx(prop, signer, presp)
	require.NoError(t, err)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 1}}

	err = v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
}

func TestInvokeNOKWritesToLSCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKWritesToLSCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKWritesToLSCC(t, l, v)
	})
}

func testInvokeNOKWritesToLSCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "lscc"), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 2}}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKWritesToESCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKWritesToESCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKWritesToESCC(t, l, v)
	})
}

func testInvokeNOKWritesToESCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "escc"), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKWritesToNotExt(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKWritesToNotExt(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKWritesToNotExt(t, l, v)
	})
}

func testInvokeNOKWritesToNotExt(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID, "escc"), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKInvokesNotExt(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKInvokesNotExt(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKInvokesNotExt(t, l, v)
	})
}

func testInvokeNOKInvokesNotExt(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "escc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ILLEGAL_WRITESET)
}

func TestInvokeNOKInvokesEmptyCCName(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKInvokesEmptyCCName(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKInvokesEmptyCCName(t, l, v)
	})
}

func testInvokeNOKInvokesEmptyCCName(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := ""

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNOKExpiredCC(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKExpiredCC(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKExpiredCC(t, l, v)
	})
}

func testInvokeNOKExpiredCC(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "vscc", "badversion", signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_EXPIRED_CHAINCODE)
}

func TestInvokeNOKBogusActions(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKBogusActions(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKBogusActions(t, l, v)
	})
}

func testInvokeNOKBogusActions(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

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
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKCCDoesntExist(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKCCDoesntExist(t, l, v)
	})
}

func testInvokeNOKCCDoesntExist(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNOKVSCCUnspecified(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNOKVSCCUnspecified(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNOKVSCCUnspecified(t, l, v)
	})
}

func testInvokeNOKVSCCUnspecified(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "", ccVersion, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err := v.Validate(b)
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_INVALID_OTHER_REASON)
}

func TestInvokeNoBlock(t *testing.T) {
	t.Run("1.2Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		testInvokeNoBlock(t, l, v)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		testInvokeNoBlock(t, l, v)
	})
}

func testInvokeNoBlock(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) {
	err := v.Validate(&common.Block{
		Data:   &common.BlockData{Data: [][]byte{}},
		Header: &common.BlockHeader{},
	})
	require.NoError(t, err)
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
		l, v, cleanup := setupLedgerAndValidatorWithV12Capabilities(t)
		defer cleanup()

		b, err := validateTxWithStateBasedEndorsement(t, l, v)

		require.NoError(t, err)
		assertValid(b, t)
	})

	t.Run("1.3Capability", func(t *testing.T) {
		l, v, cleanup := setupLedgerAndValidatorWithV13Capabilities(t)
		defer cleanup()

		b, err := validateTxWithStateBasedEndorsement(t, l, v)

		require.NoError(t, err)
		assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
	})
}

func validateTxWithStateBasedEndorsement(t *testing.T, l ledger.PeerLedger, v txvalidator.Validator) (*common.Block, error) {
	ccID := "mycc"

	putCCInfoWithVSCCAndVer(l, ccID, "vscc", ccVersion, signedByAnyMember([]string{"SampleOrg"}), t)
	putSBEP(l, ccID, "key", policydsl.MarshaledRejectAllPolicy, t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}}, Header: &common.BlockHeader{Number: 3}}

	err := v.Validate(b)

	return b, err
}

// mockLedger structure used to test ledger
// failure, therefore leveraging mocking
// library as need to simulate ledger which not
// able to get access to state db
type mockLedger struct {
	mock.Mock
}

// TxIDExists returns true if the specified txID is already present in one of the already committed blocks
func (m *mockLedger) TxIDExists(txID string) (bool, error) {
	args := m.Called(txID)
	return args.Get(0).(bool), args.Error(1)
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
func (m *mockLedger) GetTxValidationCodeByTxID(txID string) (peer.TxValidationCode, uint64, error) {
	args := m.Called(txID)
	return args.Get(0).(peer.TxValidationCode), args.Get(1).(uint64), nil
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

// CommitLegacy commits the block and the corresponding pvt data in an atomic operation
func (m *mockLedger) CommitLegacy(pvtDataAndBlock *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error {
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

func (m *mockLedger) DoesPvtDataInfoExist(blkNum uint64) (bool, error) {
	args := m.Called()
	return args.Get(0).(bool), args.Error(1)
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

func (m *mockLedger) CommitPvtDataOfOldBlocks(reconciledPvtdata []*ledger.ReconciledPvtdata, unreconciled ledger.MissingPvtDataInfo) ([]*ledger.PvtdataHashMismatch, error) {
	return nil, nil
}

func (m *mockLedger) GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error) {
	args := m.Called()
	return args.Get(0).(ledger.MissingPvtDataTracker), nil
}

func (m *mockLedger) SubmitSnapshotRequest(height uint64) error {
	return nil
}

func (m *mockLedger) PendingSnapshotRequests() ([]uint64, error) {
	return nil, nil
}

func (m *mockLedger) CancelSnapshotRequest(height uint64) error {
	return nil
}

func (m *mockLedger) CommitNotificationsChannel(done <-chan struct{}) (<-chan *ledger.CommitNotification, error) {
	return nil, nil
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

func (exec *mockQueryExecutor) GetStateRangeScanIteratorWithPagination(namespace, startKey, endKey string, pageSize int32) (ledger.QueryResultsIterator, error) {
	args := exec.Called(namespace, startKey, endKey, pageSize)
	return args.Get(0).(ledger.QueryResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) ExecuteQuery(namespace, query string) (ledger2.ResultsIterator, error) {
	args := exec.Called(namespace)
	return args.Get(0).(ledger2.ResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) ExecuteQueryWithPagination(namespace, query, bookmark string, pageSize int32) (ledger.QueryResultsIterator, error) {
	args := exec.Called(namespace, query, bookmark, pageSize)
	return args.Get(0).(ledger.QueryResultsIterator), args.Error(1)
}

func (exec *mockQueryExecutor) GetPrivateData(namespace, collection, key string) ([]byte, error) {
	args := exec.Called(namespace, collection, key)
	return args.Get(0).([]byte), args.Error(1)
}

func (exec *mockQueryExecutor) GetPrivateDataHash(namespace, collection, key string) ([]byte, error) {
	args := exec.Called(namespace, collection, key)
	return args.Get(0).([]byte), args.Error(1)
}

func (exec *mockQueryExecutor) GetPrivateDataMetadataByHash(namespace, collection string, keyhash []byte) (map[string][]byte, error) {
	args := exec.Called(namespace, collection, keyhash)
	return args.Get(0).(map[string][]byte), args.Error(1)
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

func createCustomSupportAndLedger(t *testing.T) (*mocktxvalidator.Support, ledger.PeerLedger, func()) {
	ledgerMgr, cleanup := constructLedgerMgrWithTestDefaults(t)
	gb, err := ctxt.MakeGenesisBlock("TestLedger")
	require.NoError(t, err)
	l, err := ledgerMgr.CreateLedger("TestLedger", gb)
	require.NoError(t, err)

	identity := &mocks2.Identity{}
	identity.GetIdentifierReturns(&msp.IdentityIdentifier{
		Mspid: "SampleOrg",
		Id:    "foo",
	})
	mspManager := &mocks2.MSPManager{}
	mspManager.DeserializeIdentityReturns(identity, nil)
	support := &mocktxvalidator.Support{LedgerVal: l, ACVal: preV12Capabilities(), MSPManagerVal: mspManager}
	return support, l, cleanup
}

func TestDynamicCapabilitiesAndMSP(t *testing.T) {
	factory := &mocks.PluginFactory{}
	factory.On("New").Return(testdata.NewSampleValidationPlugin(t))
	pm := &mocks.Mapper{}
	pm.On("FactoryByName", vp.Name("vscc")).Return(factory)

	support, l, cleanup := createCustomSupportAndLedger(t)
	defer cleanup()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	v := txvalidatorv14.NewTxValidator("", semaphore.New(10), support, pm, cryptoProvider)

	ccID := "mycc"

	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	// Perform a validation of a block
	err = v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)
	// Record the number of times the capabilities and the MSP Manager were invoked
	capabilityInvokeCount := support.CapabilitiesInvokeCount()
	mspManagerInvokeCount := support.MSPManagerInvokeCount()

	// Perform another validation pass, and ensure it is valid
	err = v.Validate(b)
	require.NoError(t, err)
	assertValid(b, t)

	// Ensure that the capabilities were retrieved from the support twice,
	// which proves that the capabilities are dynamically retrieved from the support each time
	require.Equal(t, 2*capabilityInvokeCount, support.CapabilitiesInvokeCount())
	// Ensure that the MSP Manager was retrieved from the support twice,
	// which proves that the MSP Manager is dynamically retrieved from the support each time
	require.Equal(t, 2*mspManagerInvokeCount, support.MSPManagerInvokeCount())
}

// TestLedgerIsNoAvailable simulates and provides a test for following scenario,
// which is based on FAB-535. Test checks the validation path which expects that
// DB won't available while trying to lookup for VSCC from LCCC and therefore
// transaction validation will have to fail. In such case the outcome should be
// the error return from validate block method and processing of transactions
// has to stop. There is suppose to be clear indication of the failure with error
// returned from the function call.
func TestLedgerIsNoAvailable(t *testing.T) {
	theLedger := new(mockLedger)
	pm := &mocks.Mapper{}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	validator := txvalidatorv14.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: preV12Capabilities()},
		pm,
		cryptoProvider,
	)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("TxIDExists", mock.Anything).Return(false, nil)

	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", mock.Anything, mock.Anything).Return([]byte{}, errors.New("Unable to connect to DB"))
	theLedger.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err = validator.Validate(b)

	assertion := require.New(t)
	// We suppose to get the error which indicates we cannot commit the block
	assertion.Error(err)
	// The error exptected to be of type VSCCInfoLookupFailureError
	assertion.NotNil(err.(*commonerrors.VSCCInfoLookupFailureError))
}

func TestLedgerIsNotAvailableForCheckingTxidDuplicate(t *testing.T) {
	theLedger := new(mockLedger)
	pm := &mocks.Mapper{}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	validator := txvalidatorv14.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: preV12Capabilities()},
		pm,
		cryptoProvider,
	)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("TxIDExists", mock.Anything).Return(false, errors.New("Unable to connect to DB"))

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err = validator.Validate(b)

	assertion := require.New(t)
	// We expect a validation error because the ledger wasn't ready to tell us whether there was a tx with that ID or not
	assertion.Error(err)
}

func TestDuplicateTxId(t *testing.T) {
	theLedger := new(mockLedger)
	pm := &mocks.Mapper{}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	validator := txvalidatorv14.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: preV12Capabilities()},
		pm,
		cryptoProvider,
	)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("TxIDExists", mock.Anything).Return(true, nil)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	err = validator.Validate(b)

	assertion := require.New(t)
	// We expect no validation error because we simply mark the tx as invalid
	assertion.NoError(err)

	// We expect the tx to be invalid because of a duplicate txid
	txsfltr := txflags.ValidationFlags(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assertion.True(txsfltr.IsInvalid(0))
	assertion.True(txsfltr.Flag(0) == peer.TxValidationCode_DUPLICATE_TXID)
}

func TestValidationInvalidEndorsing(t *testing.T) {
	theLedger := new(mockLedger)
	pm := &mocks.Mapper{}
	factory := &mocks.PluginFactory{}
	plugin := &mocks.Plugin{}
	factory.On("New").Return(plugin)
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("invalid tx"))
	pm.On("FactoryByName", vp.Name("vscc")).Return(factory)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	validator := txvalidatorv14.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{LedgerVal: theLedger, ACVal: preV12Capabilities()},
		pm,
		cryptoProvider,
	)

	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)

	theLedger.On("TxIDExists", mock.Anything).Return(false, nil)

	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}

	cdbytes := protoutil.MarshalOrPanic(cd)

	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", "lscc", ccID).Return(cdbytes, nil)
	theLedger.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	// Keep default callback
	err = validator.Validate(b)
	// Restore default callback
	require.NoError(t, err)
	assertInvalid(b, t, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
}

func createMockLedger(t *testing.T, ccID string) *mockLedger {
	l := new(mockLedger)
	l.On("TxIDExists", mock.Anything).Return(false, nil)
	cd := &ccp.ChaincodeData{
		Name:    ccID,
		Version: ccVersion,
		Vscc:    "vscc",
		Policy:  signedByAnyMember([]string{"SampleOrg"}),
	}

	cdbytes := protoutil.MarshalOrPanic(cd)
	queryExecutor := new(mockQueryExecutor)
	queryExecutor.On("GetState", "lscc", ccID).Return(cdbytes, nil)
	l.On("NewQueryExecutor", mock.Anything).Return(queryExecutor, nil)
	return l
}

func TestValidationPluginExecutionError(t *testing.T) {
	plugin := &mocks.Plugin{}
	plugin.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	l, v, cleanup := setupLedgerAndValidatorExplicit(t, preV12Capabilities(), plugin)
	defer cleanup()

	ccID := "mycc"
	putCCInfo(l, ccID, signedByAnyMember([]string{"SampleOrg"}), t)

	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	plugin.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&validation.ExecutionFailureError{
		Reason: "I/O error",
	})

	err := v.Validate(b)
	executionErr := err.(*commonerrors.VSCCExecutionFailureError)
	require.Contains(t, executionErr.Error(), "I/O error")
}

func TestValidationPluginNotFound(t *testing.T) {
	ccID := "mycc"
	tx := getEnv(ccID, nil, createRWset(t, ccID), t)
	l := createMockLedger(t, ccID)

	b := &common.Block{
		Data:   &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(tx)}},
		Header: &common.BlockHeader{},
	}

	pm := &mocks.Mapper{}
	pm.On("FactoryByName", vp.Name("vscc")).Return(nil)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	validator := txvalidatorv14.NewTxValidator(
		"",
		semaphore.New(10),
		&mocktxvalidator.Support{LedgerVal: l, ACVal: preV12Capabilities()},
		pm,
		cryptoProvider,
	)
	err = validator.Validate(b)
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

func constructLedgerMgrWithTestDefaults(t *testing.T) (*ledgermgmt.LedgerMgr, func()) {
	testDir := t.TempDir()
	initializer := ledgermgmttest.NewInitializer(testDir)
	ledgerMgr := ledgermgmt.NewLedgerMgr(initializer)
	cleanup := func() {
		ledgerMgr.Close()
	}
	return ledgerMgr, cleanup
}
