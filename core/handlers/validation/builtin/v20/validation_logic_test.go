/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v20

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v14"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	vs "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin/v20/mocks"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/capabilities.go -fake-name Capabilities . capabilities

type capabilities interface {
	validation.Capabilities
}

//go:generate counterfeiter -o mocks/state.go -fake-name State . state

type state interface {
	vs.State
}

//go:generate counterfeiter -o mocks/state_fetcher.go -fake-name StateFetcher . stateFetcher

type stateFetcher interface {
	vs.StateFetcher
}

func createTx(endorsedByDuplicatedIdentity bool) (*common.Envelope, error) {
	ccid := &peer.ChaincodeID{Name: "foo", Version: "v1"}
	cis := &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{ChaincodeId: ccid}}

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "testchannelid", cis, sid)
	if err != nil {
		return nil, err
	}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, []byte("res"), nil, ccid, id)
	if err != nil {
		return nil, err
	}

	var env *common.Envelope
	if endorsedByDuplicatedIdentity {
		env, err = protoutil.CreateSignedTx(prop, id, presp, presp)
	} else {
		env, err = protoutil.CreateSignedTx(prop, id, presp)
	}
	if err != nil {
		return nil, err
	}
	return env, err
}

func getSignedByMSPMemberPolicy(mspID string) ([]byte, error) {
	p := policydsl.SignedByMspMember(mspID)

	b, err := protoutil.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func newValidationInstance(state map[string]map[string][]byte) *Validator {
	vs := &mocks.State{}
	vs.GetStateMultipleKeysStub = func(namespace string, keys []string) ([][]byte, error) {
		if ns, ok := state[namespace]; ok {
			return [][]byte{ns[keys[0]]}, nil
		} else {
			return nil, fmt.Errorf("could not retrieve namespace %s", namespace)
		}
	}
	sf := &mocks.StateFetcher{}
	sf.FetchStateReturns(vs, nil)
	return newCustomValidationInstance(sf, &mocks.Capabilities{})
}

func newCustomValidationInstance(sf vs.StateFetcher, c validation.Capabilities) *Validator {
	sbvm := &mocks.StateBasedValidator{}
	sbvm.On("PreValidate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sbvm.On("PostValidate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mockCR := &mocks.CollectionResources{}
	mockCR.On("CollectionValidationInfo", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, nil)

	is := &mocks.IdentityDeserializer{}
	pe := &txvalidator.PolicyEvaluator{
		IdentityDeserializer: mspmgmt.GetManagerForChain("testchannelid"),
	}
	v := New(c, sf, is, pe, mockCR)

	v.stateBasedValidator = sbvm
	return v
}

func TestStateBasedValidationFailure(t *testing.T) {
	sbvm := &mocks.StateBasedValidator{}
	sbvm.On("PreValidate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sbvm.On("PostValidate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	mockCR := &mocks.CollectionResources{}
	mockCR.On("CollectionValidationInfo", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, nil)

	sf := &mocks.StateFetcher{}
	is := &mocks.IdentityDeserializer{}
	pe := &txvalidator.PolicyEvaluator{
		IdentityDeserializer: mspmgmt.GetManagerForChain("testchannelid"),
	}
	v := New(&mocks.Capabilities{}, sf, is, pe, mockCR)
	v.stateBasedValidator = sbvm

	tx, err := createTx(false)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
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
	require.Error(t, err)
	require.IsType(t, &commonerrors.VSCCEndorsementPolicyError{}, err)

	// bad path: execution error
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCExecutionFailureError{Err: fmt.Errorf("some sbe validation err")}).Once()
	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)
	require.IsType(t, &commonerrors.VSCCExecutionFailureError{}, err)

	// good path: signed by the right MSP
	sbvm.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	err = v.Validate(b, "foo", 0, 0, policy)
	require.NoError(t, err)
}

func TestInvoke(t *testing.T) {
	v := newValidationInstance(make(map[string]map[string][]byte))

	// broken Envelope
	var err error
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{[]byte("a")}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	require.Error(t, err)

	// (still) broken Envelope
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{Payload: []byte("barf")})}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	require.Error(t, err)

	// (still) broken Envelope
	e := protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: []byte("barf")}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	require.Error(t, err)

	tx, err := createTx(false)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// broken type
	e = protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_ORDERER_TRANSACTION)})}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)

	// broken tx payload
	e = protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_ORDERER_TRANSACTION)})}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)

	// good path: signed by the right MSP
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.NoError(t, err)
}

func TestToApplicationPolicyTranslator_Translate(t *testing.T) {
	tr := &toApplicationPolicyTranslator{}
	res, err := tr.Translate(nil)
	require.NoError(t, err)
	require.Nil(t, res)

	res, err = tr.Translate([]byte("barf"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not unmarshal signature policy envelope")
	require.Nil(t, res)

	res, err = tr.Translate(protoutil.MarshalOrPanic(policydsl.SignedByMspMember("the right honourable member for Ipswich")))
	require.NoError(t, err)
	require.Equal(t, res, protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: policydsl.SignedByMspMember("the right honourable member for Ipswich"),
		},
	}))
}

var (
	id        msp.SigningIdentity
	sid       []byte
	mspid     string
	channelID string = "testchannelid"
)

func TestMain(m *testing.M) {
	code := -1
	defer func() {
		os.Exit(code)
	}()
	testDir, err := ioutil.TempDir("", "v1.3-validation")
	if err != nil {
		fmt.Printf("Could not create temp dir: %s", err)
		return
	}
	defer os.RemoveAll(testDir)
	ccprovider.SetChaincodesPath(testDir)

	// setup the MSP manager so that we can sign/verify
	msptesttools.LoadMSPSetupForTesting()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		fmt.Printf("Initialize cryptoProvider bccsp failed: %s", err)
		return
	}

	id, err = mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("GetDefaultSigningIdentity failed with err %s", err)
		return
	}

	sid, err = id.Serialize()
	if err != nil {
		fmt.Printf("Serialize failed with err %s", err)
		return
	}

	// determine the MSP identifier for the first MSP in the default chain
	var msp msp.MSP
	mspMgr := mspmgmt.GetManagerForChain(channelID)
	msps, err := mspMgr.GetMSPs()
	if err != nil {
		fmt.Printf("Could not retrieve the MSPs for the chain manager, err %s", err)
		return
	}
	if len(msps) == 0 {
		fmt.Printf("At least one MSP was expected")
		return
	}
	for _, m := range msps {
		msp = m
		break
	}
	mspid, err = msp.GetIdentifier()
	if err != nil {
		fmt.Printf("Failure getting the msp identifier, err %s", err)
		return
	}

	// also set the MSP for the "test" chain
	mspmgmt.XXXSetMSPManager("mycc", mspmgmt.GetManagerForChain("testchannelid"))

	code = m.Run()
}
