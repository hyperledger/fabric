/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package v13

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	mc "github.com/hyperledger/fabric/common/mocks/config"
	lm "github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/common/util"
	aclmocks "github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	mocks2 "github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	cutils "github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin/v13/mocks"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	per "github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func createTx(endorsedByDuplicatedIdentity bool) (*common.Envelope, error) {
	ccid := &peer.ChaincodeID{Name: "foo", Version: "v1"}
	cis := &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{ChaincodeId: ccid}}

	prop, _, err := utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, sid)
	if err != nil {
		return nil, err
	}

	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, []byte("res"), nil, ccid, nil, id)
	if err != nil {
		return nil, err
	}

	var env *common.Envelope
	if endorsedByDuplicatedIdentity {
		env, err = utils.CreateSignedTx(prop, id, presp, presp)
	} else {
		env, err = utils.CreateSignedTx(prop, id, presp)
	}
	if err != nil {
		return nil, err
	}
	return env, err
}

func processSignedCDS(cds *peer.ChaincodeDeploymentSpec, policy *common.SignaturePolicyEnvelope) ([]byte, error) {
	env, err := ccpackage.OwnerCreateSignedCCDepSpec(cds, policy, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create package %s", err)
	}

	b := utils.MarshalOrPanic(env)

	ccpack := &ccprovider.SignedCDSPackage{}
	cd, err := ccpack.InitFromBuffer(b)
	if err != nil {
		return nil, fmt.Errorf("error owner creating package %s", err)
	}

	if err = ccpack.PutChaincodeToFS(); err != nil {
		return nil, fmt.Errorf("error putting package on the FS %s", err)
	}

	cd.InstantiationPolicy = utils.MarshalOrPanic(policy)

	return utils.MarshalOrPanic(cd), nil
}

func constructDeploymentSpec(name string, path string, version string, initArgs [][]byte, createFS bool) (*peer.ChaincodeDeploymentSpec, error) {
	spec := &peer.ChaincodeSpec{Type: 1, ChaincodeId: &peer.ChaincodeID{Name: name, Path: path, Version: version}, Input: &peer.ChaincodeInput{Args: initArgs}}

	codePackageBytes := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(codePackageBytes)
	tw := tar.NewWriter(gz)

	err := cutils.WriteBytesToPackage("src/garbage.go", []byte(name+path+version), tw)
	if err != nil {
		return nil, err
	}

	tw.Close()
	gz.Close()

	chaincodeDeploymentSpec := &peer.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes.Bytes()}

	if createFS {
		err := ccprovider.PutChaincodeIntoFS(chaincodeDeploymentSpec)
		if err != nil {
			return nil, err
		}
	}

	return chaincodeDeploymentSpec, nil
}

func createCCDataRWsetWithCollection(nameK, nameV, version string, policy []byte, collectionConfigPackage []byte) ([]byte, error) {
	cd := &ccprovider.ChaincodeData{
		Name:                nameV,
		Version:             version,
		InstantiationPolicy: policy,
	}

	cdbytes := utils.MarshalOrPanic(cd)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", nameK, cdbytes)
	rwsetBuilder.AddToWriteSet("lscc", privdata.BuildCollectionKVSKey(nameK), collectionConfigPackage)
	sr, err := rwsetBuilder.GetTxSimulationResults()
	if err != nil {
		return nil, err
	}
	return sr.GetPubSimulationBytes()
}

func createCCDataRWset(nameK, nameV, version string, policy []byte) ([]byte, error) {
	cd := &ccprovider.ChaincodeData{
		Name:                nameV,
		Version:             version,
		InstantiationPolicy: policy,
	}

	cdbytes := utils.MarshalOrPanic(cd)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", nameK, cdbytes)
	sr, err := rwsetBuilder.GetTxSimulationResults()
	if err != nil {
		return nil, err
	}
	return sr.GetPubSimulationBytes()
}

func createLSCCTxWithCollection(ccname, ccver, f string, res []byte, policy []byte, ccpBytes []byte) (*common.Envelope, error) {
	return createLSCCTxPutCdsWithCollection(ccname, ccver, f, res, nil, true, policy, ccpBytes)
}

func createLSCCTx(ccname, ccver, f string, res []byte) (*common.Envelope, error) {
	return createLSCCTxPutCds(ccname, ccver, f, res, nil, true)
}

func createLSCCTxPutCdsWithCollection(ccname, ccver, f string, res, cdsbytes []byte, putcds bool, policy []byte, ccpBytes []byte) (*common.Envelope, error) {
	cds := &peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{
				Name:    ccname,
				Version: ccver,
			},
			Type: peer.ChaincodeSpec_GOLANG,
		},
	}

	cdsBytes, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	var cis *peer.ChaincodeInvocationSpec
	if putcds {
		if cdsbytes != nil {
			cdsBytes = cdsbytes
		}
		cis = &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
				Input: &peer.ChaincodeInput{
					Args: [][]byte{[]byte(f), []byte("barf"), cdsBytes, []byte("escc"), []byte("vscc"), policy, ccpBytes},
				},
				Type: peer.ChaincodeSpec_GOLANG,
			},
		}
	} else {
		cis = &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
				Input: &peer.ChaincodeInput{
					Args: [][]byte{[]byte(f), []byte("barf")},
				},
				Type: peer.ChaincodeSpec_GOLANG,
			},
		}
	}

	prop, _, err := utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, sid)
	if err != nil {
		return nil, err
	}

	ccid := &peer.ChaincodeID{Name: ccname, Version: ccver}

	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, res, nil, ccid, nil, id)
	if err != nil {
		return nil, err
	}

	return utils.CreateSignedTx(prop, id, presp)
}

func createLSCCTxPutCds(ccname, ccver, f string, res, cdsbytes []byte, putcds bool) (*common.Envelope, error) {
	cds := &peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{
				Name:    ccname,
				Version: ccver,
			},
			Type: peer.ChaincodeSpec_GOLANG,
		},
	}

	cdsBytes, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	var cis *peer.ChaincodeInvocationSpec
	if putcds {
		if cdsbytes != nil {
			cdsBytes = cdsbytes
		}
		cis = &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
				Input: &peer.ChaincodeInput{
					Args: [][]byte{[]byte(f), []byte("barf"), cdsBytes},
				},
				Type: peer.ChaincodeSpec_GOLANG,
			},
		}
	} else {
		cis = &peer.ChaincodeInvocationSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
				Input: &peer.ChaincodeInput{
					Args: [][]byte{[]byte(f), []byte(ccname)},
				},
				Type: peer.ChaincodeSpec_GOLANG,
			},
		}
	}

	prop, _, err := utils.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, sid)
	if err != nil {
		return nil, err
	}

	ccid := &peer.ChaincodeID{Name: ccname, Version: ccver}

	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, res, nil, ccid, nil, id)
	if err != nil {
		return nil, err
	}

	return utils.CreateSignedTx(prop, id, presp)
}

func getSignedByMSPMemberPolicy(mspID string) ([]byte, error) {
	p := cauthdsl.SignedByMspMember(mspID)

	b, err := utils.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func getSignedByMSPAdminPolicy(mspID string) ([]byte, error) {
	p := cauthdsl.SignedByMspAdmin(mspID)

	b, err := utils.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func newValidationInstance(state map[string]map[string][]byte) *Validator {
	qec := &mocks2.QueryExecutorCreator{}
	qec.On("NewQueryExecutor").Return(lm.NewMockQueryExecutor(state), nil)
	return newCustomValidationInstance(qec, &mc.MockApplicationCapabilities{})
}

func newCustomValidationInstance(qec txvalidator.QueryExecutorCreator, c validation.Capabilities) *Validator {
	sbvm := &mocks.StateBasedValidator{}
	sbvm.On("PreValidate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sbvm.On("PostValidate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	sf := &txvalidator.StateFetcherImpl{QueryExecutorCreator: qec}
	is := &mocks.IdentityDeserializer{}
	pe := &txvalidator.PolicyEvaluator{
		IdentityDeserializer: mspmgmt.GetManagerForChain(util.GetTestChainID()),
	}
	v := New(c, sf, is, pe)

	v.stateBasedValidator = sbvm
	return v
}

func TestStateBasedValidationFailure(t *testing.T) {
	qec := &mocks2.QueryExecutorCreator{}
	qec.On("NewQueryExecutor").Return(lm.NewMockQueryExecutor(make(map[string]map[string][]byte)), nil)

	sbvm := &mocks.StateBasedValidator{}
	sbvm.On("PreValidate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sbvm.On("PostValidate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	sf := &txvalidator.StateFetcherImpl{QueryExecutorCreator: qec}
	is := &mocks.IdentityDeserializer{}
	pe := &txvalidator.PolicyEvaluator{
		IdentityDeserializer: mspmgmt.GetManagerForChain(util.GetTestChainID()),
	}
	v := New(&mc.MockApplicationCapabilities{}, sf, is, pe)
	v.stateBasedValidator = sbvm

	tx, err := createTx(false)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}

	// bad path: policy validation error
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCEndorsementPolicyError{Err: fmt.Errorf("some sbe validation err")}).Once()
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.Error(t, err)
	assert.IsType(t, &commonerrors.VSCCEndorsementPolicyError{}, err)

	// bad path: execution error
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCExecutionFailureError{Err: fmt.Errorf("some sbe validation err")}).Once()
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.Error(t, err)
	assert.IsType(t, &commonerrors.VSCCExecutionFailureError{}, err)

	// good path: signed by the right MSP
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.NoError(t, err)
}

func TestInvoke(t *testing.T) {
	v := newValidationInstance(make(map[string]map[string][]byte))

	// broken Envelope
	var err error
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{[]byte("a")}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	assert.Error(t, err)

	// (still) broken Envelope
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{Payload: []byte("barf")})}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	assert.Error(t, err)

	// (still) broken Envelope
	e := utils.MarshalOrPanic(&common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: []byte("barf")}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	assert.Error(t, err)

	tx, err := createTx(false)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// broken type
	e = utils.MarshalOrPanic(&common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_ORDERER_TRANSACTION)})}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.Error(t, err)

	// broken tx payload
	e = utils.MarshalOrPanic(&common.Envelope{Payload: utils.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_ORDERER_TRANSACTION)})}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.Error(t, err)

	// good path: signed by the right MSP
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.NoError(t, err)
}

func TestRWSetTooBig(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	r := stublccc.MockInit("1", [][]byte{})
	if r.Status != shim.OK {
		fmt.Println("Init failed", string(r.Message))
		t.FailNow()
	}

	ccname := "mycc"
	ccver := "1"

	cd := &ccprovider.ChaincodeData{
		Name:                ccname,
		Version:             ccver,
		InstantiationPolicy: nil,
	}

	cdbytes := utils.MarshalOrPanic(cd)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", ccname, cdbytes)
	rwsetBuilder.AddToWriteSet("lscc", "spurious", []byte("spurious"))

	sr, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	srBytes, err := sr.GetPubSimulationBytes()
	assert.NoError(t, err)
	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, srBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

func TestValidateDeployFail(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)
	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"

	/*********************/
	/* test no write set */
	/*********************/

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, nil)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/************************/
	/* test bogus write set */
	/************************/

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", ccname, []byte("barf"))
	sr, err := rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	resBogusBytes, err := sr.GetPubSimulationBytes()
	assert.NoError(t, err)
	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, resBogusBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/**********************/
	/* test bad LSCC args */
	/**********************/

	res, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err = createLSCCTxPutCds(ccname, ccver, lscc.DEPLOY, res, nil, false)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/**********************/
	/* test bad LSCC args */
	/**********************/

	res, err = createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err = createLSCCTxPutCds(ccname, ccver, lscc.DEPLOY, res, []byte("barf"), true)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/***********************/
	/* test bad cc version */
	/***********************/

	res, err = createCCDataRWset(ccname, ccname, ccver+".1", nil)
	assert.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/*************/
	/* bad rwset */
	/*************/

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, []byte("barf"))
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/********************/
	/* test bad cc name */
	/********************/

	res, err = createCCDataRWset(ccname+".badbad", ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/**********************/
	/* test bad cc name 2 */
	/**********************/

	res, err = createCCDataRWset(ccname, ccname+".badbad", ccver, nil)
	assert.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

	/************************/
	/* test suprious writes */
	/************************/

	cd := &ccprovider.ChaincodeData{
		Name:                ccname,
		Version:             ccver,
		InstantiationPolicy: nil,
	}

	cdbytes := utils.MarshalOrPanic(cd)
	rwsetBuilder = rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", ccname, cdbytes)
	rwsetBuilder.AddToWriteSet("bogusbogus", "key", []byte("val"))
	sr, err = rwsetBuilder.GetTxSimulationResults()
	assert.NoError(t, err)
	srBytes, err := sr.GetPubSimulationBytes()
	assert.NoError(t, err)
	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, srBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)

}

func TestAlreadyDeployed(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)
	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := utils.MockSignedEndorserProposal2OrPanic(chainId, &peer.ChaincodeSpec{}, id)
	args := [][]byte{[]byte("deploy"), []byte(ccname), b}
	if res := stublccc.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		fmt.Printf("%#v\n", res)
		t.FailNow()
	}

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

func TestValidateDeployNoLedger(t *testing.T) {
	qec := &mocks2.QueryExecutorCreator{}
	qec.On("NewQueryExecutor").Return(nil, errors.New("failed obtaining query executor"))
	v := newCustomValidationInstance(qec, &mc.MockApplicationCapabilities{})

	ccname := "mycc"
	ccver := "1"

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	assert.NoError(t, err)
	res, err := createCCDataRWset(ccname, ccname, ccver, defaultPolicy)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

func TestValidateDeployOK(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	assert.NoError(t, err)
	res, err := createCCDataRWset(ccname, ccname, ccver, defaultPolicy)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.NoError(t, err)
}

func TestValidateDeployWithCollection(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv: &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{
			PrivateChannelDataRv: true,
		}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	qec := &mocks2.QueryExecutorCreator{}
	qec.On("NewQueryExecutor").Return(lm.NewMockQueryExecutor(state), nil)
	v := newCustomValidationInstance(qec, &mc.MockApplicationCapabilities{
		PrivateChannelDataRv: true,
	})

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	r := stublccc.MockInit("1", [][]byte{})
	if r.Status != shim.OK {
		fmt.Println("Init failed", string(r.Message))
		t.FailNow()
	}

	ccname := "mycc"
	ccver := "1"

	collName1 := "mycollection1"
	collName2 := "mycollection2"
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 1000
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	// Test 1: Deploy chaincode with a valid collection configs --> success
	ccp := &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1, coll2}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	assert.NoError(t, err)
	res, err := createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
	assert.NoError(t, err)

	tx, err := createLSCCTxWithCollection(ccname, ccver, lscc.DEPLOY, res, defaultPolicy, ccpBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.NoError(t, err)

	// Test 2: Deploy the chaincode with duplicate collection configs --> no error as the
	// peer is not in V1_2Validation mode
	ccp = &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1, coll2, coll1}}
	ccpBytes, err = proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
	assert.NoError(t, err)

	tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.DEPLOY, res, defaultPolicy, ccpBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.NoError(t, err)

	// Test 3: Once the V1_2Validation is enabled, validation should fail due to duplicate collection configs
	state = make(map[string]map[string][]byte)
	mp = (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv: &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{
			PrivateChannelDataRv: true,
			V1_2ValidationRv:     true,
		}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v = newValidationInstance(state)

	lccc = lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc = shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	r = stublccc.MockInit("1", [][]byte{})
	if r.Status != shim.OK {
		fmt.Println("Init failed", string(r.Message))
		t.FailNow()
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
}

func TestValidateDeployWithPolicies(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"

	/*********************************************/
	/* test 1: success with an accept-all policy */
	/*********************************************/

	res, err := createCCDataRWset(ccname, ccname, ccver, cauthdsl.MarshaledAcceptAllPolicy)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	assert.NoError(t, err)

	/********************************************/
	/* test 2: failure with a reject-all policy */
	/********************************************/

	res, err = createCCDataRWset(ccname, ccname, ccver, cauthdsl.MarshaledRejectAllPolicy)
	assert.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

func TestInvalidUpgrade(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

func TestValidateUpgradeOK(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := utils.MockSignedEndorserProposal2OrPanic(chainId, &peer.ChaincodeSpec{}, id)
	args := [][]byte{[]byte("deploy"), []byte(ccname), b}
	if res := stublccc.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		fmt.Printf("%#v\n", res)
		t.FailNow()
	}

	ccver = "2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(bl, "foo", 0, 0, policy)
	assert.NoError(t, err)
}

func TestInvalidateUpgradeBadVersion(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := utils.MockSignedEndorserProposal2OrPanic(chainId, &peer.ChaincodeSpec{}, id)
	args := [][]byte{[]byte("deploy"), []byte(ccname), b}
	if res := stublccc.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		fmt.Printf("%#v\n", res)
		t.FailNow()
	}

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

func validateUpgradeWithCollection(t *testing.T, V1_2Validation bool) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv: &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{
			PrivateChannelDataRv: true,
			V1_2ValidationRv:     V1_2Validation,
		}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	qec := &mocks2.QueryExecutorCreator{}
	qec.On("NewQueryExecutor").Return(lm.NewMockQueryExecutor(state), nil)
	v := newCustomValidationInstance(qec, &mc.MockApplicationCapabilities{
		PrivateChannelDataRv: true,
		V1_2ValidationRv:     V1_2Validation,
	})

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	r := stublccc.MockInit("1", [][]byte{})
	if r.Status != shim.OK {
		fmt.Println("Init failed", string(r.Message))
		t.FailNow()
	}

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := utils.MockSignedEndorserProposal2OrPanic(chainId, &peer.ChaincodeSpec{}, id)
	args := [][]byte{[]byte("deploy"), []byte(ccname), b}
	if res := stublccc.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		fmt.Printf("%#v\n", res)
		t.FailNow()
	}

	ccver = "2"

	collName1 := "mycollection1"
	collName2 := "mycollection2"
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 1000
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	// Test 1: Valid Collection Config in the upgrade.
	// V1_2Validation enabled: success
	// V1_2Validation disable: fail (as no collection updates are allowed)
	// Note: We might change V1_2Validation with CollectionUpdate capability
	ccp := &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1, coll2}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	assert.NoError(t, err)
	res, err := createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
	assert.NoError(t, err)

	tx, err := createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	if V1_2Validation {
		assert.NoError(t, err)
	} else {
		assert.Error(t, err, "LSCC can only issue a single putState upon deploy/upgrade")
	}

	state["lscc"][privdata.BuildCollectionKVSKey(ccname)] = ccpBytes

	if V1_2Validation {
		ccver = "3"

		collName3 := "mycollection3"
		coll3 := createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

		// Test 2: some existing collections are missing in the updated config and peer in
		// V1_2Validation mode --> error
		ccp = &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll3}}
		ccpBytes, err = proto.Marshal(ccp)
		assert.NoError(t, err)
		assert.NotNil(t, ccpBytes)

		res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
		assert.NoError(t, err)

		tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
		if err != nil {
			t.Fatalf("createTx returned err %s", err)
		}

		envBytes, err = utils.GetBytesEnvelope(tx)
		if err != nil {
			t.Fatalf("GetBytesEnvelope returned err %s", err)
		}

		bl = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		assert.Error(t, err, "Some existing collection configurations are missing in the new collection configuration package")

		ccver = "3"

		// Test 3: some existing collections are missing in the updated config and peer in
		// V1_2Validation mode --> error
		ccp = &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1, coll3}}
		ccpBytes, err = proto.Marshal(ccp)
		assert.NoError(t, err)
		assert.NotNil(t, ccpBytes)

		res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
		assert.NoError(t, err)

		tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
		if err != nil {
			t.Fatalf("createTx returned err %s", err)
		}

		envBytes, err = utils.GetBytesEnvelope(tx)
		if err != nil {
			t.Fatalf("GetBytesEnvelope returned err %s", err)
		}

		args = [][]byte{[]byte("dv"), envBytes, policy, ccpBytes}
		bl = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		assert.Error(t, err, "existing collection named mycollection2 is missing in the new collection configuration package")

		ccver = "3"

		// Test 4: valid collection config config and peer in V1_2Validation mode --> success
		ccp = &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1, coll2, coll3}}
		ccpBytes, err = proto.Marshal(ccp)
		assert.NoError(t, err)
		assert.NotNil(t, ccpBytes)

		res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
		assert.NoError(t, err)

		tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
		if err != nil {
			t.Fatalf("createTx returned err %s", err)
		}

		envBytes, err = utils.GetBytesEnvelope(tx)
		if err != nil {
			t.Fatalf("GetBytesEnvelope returned err %s", err)
		}

		bl = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		assert.NoError(t, err)
	}
}

func TestValidateUpgradeWithCollection(t *testing.T) {
	// with V1_2Validation enabled
	validateUpgradeWithCollection(t, true)
	// with V1_2Validation disabled
	validateUpgradeWithCollection(t, false)
}

func TestValidateUpgradeWithPoliciesOK(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	_, err = processSignedCDS(cds, cauthdsl.AcceptAllPolicy)
	assert.NoError(t, err)
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := utils.MockSignedEndorserProposal2OrPanic(chainId, &peer.ChaincodeSpec{}, id)
	args := [][]byte{[]byte("deploy"), []byte(ccname), b}
	if res := stublccc.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		fmt.Printf("%#v\n", res)
		t.FailNow()
	}

	ccver = "2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	args = [][]byte{[]byte("dv"), envBytes, policy}
	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(bl, "foo", 0, 0, policy)
	assert.NoError(t, err)
}

func TestValidateUpgradeWithNewFailAllIP(t *testing.T) {
	// we're testing upgrade.
	// In particular, we want to test the scenario where the upgrade
	// complies with the instantiation policy of the current version
	// BUT NOT the instantiation policy of the new version. For this
	// reason we first deploy a cc with IP which is equal to the AcceptAllPolicy
	// and then try to upgrade with a cc with the RejectAllPolicy.
	// We run this test twice, once with the V11 capability (and expect
	// a failure) and once without (and we expect success).

	validateUpgradeWithNewFailAllIP(t, true, true)
	validateUpgradeWithNewFailAllIP(t, false, false)
}

func validateUpgradeWithNewFailAllIP(t *testing.T, v11capability, expecterr bool) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{V1_1ValidationRv: v11capability}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	qec := &mocks2.QueryExecutorCreator{}
	capabilities := &mc.MockApplicationCapabilities{}
	if v11capability {
		capabilities.V1_1ValidationRv = true
	}
	qec.On("NewQueryExecutor").Return(lm.NewMockQueryExecutor(state), nil)
	v := newCustomValidationInstance(qec, capabilities)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	// deploy the chaincode with an accept all policy

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"
	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	_, err = processSignedCDS(cds, cauthdsl.AcceptAllPolicy)
	assert.NoError(t, err)
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := utils.MockSignedEndorserProposal2OrPanic(chainId, &peer.ChaincodeSpec{}, id)
	args := [][]byte{[]byte("deploy"), []byte(ccname), b}
	if res := stublccc.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		fmt.Printf("%#v\n", res)
		t.FailNow()
	}

	// if we're here, we have a cc deployed with an accept all IP

	// now we upgrade, with v 2 of the same cc, with the crucial difference that it has a reject all IP

	ccver = "2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver,
		cauthdsl.MarshaledRejectAllPolicy, // here's where we specify the IP of the upgraded cc
	)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// execute the upgrade tx
	if expecterr {
		bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		assert.Error(t, err)
	} else {
		bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		assert.NoError(t, err)
	}
}

func TestValidateUpgradeWithPoliciesFail(t *testing.T) {
	state := make(map[string]map[string][]byte)
	mp := (&scc.MocksccProviderFactory{
		Qe: lm.NewMockQueryExecutor(state),
		ApplicationConfigBool: true,
		ApplicationConfigRv:   &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
	}).NewSystemChaincodeProvider().(*scc.MocksccProviderImpl)

	v := newValidationInstance(state)

	mockAclProvider := &aclmocks.MockACLProvider{}
	lccc := lscc.New(mp, mockAclProvider, platforms.NewRegistry(&golang.Platform{}))
	stublccc := shim.NewMockStub("lscc", lccc)
	state["lscc"] = stublccc.State

	ccname := "mycc"
	ccver := "1"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ppath := lccctestpath + "/" + ccname + "." + ccver

	os.Remove(ppath)

	cds, err := constructDeploymentSpec(ccname, path, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	if err != nil {
		fmt.Printf("%s\n", err)
		t.FailNow()
	}
	cdbytes, err := processSignedCDS(cds, cauthdsl.RejectAllPolicy)
	assert.NoError(t, err)
	defer os.Remove(ppath)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	// Simulate the lscc invocation whilst skipping the policy validation,
	// otherwise we wouldn't be able to deply a chaincode with a reject all policy
	stublccc.MockTransactionStart("barf")
	err = stublccc.PutState(ccname, cdbytes)
	assert.NoError(t, err)
	stublccc.MockTransactionEnd("barf")

	ccver = "2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	assert.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	assert.Error(t, err)
}

var id msp.SigningIdentity
var sid []byte
var mspid string
var chainId string = util.GetTestChainID()

type mockPolicyCheckerFactory struct {
}

func (c *mockPolicyCheckerFactory) NewPolicyChecker() policy.PolicyChecker {
	return &mockPolicyChecker{}
}

type mockPolicyChecker struct {
}

func (c *mockPolicyChecker) CheckPolicy(channelID, policyName string, signedProp *peer.SignedProposal) error {
	return nil
}

func (c *mockPolicyChecker) CheckPolicyBySignedData(channelID, policyName string, sd []*common.SignedData) error {
	return nil
}

func (c *mockPolicyChecker) CheckPolicyNoChannel(policyName string, signedProp *peer.SignedProposal) error {
	return nil
}

func createCollectionConfig(collectionName string, signaturePolicyEnvelope *common.SignaturePolicyEnvelope,
	requiredPeerCount int32, maximumPeerCount int32, blockToLive uint64,
) *common.CollectionConfig {
	signaturePolicy := &common.CollectionPolicyConfig_SignaturePolicy{
		SignaturePolicy: signaturePolicyEnvelope,
	}
	accessPolicy := &common.CollectionPolicyConfig{
		Payload: signaturePolicy,
	}

	return &common.CollectionConfig{
		Payload: &common.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &common.StaticCollectionConfig{
				Name:              collectionName,
				MemberOrgsPolicy:  accessPolicy,
				RequiredPeerCount: requiredPeerCount,
				MaximumPeerCount:  maximumPeerCount,
				BlockToLive:       blockToLive,
			},
		},
	}
}

func testValidateCollection(t *testing.T, v *Validator, collectionConfigs []*common.CollectionConfig, cdRWSet *ccprovider.ChaincodeData,
	lsccFunc string, ac channelconfig.ApplicationCapabilities, chid string,
) error {
	ccp := &common.CollectionConfigPackage{Config: collectionConfigs}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	lsccargs := [][]byte{nil, nil, nil, nil, nil, ccpBytes}
	rwset := &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: cdRWSet.Name}, {Key: privdata.BuildCollectionKVSKey(cdRWSet.Name), Value: ccpBytes}}}

	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	return err

}

func TestValidateRWSetAndCollectionForDeploy(t *testing.T) {
	var err error
	chid := "ch"
	ccid := "mycc"
	ccver := "1.0"
	cdRWSet := &ccprovider.ChaincodeData{Name: ccid, Version: ccver}

	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ac := capabilities.NewApplicationProvider(map[string]*common.Capability{
		capabilities.ApplicationV1_1: {},
	})

	lsccFunc := lscc.DEPLOY
	// Test 1: More than two entries in the rwset -> error
	rwset := &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: "b"}, {Key: "c"}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, nil, lsccFunc, ac, chid)
	assert.EqualError(t, err, "LSCC can only issue one or two putState upon deploy")

	// Test 2: Invalid key for the collection config package -> error
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: "b"}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, nil, lsccFunc, ac, chid)
	assert.EqualError(t, err, "invalid key for the collection of chaincode mycc:1.0; expected 'mycc~collection', received 'b'")

	// Test 3: No collection config package -> success
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, nil, lsccFunc, ac, chid)
	assert.NoError(t, err)

	lsccargs := [][]byte{nil, nil, nil, nil, nil, nil}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	assert.NoError(t, err)

	// Test 4: Valid key for the collection config package -> success
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: privdata.BuildCollectionKVSKey(ccid)}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	assert.NoError(t, err)

	// Test 5: Collection configuration of the lscc args doesn't match the rwset
	lsccargs = [][]byte{nil, nil, nil, nil, nil, []byte("barf")}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	assert.EqualError(t, err, "collection configuration arguments supplied for chaincode mycc:1.0 do not match the configuration in the lscc writeset")

	// Test 6: Invalid collection config package -> error
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: privdata.BuildCollectionKVSKey("mycc"), Value: []byte("barf")}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	assert.EqualError(t, err, "invalid collection configuration supplied for chaincode mycc:1.0")

	// Test 7: Valid collection config package -> success
	collName1 := "mycollection1"
	collName2 := "mycollection2"
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 10000
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2}, cdRWSet, lsccFunc, ac, chid)
	assert.NoError(t, err)

	// Test 8: Duplicate collections in the collection config package -> success as the peer is in v1.1 validation mode
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll1}, cdRWSet, lsccFunc, ac, chid)
	assert.NoError(t, err)

	// Test 9: requiredPeerCount > maximumPeerCount -> success as the peer is in v1.1 validation mode
	collName3 := "mycollection3"
	requiredPeerCount = 2
	maximumPeerCount = 1
	blockToLive = 10000
	coll3 := createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.NoError(t, err)

	// Enable v1.2 validation mode
	ac = capabilities.NewApplicationProvider(map[string]*common.Capability{
		capabilities.ApplicationV1_2: {},
	})

	// Test 10: Duplicate collections in the collection config package -> error
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll1}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "collection-name: mycollection1 -- found duplicate collection configuration")

	// Test 11: requiredPeerCount < 0 -> error
	requiredPeerCount = -2
	maximumPeerCount = 1
	blockToLive = 10000
	coll3 = createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "collection-name: mycollection3 -- requiredPeerCount (1) cannot be less than zero (-2)",
		collName3, maximumPeerCount, requiredPeerCount)

	// Test 11: requiredPeerCount > maximumPeerCount -> error
	requiredPeerCount = 2
	maximumPeerCount = 1
	blockToLive = 10000
	coll3 = createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "collection-name: mycollection3 -- maximum peer count (1) cannot be greater than the required peer count (2)")

	// Test 12: AND concatenation of orgs in access policy -> error
	requiredPeerCount = 1
	maximumPeerCount = 2
	policyEnvelope = cauthdsl.Envelope(cauthdsl.And(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	coll3 = createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "collection-name: mycollection3 -- error in member org policy: signature policy is not an OR concatenation, NOutOf 2")

	// Test 13: deploy with existing collection config on the ledger -> error
	ccp := &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	state["lscc"][privdata.BuildCollectionKVSKey(ccid)] = ccpBytes
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "collection data should not exist for chaincode mycc:1.0")
}

func TestValidateRWSetAndCollectionForUpgrade(t *testing.T) {
	chid := "ch"
	ccid := "mycc"
	ccver := "1.0"
	cdRWSet := &ccprovider.ChaincodeData{Name: ccid, Version: ccver}

	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ac := capabilities.NewApplicationProvider(map[string]*common.Capability{
		capabilities.ApplicationV1_2: {},
	})

	lsccFunc := lscc.UPGRADE

	collName1 := "mycollection1"
	collName2 := "mycollection2"
	collName3 := "mycollection3"
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 3
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll3 := createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	ccp := &common.CollectionConfigPackage{Config: []*common.CollectionConfig{coll1, coll2}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)

	// Test 1: no existing collection config package -> success
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1}, cdRWSet, lsccFunc, ac, chid)
	assert.NoError(t, err)

	state["lscc"][privdata.BuildCollectionKVSKey(ccid)] = ccpBytes

	// Test 2: exactly same as the existing collection config package -> success
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2}, cdRWSet, lsccFunc, ac, chid)
	assert.NoError(t, err)

	// Test 3: missing one existing collection (check based on the length) -> error
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "the following existing collections are missing in the new collection configuration package: [mycollection2]")

	// Test 4: missing one existing collection (check based on the collection names) -> error
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "the following existing collections are missing in the new collection configuration package: [mycollection2]")

	// Test 5: adding a new collection along with the existing collections -> success
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.NoError(t, err)

	newBlockToLive := blockToLive + 1
	coll2 = createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, newBlockToLive)

	// Test 6: modify the BlockToLive in an existing collection -> error
	err = testValidateCollection(t, v, []*common.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	assert.EqualError(t, err, "the BlockToLive in the following existing collections must not be modified: [mycollection2]")
}

var lccctestpath = "/tmp/lscc-validation-test"

func TestMain(m *testing.M) {
	ccprovider.SetChaincodesPath(lccctestpath)
	policy.RegisterPolicyCheckerFactory(&mockPolicyCheckerFactory{})

	mspGetter := func(cid string) []string {
		return []string{"SampleOrg"}
	}

	per.MockSetMSPIDGetter(mspGetter)

	var err error

	// setup the MSP manager so that we can sign/verify
	msptesttools.LoadMSPSetupForTesting()

	id, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("GetSigningIdentity failed with err %s", err)
		os.Exit(-1)
	}

	sid, err = id.Serialize()
	if err != nil {
		fmt.Printf("Serialize failed with err %s", err)
		os.Exit(-1)
	}

	// determine the MSP identifier for the first MSP in the default chain
	var msp msp.MSP
	mspMgr := mspmgmt.GetManagerForChain(chainId)
	msps, err := mspMgr.GetMSPs()
	if err != nil {
		fmt.Printf("Could not retrieve the MSPs for the chain manager, err %s", err)
		os.Exit(-1)
	}
	if len(msps) == 0 {
		fmt.Printf("At least one MSP was expected")
		os.Exit(-1)
	}
	for _, m := range msps {
		msp = m
		break
	}
	mspid, err = msp.GetIdentifier()
	if err != nil {
		fmt.Printf("Failure getting the msp identifier, err %s", err)
		os.Exit(-1)
	}

	// also set the MSP for the "test" chain
	mspmgmt.XXXSetMSPManager("mycc", mspmgmt.GetManagerForChain(util.GetTestChainID()))

	os.Exit(m.Run())
}

func TestInValidCollectionName(t *testing.T) {
	validNames := []string{"collection1", "collection_2"}
	inValidNames := []string{"collection.1", "collection%2", ""}

	for _, name := range validNames {
		assert.NoError(t, validateCollectionName(name), "Testing for name = "+name)
	}
	for _, name := range inValidNames {
		assert.Error(t, validateCollectionName(name), "Testing for name = "+name)
	}
}
