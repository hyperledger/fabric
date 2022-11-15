/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package v12

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v14"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	vs "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin/v12/mocks"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/state.go -fake-name State . vsState

type vsState interface {
	vs.State
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

func createCCDataRWsetWithCollection(nameK, nameV, version string, policy []byte, collectionConfigPackage []byte) ([]byte, error) {
	cd := &ccprovider.ChaincodeData{
		Name:                nameV,
		Version:             version,
		InstantiationPolicy: policy,
	}

	cdbytes := protoutil.MarshalOrPanic(cd)

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

	cdbytes := protoutil.MarshalOrPanic(cd)

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

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "testchannelid", cis, sid)
	if err != nil {
		return nil, err
	}

	ccid := &peer.ChaincodeID{Name: ccname, Version: ccver}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, res, nil, ccid, id)
	if err != nil {
		return nil, err
	}

	return protoutil.CreateSignedTx(prop, id, presp)
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

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "testchannelid", cis, sid)
	if err != nil {
		return nil, err
	}

	ccid := &peer.ChaincodeID{Name: ccname, Version: ccver}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, res, nil, ccid, id)
	if err != nil {
		return nil, err
	}

	return protoutil.CreateSignedTx(prop, id, presp)
}

func getSignedByMSPMemberPolicy(mspID string) ([]byte, error) {
	p := policydsl.SignedByMspMember(mspID)

	b, err := protoutil.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func getSignedByOneMemberTwicePolicy(mspID string) ([]byte, error) {
	principal := &mspproto.MSPPrincipal{
		PrincipalClassification: mspproto.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&mspproto.MSPRole{Role: mspproto.MSPRole_MEMBER, MspIdentifier: mspID}),
	}

	p := &common.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       policydsl.NOutOf(2, []*common.SignaturePolicy{policydsl.SignedBy(0), policydsl.SignedBy(0)}),
		Identities: []*mspproto.MSPPrincipal{principal},
	}
	b, err := protoutil.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func getSignedByMSPAdminPolicy(mspID string) ([]byte, error) {
	p := policydsl.SignedByMspAdmin(mspID)

	b, err := protoutil.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
}

func newValidationInstance(state map[string]map[string][]byte) *Validator {
	c := &mocks.Capabilities{}
	c.On("PrivateChannelData").Return(false)
	c.On("V1_1Validation").Return(false)
	c.On("V1_2Validation").Return(false)
	vs := &mocks.State{}
	vs.GetStateMultipleKeysStub = func(namespace string, keys []string) ([][]byte, error) {
		if ns, ok := state[namespace]; ok {
			return [][]byte{ns[keys[0]]}, nil
		} else {
			return nil, fmt.Errorf("could not retrieve namespace %s", namespace)
		}
	}
	sf := &mocks.StateFetcher{}
	sf.On("FetchState").Return(vs, nil)
	return newCustomValidationInstance(sf, c)
}

func newCustomValidationInstance(sf StateFetcher, c validation.Capabilities) *Validator {
	is := &mocks.IdentityDeserializer{}
	pe := &txvalidator.PolicyEvaluator{
		IdentityDeserializer: mspmgmt.GetManagerForChain("testchannelid"),
	}
	return New(c, sf, is, pe)
}

func TestDeduplicateIdentity(t *testing.T) {
	// We allocate a slice with capacity greater than the length
	proposalResponsePayload := []byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}
	prpBuff := make([]byte, len(proposalResponsePayload), len(proposalResponsePayload)*2)
	copy(prpBuff, proposalResponsePayload)

	identity1 := protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
		IdBytes: []byte{1, 1, 1},
	})
	identity2 := protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
		IdBytes: []byte{2, 2, 2},
	})

	chaincodeActionPayload := &peer.ChaincodeActionPayload{
		Action: &peer.ChaincodeEndorsedAction{
			Endorsements: []*peer.Endorsement{
				{
					Endorser: identity1,
				},
				{
					Endorser: identity2,
				},
			},
			ProposalResponsePayload: prpBuff,
		},
	}

	signedData, err := (&Validator{}).deduplicateIdentity(chaincodeActionPayload)
	require.NoError(t, err)
	// The original bytes of proposalResponsePayload are preserved
	require.Equal(t, proposalResponsePayload, signedData[0].Data[:len(proposalResponsePayload)])
	require.Equal(t, proposalResponsePayload, signedData[1].Data[:len(proposalResponsePayload)])
	// And are suffixed with the identity bytes
	require.Equal(t, identity1, signedData[0].Data[len(proposalResponsePayload):])
	require.Equal(t, identity2, signedData[1].Data[len(proposalResponsePayload):])
}

func TestInvoke(t *testing.T) {
	v := newValidationInstance(make(map[string]map[string][]byte))

	// broken Envelope
	var err error
	b := &common.Block{Data: &common.BlockData{Data: [][]byte{[]byte("a")}}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	require.Error(t, err)

	// (still) broken Envelope
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{Payload: []byte("barf")})}}}
	err = v.Validate(b, "foo", 0, 0, []byte("a"))
	require.Error(t, err)

	// (still) broken Envelope
	e := protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: []byte("barf")}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}}
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

	// broken policy
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "foo", 0, 0, []byte("barf"))
	require.Error(t, err)

	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// broken type
	e = protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_ORDERER_TRANSACTION)})}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)

	// broken tx payload
	e = protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_ORDERER_TRANSACTION)})}})})
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{e}}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)

	// good path: signed by the right MSP
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.NoError(t, err)

	// bad path: signed by the wrong MSP
	policy, err = getSignedByMSPMemberPolicy("barf")
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)

	// bad path: signed by duplicated MSP identity
	policy, err = getSignedByOneMemberTwicePolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}
	tx, err = createTx(true)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}
	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}
	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "foo", 0, 0, policy)
	require.Error(t, err)
}

func TestRWSetTooBig(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "1"

	cd := &ccprovider.ChaincodeData{
		Name:                ccname,
		Version:             ccver,
		InstantiationPolicy: nil,
	}

	cdbytes := protoutil.MarshalOrPanic(cd)

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", ccname, cdbytes)
	rwsetBuilder.AddToWriteSet("lscc", "spurious", []byte("spurious"))

	sr, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	srBytes, err := sr.GetPubSimulationBytes()
	require.NoError(t, err)
	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, srBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "LSCC can only issue a single putState upon deploy")
	t.Logf("error: %s", err)
}

func TestValidateDeployFail(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "1"

	/*********************/
	/* test no write set */
	/*********************/

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, nil)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "No read write set for lscc was found")

	/************************/
	/* test bogus write set */
	/************************/

	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", ccname, []byte("barf"))
	sr, err := rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	resBogusBytes, err := sr.GetPubSimulationBytes()
	require.NoError(t, err)
	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, resBogusBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.ErrorContains(t, err, "unmarshalling of ChaincodeData failed")

	/**********************/
	/* test bad LSCC args */
	/**********************/

	res, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err = createLSCCTxPutCds(ccname, ccver, lscc.DEPLOY, res, nil, false)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "Wrong number of arguments for invocation lscc(deploy): expected at least 2, received 1")

	/**********************/
	/* test bad LSCC args */
	/**********************/

	res, err = createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err = createLSCCTxPutCds(ccname, ccver, lscc.DEPLOY, res, []byte("barf"), true)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.ErrorContains(t, err, "GetChaincodeDeploymentSpec error error unmarshalling ChaincodeDeploymentSpec")

	/***********************/
	/* test bad cc version */
	/***********************/

	res, err = createCCDataRWset(ccname, ccname, ccver+".1", nil)
	require.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "expected cc version 1, found 1.1")

	/*************/
	/* bad rwset */
	/*************/

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, []byte("barf"))
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.ErrorContains(t, err, "txRWSet.FromProtoBytes error")

	/********************/
	/* test bad cc name */
	/********************/

	res, err = createCCDataRWset(ccname+".badbad", ccname, ccver, nil)
	require.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "expected key mycc, found mycc.badbad")

	/**********************/
	/* test bad cc name 2 */
	/**********************/

	res, err = createCCDataRWset(ccname, ccname+".badbad", ccver, nil)
	require.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "expected cc name mycc, found mycc.badbad")

	/************************/
	/* test spurious writes */
	/************************/

	cd := &ccprovider.ChaincodeData{
		Name:                ccname,
		Version:             ccver,
		InstantiationPolicy: nil,
	}

	cdbytes := protoutil.MarshalOrPanic(cd)
	rwsetBuilder = rwsetutil.NewRWSetBuilder()
	rwsetBuilder.AddToWriteSet("lscc", ccname, cdbytes)
	rwsetBuilder.AddToWriteSet("bogusbogus", "key", []byte("val"))
	sr, err = rwsetBuilder.GetTxSimulationResults()
	require.NoError(t, err)
	srBytes, err := sr.GetPubSimulationBytes()
	require.NoError(t, err)
	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, srBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "LSCC invocation is attempting to write to namespace bogusbogus")
}

func TestAlreadyDeployed(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "alreadydeployed"

	// create state for ccname to simulate deployment
	state["lscc"][ccname] = []byte{}

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	require.EqualError(t, err, "Chaincode mycc is already instantiated")
}

func TestValidateDeployNoLedger(t *testing.T) {
	sf := &mocks.StateFetcher{}
	sf.On("FetchState").Return(nil, errors.New("failed obtaining query executor"))
	capabilities := &mocks.Capabilities{}
	capabilities.On("PrivateChannelData").Return(false)
	capabilities.On("V1_1Validation").Return(false)
	capabilities.On("V1_2Validation").Return(false)
	v := newCustomValidationInstance(sf, capabilities)

	ccname := "mycc"
	ccver := "1"

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	require.NoError(t, err)
	res, err := createCCDataRWset(ccname, ccname, ccver, defaultPolicy)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "could not retrieve QueryExecutor for channel testchannelid, error failed obtaining query executor")
}

func TestValidateDeployNOKNilChaincodeSpec(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "1"

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	require.NoError(t, err)
	res, err := createCCDataRWset(ccname, ccname, ccver, defaultPolicy)
	require.NoError(t, err)

	// Create a ChaincodeDeploymentSpec with nil ChaincodeSpec for negative test
	cdsBytes, err := proto.Marshal(&peer.ChaincodeDeploymentSpec{})
	require.NoError(t, err)

	// ChaincodeDeploymentSpec/ChaincodeSpec are derived from cdsBytes (i.e., cis.ChaincodeSpec.Input.Args[2])
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: "lscc"},
			Input: &peer.ChaincodeInput{
				Args: [][]byte{[]byte(lscc.DEPLOY), []byte("barf"), cdsBytes},
			},
			Type: peer.ChaincodeSpec_GOLANG,
		},
	}

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "testchannelid", cis, sid)
	require.NoError(t, err)

	ccid := &peer.ChaincodeID{Name: ccname, Version: ccver}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, &peer.Response{Status: 200}, res, nil, ccid, id)
	require.NoError(t, err)

	env, err := protoutil.CreateSignedTx(prop, id, presp)
	require.NoError(t, err)

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(env)}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "VSCC error: invocation of lscc(deploy) does not have appropriate arguments")
}

func TestValidateDeployOK(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "1"

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	require.NoError(t, err)
	res, err := createCCDataRWset(ccname, ccname, ccver, defaultPolicy)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.NoError(t, err)
}

func TestValidateDeployNOK(t *testing.T) {
	testCases := []struct {
		description string
		ccName      string
		ccVersion   string
		errMsg      string
	}{
		{description: "empty cc name", ccName: "", ccVersion: "1", errMsg: "invalid chaincode name ''"},
		{description: "bad first character in cc name", ccName: "_badname", ccVersion: "1.2", errMsg: "invalid chaincode name '_badname'"},
		{description: "bad character in cc name", ccName: "bad.name", ccVersion: "1-5", errMsg: "invalid chaincode name 'bad.name'"},
		{description: "empty cc version", ccName: "1good_name", ccVersion: "", errMsg: "invalid chaincode version ''"},
		{description: "bad cc version", ccName: "good-name", ccVersion: "$badversion", errMsg: "invalid chaincode version '$badversion'"},
		{description: "use system cc name", ccName: "qscc", ccVersion: "2.1", errMsg: "chaincode name 'qscc' is reserved for system chaincodes"},
	}

	// create validator and policy
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	policy, err := getSignedByMSPAdminPolicy(mspid)
	require.NoError(t, err)

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			testChaincodeDeployNOK(t, tc.ccName, tc.ccVersion, tc.errMsg, v, policy)
		})
	}
}

func testChaincodeDeployNOK(t *testing.T, ccName, ccVersion, errMsg string, v *Validator, policy []byte) {
	res, err := createCCDataRWset(ccName, ccName, ccVersion, policy)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccName, ccVersion, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}, Header: &common.BlockHeader{}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, errMsg)
}

func TestValidateDeployWithCollection(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	vs := &mocks.State{}
	vs.GetStateMultipleKeysStub = func(namespace string, keys []string) ([][]byte, error) {
		if ns, ok := state[namespace]; ok {
			return [][]byte{ns[keys[0]]}, nil
		} else {
			return nil, fmt.Errorf("could not retrieve namespace %s", namespace)
		}
	}
	sf := &mocks.StateFetcher{}
	sf.On("FetchState").Return(vs, nil)
	capabilities := &mocks.Capabilities{}
	capabilities.On("PrivateChannelData").Return(true)
	capabilities.On("V1_1Validation").Return(true)
	capabilities.On("V1_2Validation").Return(false)
	v := newCustomValidationInstance(sf, capabilities)

	ccname := "mycc"
	ccver := "1"

	collName1 := "mycollection1"
	collName2 := "mycollection2"
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 1000
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	// Test 1: Deploy chaincode with a valid collection configs --> success
	ccp := &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll2}}
	ccpBytes, err := proto.Marshal(ccp)
	require.NoError(t, err)
	require.NotNil(t, ccpBytes)

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	require.NoError(t, err)
	res, err := createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
	require.NoError(t, err)

	tx, err := createLSCCTxWithCollection(ccname, ccver, lscc.DEPLOY, res, defaultPolicy, ccpBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.NoError(t, err)

	// Test 2: Deploy the chaincode with duplicate collection configs --> no error as the
	// peer is not in V1_2Validation mode
	ccp = &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll2, coll1}}
	ccpBytes, err = proto.Marshal(ccp)
	require.NoError(t, err)
	require.NotNil(t, ccpBytes)

	res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
	require.NoError(t, err)

	tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.DEPLOY, res, defaultPolicy, ccpBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.NoError(t, err)

	// Test 3: Once the V1_2Validation is enabled, validation should fail due to duplicate collection configs
	capabilities = &mocks.Capabilities{}
	capabilities.On("PrivateChannelData").Return(true)
	capabilities.On("V1_2Validation").Return(true)
	v = newCustomValidationInstance(sf, capabilities)

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "collection-name: mycollection1 -- found duplicate collection configuration")
}

func TestValidateDeployWithPolicies(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "1"

	/*********************************************/
	/* test 1: success with an accept-all policy */
	/*********************************************/

	res, err := createCCDataRWset(ccname, ccname, ccver, policydsl.MarshaledAcceptAllPolicy)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.NoError(t, err)

	/********************************************/
	/* test 2: failure with a reject-all policy */
	/********************************************/

	res, err = createCCDataRWset(ccname, ccname, ccver, policydsl.MarshaledRejectAllPolicy)
	require.NoError(t, err)

	tx, err = createLSCCTx(ccname, ccver, lscc.DEPLOY, res)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err = protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err = getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "chaincode instantiation policy violated, error signature set did not satisfy policy")
}

func TestInvalidUpgrade(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	b := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(b, "lscc", 0, 0, policy)
	require.EqualError(t, err, "Upgrading non-existent chaincode mycc")
}

func TestValidateUpgradeOK(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "upgradeok"
	ccver = "2"

	// policy signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// create lscc record
	cd := &ccprovider.ChaincodeData{
		InstantiationPolicy: policy,
	}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		t.Fatalf("Failed to marshal ChaincodeData: %s", err)
	}
	state["lscc"][ccname] = cdbytes

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	require.NoError(t, err)
}

func TestInvalidateUpgradeBadVersion(t *testing.T) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "upgradebadversion"

	// policy signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// create lscc record
	cd := &ccprovider.ChaincodeData{
		InstantiationPolicy: policy,
		Version:             ccver,
	}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		t.Fatalf("Failed to marshal ChaincodeData: %s", err)
	}
	state["lscc"][ccname] = cdbytes

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	require.EqualError(t, err, fmt.Sprintf("Existing version of the cc on the ledger (%s) should be different from the upgraded one", ccver))
}

func validateUpgradeWithCollection(t *testing.T, V1_2Validation bool) {
	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	vs := &mocks.State{}
	vs.GetStateMultipleKeysStub = func(namespace string, keys []string) ([][]byte, error) {
		if ns, ok := state[namespace]; ok {
			return [][]byte{ns[keys[0]]}, nil
		} else {
			return nil, fmt.Errorf("could not retrieve namespace %s", namespace)
		}
	}
	sf := &mocks.StateFetcher{}
	sf.On("FetchState").Return(vs, nil)
	capabilities := &mocks.Capabilities{}
	capabilities.On("PrivateChannelData").Return(true)
	capabilities.On("V1_1Validation").Return(true)
	capabilities.On("V1_2Validation").Return(V1_2Validation)
	v := newCustomValidationInstance(sf, capabilities)

	ccname := "mycc"
	ccver := "2"

	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// create lscc record
	cd := &ccprovider.ChaincodeData{
		InstantiationPolicy: policy,
	}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		t.Fatalf("Failed to marshal ChaincodeData: %s", err)
	}
	state["lscc"][ccname] = cdbytes

	collName1 := "mycollection1"
	collName2 := "mycollection2"
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
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
	ccp := &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll2}}
	ccpBytes, err := proto.Marshal(ccp)
	require.NoError(t, err)
	require.NotNil(t, ccpBytes)

	defaultPolicy, err := getSignedByMSPAdminPolicy(mspid)
	require.NoError(t, err)
	res, err := createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
	require.NoError(t, err)

	tx, err := createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	if V1_2Validation {
		require.NoError(t, err)
	} else {
		require.Error(t, err, "LSCC can only issue a single putState upon deploy/upgrade")
	}

	state["lscc"][privdata.BuildCollectionKVSKey(ccname)] = ccpBytes

	if V1_2Validation {
		ccver = "3"

		collName3 := "mycollection3"
		coll3 := createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

		// Test 2: some existing collections are missing in the updated config and peer in
		// V1_2Validation mode --> error
		ccp = &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll3}}
		ccpBytes, err = proto.Marshal(ccp)
		require.NoError(t, err)
		require.NotNil(t, ccpBytes)

		res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
		require.NoError(t, err)

		tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
		if err != nil {
			t.Fatalf("createTx returned err %s", err)
		}

		envBytes, err = protoutil.GetBytesEnvelope(tx)
		if err != nil {
			t.Fatalf("GetBytesEnvelope returned err %s", err)
		}

		bl = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		require.Error(t, err, "Some existing collection configurations are missing in the new collection configuration package")

		ccver = "3"

		// Test 3: some existing collections are missing in the updated config and peer in
		// V1_2Validation mode --> error
		ccp = &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll3}}
		ccpBytes, err = proto.Marshal(ccp)
		require.NoError(t, err)
		require.NotNil(t, ccpBytes)

		res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
		require.NoError(t, err)

		tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
		if err != nil {
			t.Fatalf("createTx returned err %s", err)
		}

		envBytes, err = protoutil.GetBytesEnvelope(tx)
		if err != nil {
			t.Fatalf("GetBytesEnvelope returned err %s", err)
		}

		bl = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		require.Error(t, err, "existing collection named mycollection2 is missing in the new collection configuration package")

		ccver = "3"

		// Test 4: valid collection config and peer in V1_2Validation mode --> success
		ccp = &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll2, coll3}}
		ccpBytes, err = proto.Marshal(ccp)
		require.NoError(t, err)
		require.NotNil(t, ccpBytes)

		res, err = createCCDataRWsetWithCollection(ccname, ccname, ccver, defaultPolicy, ccpBytes)
		require.NoError(t, err)

		tx, err = createLSCCTxWithCollection(ccname, ccver, lscc.UPGRADE, res, defaultPolicy, ccpBytes)
		if err != nil {
			t.Fatalf("createTx returned err %s", err)
		}

		envBytes, err = protoutil.GetBytesEnvelope(tx)
		if err != nil {
			t.Fatalf("GetBytesEnvelope returned err %s", err)
		}

		bl = &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		require.NoError(t, err)
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
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	ccname := "mycc"
	ccver := "upgradewithpoliciesok"

	// policy signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	// create lscc record
	cd := &ccprovider.ChaincodeData{
		InstantiationPolicy: policy,
	}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		t.Fatalf("Failed to marshal ChaincodeData: %s", err)
	}
	state["lscc"][ccname] = cdbytes

	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	require.NoError(t, err)
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
	state["lscc"] = make(map[string][]byte)

	vs := &mocks.State{}
	vs.GetStateMultipleKeysStub = func(namespace string, keys []string) ([][]byte, error) {
		if ns, ok := state[namespace]; ok {
			return [][]byte{ns[keys[0]]}, nil
		} else {
			return nil, fmt.Errorf("could not retrieve namespace %s", namespace)
		}
	}
	sf := &mocks.StateFetcher{}
	sf.On("FetchState").Return(vs, nil)
	capabilities := &mocks.Capabilities{}
	capabilities.On("PrivateChannelData").Return(true)
	capabilities.On("V1_1Validation").Return(v11capability)
	capabilities.On("V1_2Validation").Return(false)
	v := newCustomValidationInstance(sf, capabilities)

	ccname := "mycc"
	ccver := "2"

	// create lscc record with accept all instantiation policy
	ipbytes, err := proto.Marshal(policydsl.AcceptAllPolicy)
	if err != nil {
		t.Fatalf("Failed to marshal AcceptAllPolicy: %s", err)
	}
	cd := &ccprovider.ChaincodeData{
		InstantiationPolicy: ipbytes,
	}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		t.Fatalf("Failed to marshal ChaincodeData: %s", err)
	}
	state["lscc"][ccname] = cdbytes

	// now we upgrade, with v 2 of the same cc, with the crucial difference that it has a reject all IP
	ccver = ccver + ".2"

	simresres, err := createCCDataRWset(ccname, ccname, ccver,
		policydsl.MarshaledRejectAllPolicy, // here's where we specify the IP of the upgraded cc
	)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
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

	// execute the upgrade tx
	if expecterr {
		bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		require.Error(t, err)
	} else {
		bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
		err = v.Validate(bl, "lscc", 0, 0, policy)
		require.NoError(t, err)
	}
}

func TestValidateUpgradeWithPoliciesFail(t *testing.T) {
	ccname := "mycc"
	ccver := "upgradewithpoliciesfail"

	state := make(map[string]map[string][]byte)
	state["lscc"] = make(map[string][]byte)

	v := newValidationInstance(state)

	// create lscc record with reject all instantiation policy
	ipbytes, err := proto.Marshal(policydsl.RejectAllPolicy)
	if err != nil {
		t.Fatalf("Failed to marshal RejectAllPolicy: %s", err)
	}
	cd := &ccprovider.ChaincodeData{
		InstantiationPolicy: ipbytes,
		Version:             ccver,
	}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		t.Fatalf("Failed to marshal ChaincodeData: %s", err)
	}
	state["lscc"][ccname] = cdbytes

	ccver = "2"
	simresres, err := createCCDataRWset(ccname, ccname, ccver, nil)
	require.NoError(t, err)

	tx, err := createLSCCTx(ccname, ccver, lscc.UPGRADE, simresres)
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
	}

	envBytes, err := protoutil.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
	}

	// good path: signed by the right MSP
	policy, err := getSignedByMSPMemberPolicy(mspid)
	if err != nil {
		t.Fatalf("failed getting policy, err %s", err)
	}

	bl := &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}
	err = v.Validate(bl, "lscc", 0, 0, policy)
	require.EqualError(t, err, "chaincode instantiation policy violated, error signature set did not satisfy policy")
}

var (
	id      msp.SigningIdentity
	sid     []byte
	mspid   string
	chainId string = "testchannelid"
)

func createCollectionConfig(collectionName string, signaturePolicyEnvelope *common.SignaturePolicyEnvelope,
	requiredPeerCount int32, maximumPeerCount int32, blockToLive uint64,
) *peer.CollectionConfig {
	signaturePolicy := &peer.CollectionPolicyConfig_SignaturePolicy{
		SignaturePolicy: signaturePolicyEnvelope,
	}
	accessPolicy := &peer.CollectionPolicyConfig{
		Payload: signaturePolicy,
	}

	return &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:              collectionName,
				MemberOrgsPolicy:  accessPolicy,
				RequiredPeerCount: requiredPeerCount,
				MaximumPeerCount:  maximumPeerCount,
				BlockToLive:       blockToLive,
			},
		},
	}
}

func testValidateCollection(t *testing.T, v *Validator, collectionConfigs []*peer.CollectionConfig, cdRWSet *ccprovider.ChaincodeData,
	lsccFunc string, ac channelconfig.ApplicationCapabilities, chid string,
) error {
	ccp := &peer.CollectionConfigPackage{Config: collectionConfigs}
	ccpBytes, err := proto.Marshal(ccp)
	require.NoError(t, err)
	require.NotNil(t, ccpBytes)

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
	require.EqualError(t, err, "LSCC can only issue one or two putState upon deploy")

	// Test 2: Invalid key for the collection config package -> error
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: "b"}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, nil, lsccFunc, ac, chid)
	require.EqualError(t, err, "invalid key for the collection of chaincode mycc:1.0; expected 'mycc~collection', received 'b'")

	// Test 3: No collection config package -> success
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, nil, lsccFunc, ac, chid)
	require.NoError(t, err)

	lsccargs := [][]byte{nil, nil, nil, nil, nil, nil}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	require.NoError(t, err)

	// Test 4: Valid key for the collection config package -> success
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: privdata.BuildCollectionKVSKey(ccid)}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	require.NoError(t, err)

	// Test 5: Collection configuration of the lscc args doesn't match the rwset
	lsccargs = [][]byte{nil, nil, nil, nil, nil, []byte("barf")}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	require.EqualError(t, err, "collection configuration arguments supplied for chaincode mycc:1.0 do not match the configuration in the lscc writeset")

	// Test 6: Invalid collection config package -> error
	rwset = &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: ccid}, {Key: privdata.BuildCollectionKVSKey("mycc"), Value: []byte("barf")}}}
	err = v.validateRWSetAndCollection(rwset, cdRWSet, lsccargs, lsccFunc, ac, chid)
	require.EqualError(t, err, "invalid collection configuration supplied for chaincode mycc:1.0")

	// Test 7: Valid collection config package -> success
	collName1 := "mycollection1"
	collName2 := "mycollection2"
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 10000
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2}, cdRWSet, lsccFunc, ac, chid)
	require.NoError(t, err)

	// Test 8: Duplicate collections in the collection config package -> success as the peer is in v1.1 validation mode
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll1}, cdRWSet, lsccFunc, ac, chid)
	require.NoError(t, err)

	// Test 9: requiredPeerCount > maximumPeerCount -> success as the peer is in v1.1 validation mode
	collName3 := "mycollection3"
	requiredPeerCount = 2
	maximumPeerCount = 1
	blockToLive = 10000
	coll3 := createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	require.NoError(t, err)

	// Enable v1.2 validation mode
	ac = capabilities.NewApplicationProvider(map[string]*common.Capability{
		capabilities.ApplicationV1_2: {},
	})

	// Test 10: Duplicate collections in the collection config package -> error
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll1}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "collection-name: mycollection1 -- found duplicate collection configuration")

	// Test 11: requiredPeerCount < 0 -> error
	requiredPeerCount = -2
	maximumPeerCount = 1
	blockToLive = 10000
	coll3 = createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "collection-name: mycollection3 -- requiredPeerCount (-2) cannot be less than zero",
		collName3, maximumPeerCount, requiredPeerCount)

	// Test 11: requiredPeerCount > maximumPeerCount -> error
	requiredPeerCount = 2
	maximumPeerCount = 1
	blockToLive = 10000
	coll3 = createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "collection-name: mycollection3 -- maximum peer count (1) cannot be less than the required peer count (2)")

	// Test 12: AND concatenation of orgs in access policy -> error
	requiredPeerCount = 1
	maximumPeerCount = 2
	policyEnvelope = policydsl.Envelope(policydsl.And(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
	coll3 = createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll3}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "collection-name: mycollection3 -- error in member org policy: signature policy is not an OR concatenation, NOutOf 2")

	// Test 13: deploy with existing collection config on the ledger -> error
	ccp := &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1}}
	ccpBytes, err := proto.Marshal(ccp)
	require.NoError(t, err)
	state["lscc"][privdata.BuildCollectionKVSKey(ccid)] = ccpBytes
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "collection data should not exist for chaincode mycc:1.0")
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
	signers := [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := policydsl.Envelope(policydsl.Or(policydsl.SignedBy(0), policydsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	var blockToLive uint64
	requiredPeerCount = 1
	maximumPeerCount = 2
	blockToLive = 3
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll2 := createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)
	coll3 := createCollectionConfig(collName3, policyEnvelope, requiredPeerCount, maximumPeerCount, blockToLive)

	ccp := &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll2}}
	ccpBytes, err := proto.Marshal(ccp)
	require.NoError(t, err)

	// Test 1: no existing collection config package -> success
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1}, cdRWSet, lsccFunc, ac, chid)
	require.NoError(t, err)

	state["lscc"][privdata.BuildCollectionKVSKey(ccid)] = ccpBytes

	// Test 2: exactly same as the existing collection config package -> success
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2}, cdRWSet, lsccFunc, ac, chid)
	require.NoError(t, err)

	// Test 3: missing one existing collection (check based on the length) -> error
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "the following existing collections are missing in the new collection configuration package: [mycollection2]")

	// Test 4: missing one existing collection (check based on the collection names) -> error
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll3}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "the following existing collections are missing in the new collection configuration package: [mycollection2]")

	// Test 5: adding a new collection along with the existing collections -> success
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	require.NoError(t, err)

	newBlockToLive := blockToLive + 1
	coll2 = createCollectionConfig(collName2, policyEnvelope, requiredPeerCount, maximumPeerCount, newBlockToLive)

	// Test 6: modify the BlockToLive in an existing collection -> error
	err = testValidateCollection(t, v, []*peer.CollectionConfig{coll1, coll2, coll3}, cdRWSet, lsccFunc, ac, chid)
	require.EqualError(t, err, "the BlockToLive in the following existing collections must not be modified: [mycollection2]")
}

func TestMain(m *testing.M) {
	code := -1
	defer func() {
		os.Exit(code)
	}()
	testDir, err := ioutil.TempDir("", "v1.2-validation")
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
		fmt.Printf("Initialize cryptoProvider bccsp failed: %s", cryptoProvider)
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
	mspMgr := mspmgmt.GetManagerForChain(chainId)
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

func TestInValidCollectionName(t *testing.T) {
	validNames := []string{"collection1", "collection_2"}
	inValidNames := []string{"collection.1", "collection%2", ""}

	for _, name := range validNames {
		require.NoError(t, validateCollectionName(name), "Testing for name = "+name)
	}
	for _, name := range inValidNames {
		require.Error(t, validateCollectionName(name), "Testing for name = "+name)
	}
}
