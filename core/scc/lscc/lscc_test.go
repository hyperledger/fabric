/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/mocks/config"
	mscc "github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/mocks/scc/lscc"
	"github.com/hyperledger/fabric/core/policy"
	policymocks "github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func constructDeploymentSpec(name string, path string, version string, initArgs [][]byte, createInvalidIndex bool, createFS bool, scc *lifeCycleSysCC) (*pb.ChaincodeDeploymentSpec, error) {
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: name, Path: path, Version: version}, Input: &pb.ChaincodeInput{Args: initArgs}}

	codePackageBytes := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(codePackageBytes)
	tw := tar.NewWriter(gz)

	err := cutil.WriteBytesToPackage("src/garbage.go", []byte(name+path+version), tw)
	if err != nil {
		return nil, err
	}

	// create an invalid couchdb index definition for negative testing
	if createInvalidIndex {
		err = cutil.WriteBytesToPackage("META-INF/statedb/couchdb/indexes/badIndex.json", []byte("invalid index definition"), tw)
		if err != nil {
			return nil, err
		}
	}

	tw.Close()
	gz.Close()

	depSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes.Bytes()}

	if createFS {
		buf, err := proto.Marshal(depSpec)
		if err != nil {
			return nil, err
		}
		cccdspack := &ccprovider.CDSPackage{}
		if _, err := cccdspack.InitFromBuffer(buf); err != nil {
			return nil, err
		}

		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageRv = cccdspack
		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageErr = nil
		scc.support.(*lscc.MockSupport).GetChaincodesFromLocalStorageRv = &pb.ChaincodeQueryResponse{Chaincodes: []*pb.ChaincodeInfo{{}}}
		scc.support.(*lscc.MockSupport).GetChaincodesFromLocalStorageErr = nil
	} else {
		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageRv = nil
		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageErr = errors.New("barf")
		scc.support.(*lscc.MockSupport).GetChaincodesFromLocalStorageRv = nil
		scc.support.(*lscc.MockSupport).GetChaincodesFromLocalStorageErr = errors.New("barf")
	}

	return depSpec, nil
}

//TestInstall tests the install function with various inputs
func TestInstall(t *testing.T) {

	// Initialize cceventmgmt Mgr
	// TODO cceventmgmt singleton should be refactored out of peer in the future. See CR 16549 for details.
	cceventmgmt.Initialize()

	scc := New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(INSTALL)}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(INSTALL)}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	testInstall(t, "example02", "0", path, false, "", "Alice", scc, stub)
	testInstall(t, "example02-2", "1.0", path, false, "", "Alice", scc, stub)
	testInstall(t, "example02.go", "0", path, false, InvalidChaincodeNameErr("example02.go").Error(), "Alice", scc, stub)
	testInstall(t, "", "0", path, false, EmptyChaincodeNameErr("").Error(), "Alice", scc, stub)
	testInstall(t, "example02", "1{}0", path, false, InvalidVersionErr("1{}0").Error(), "Alice", scc, stub)
	testInstall(t, "example02", "0", path, true, InvalidStatedbArtifactsErr("").Error(), "Alice", scc, stub)
	testInstall(t, "example02", "0", path, false, "Authorization for INSTALL has been denied", "Bob", scc, stub)
	testInstall(t, "example02-2", "1.0-alpha+001", path, false, "", "Alice", scc, stub)
	testInstall(t, "example02-2", "1.0+sha.c0ffee", path, false, "", "Alice", scc, stub)

	scc.support.(*lscc.MockSupport).PutChaincodeToLocalStorageErr = errors.New("barf")

	testInstall(t, "example02", "0", path, false, "barf", "Alice", scc, stub)

	testInstall(t, "lscc", "0", path, false, "cannot install: lscc is the name of a system chaincode", "Alice", scc, stub)
}

func testInstall(t *testing.T, ccname string, version string, path string, createInvalidIndex bool, expectedErrorMsg string, caller string, scc *lifeCycleSysCC, stub *shim.MockStub) {
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, createInvalidIndex, false, scc)
	assert.NoError(t, err)
	b := utils.MarshalOrPanic(cds)

	//constructDeploymentSpec puts the depspec on the FS. This should succeed
	args := [][]byte{[]byte(INSTALL), b}

	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte(caller), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	if expectedErrorMsg == "" {
		res := stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	} else {
		res := stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.True(t, strings.HasPrefix(string(res.Message), expectedErrorMsg), res.Message)
	}
}

func TestDeploy(t *testing.T) {
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	testDeploy(t, "example02", "0", path, false, false, true, "", nil, nil, nil)
	testDeploy(t, "example02", "1.0", path, false, false, true, "", nil, nil, nil)
	testDeploy(t, "example02", "1.0", path, false, false, false, "cannot get package for chaincode (example02:1.0)", nil, nil, nil)
	testDeploy(t, "example02", "0", path, true, false, true, EmptyChaincodeNameErr("").Error(), nil, nil, nil)
	testDeploy(t, "example02", "0", path, false, true, true, EmptyVersionErr("example02").Error(), nil, nil, nil)
	testDeploy(t, "example02.go", "0", path, false, false, true, InvalidChaincodeNameErr("example02.go").Error(), nil, nil, nil)
	testDeploy(t, "example02", "1{}0", path, false, false, true, InvalidVersionErr("1{}0").Error(), nil, nil, nil)
	testDeploy(t, "example02", "0", path, true, true, true, EmptyChaincodeNameErr("").Error(), nil, nil, nil)

	scc := New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(DEPLOY)}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(DEPLOY), []byte(""), []byte("")}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(DEPLOY), []byte("chain"), []byte("barf")}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	testDeploy(t, "example02", "1.0", path, false, false, true, "", scc, stub, nil)
	testDeploy(t, "example02", "1.0", path, false, false, true, "chaincode exists example02", scc, stub, nil)

	scc = New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyErr = errors.New("barf")

	testDeploy(t, "example02", "1.0", path, false, false, true, "barf", scc, stub, nil)

	scc = New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).CheckInstantiationPolicyErr = errors.New("barf")

	testDeploy(t, "example02", "1.0", path, false, false, true, "barf", scc, stub, nil)

	scc = New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	// As the PrivateChannelData is disabled, the following error message is expected due to the presence of
	// collectionConfigBytes in the stub.args
	errMessage := InvalidArgsLenErr(7).Error()
	testDeploy(t, "example02", "1.0", path, false, false, true, InvalidArgsLenErr(7).Error(), scc, stub, []byte("collections"))

	// Enable PrivateChannelData
	mocksccProvider := (&mscc.MocksccProviderFactory{
		ApplicationConfigBool: true,
		ApplicationConfigRv: &config.MockApplication{
			CapabilitiesRv: &config.MockApplicationCapabilities{
				PrivateChannelDataRv: true,
			},
		},
	}).NewSystemChaincodeProvider().(*mscc.MocksccProviderImpl)

	scc = New(mocksccProvider, mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	// As the PrivateChannelData is enabled and collectionConfigBytes is invalid, the following error
	// message is expected.
	errMessage = "invalid collection configuration supplied for chaincode example02:1.0"
	testDeploy(t, "example02", "1.0", path, false, false, true, errMessage, scc, stub, []byte("invalid collection"))
	// Should contain an entry for the chaincodeData only
	assert.Equal(t, 1, len(stub.State))
	_, ok := stub.State["example02"]
	assert.Equal(t, true, ok)

	collName1 := "mycollection1"
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	requiredPeerCount = 1
	maximumPeerCount = 2
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount)

	ccp := &common.CollectionConfigPackage{[]*common.CollectionConfig{coll1}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	scc = New(mocksccProvider, mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	// As the PrivateChannelData is enabled and collectionConfigBytes is valid, no error is expected
	testDeploy(t, "example02", "1.0", path, false, false, true, "", scc, stub, ccpBytes)
	// Should contain two entries: one for the chaincodeData and another for the collectionConfigBytes
	assert.Equal(t, 2, len(stub.State))
	_, ok = stub.State["example02"]
	assert.Equal(t, true, ok)
	actualccpBytes, ok := stub.State["example02~collection"]
	assert.Equal(t, true, ok)
	assert.Equal(t, ccpBytes, actualccpBytes)

	scc = New(mocksccProvider, mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	// As the PrivateChannelData is enabled and collectionConfigBytes is nil, no error is expected
	testDeploy(t, "example02", "1.0", path, false, false, true, "", scc, stub, []byte("nil"))
	// Should contain an entry for the chaincodeData only. As the collectionConfigBytes is nil, it
	// is ignored
	assert.Equal(t, 1, len(stub.State))
	_, ok = stub.State["example02"]
	assert.Equal(t, true, ok)
}

func createCollectionConfig(collectionName string, signaturePolicyEnvelope *common.SignaturePolicyEnvelope,
	requiredPeerCount int32, maximumPeerCount int32,
) *common.CollectionConfig {
	signaturePolicy := &common.CollectionPolicyConfig_SignaturePolicy{
		SignaturePolicy: signaturePolicyEnvelope,
	}
	accessPolicy := &common.CollectionPolicyConfig{
		Payload: signaturePolicy,
	}

	return &common.CollectionConfig{
		Payload: &common.CollectionConfig_StaticCollectionConfig{
			&common.StaticCollectionConfig{
				Name:              collectionName,
				MemberOrgsPolicy:  accessPolicy,
				RequiredPeerCount: requiredPeerCount,
				MaximumPeerCount:  maximumPeerCount,
			},
		},
	}
}

func testDeploy(t *testing.T, ccname string, version string, path string, forceBlankCCName bool, forceBlankVersion bool, install bool, expectedErrorMsg string, scc *lifeCycleSysCC, stub *shim.MockStub, collectionConfigBytes []byte) {
	if scc == nil {
		scc = New(NewMockProvider(), mockAclProvider)
		scc.support = &lscc.MockSupport{}
		stub = shim.NewMockStub("lscc", scc)
		res := stub.MockInit("1", nil)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	}
	stub.ChannelID = chainid

	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic(chainid, &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false, install, scc)
	assert.NoError(t, err)

	if forceBlankCCName {
		cds.ChaincodeSpec.ChaincodeId.Name = ""
	}
	if forceBlankVersion {
		cds.ChaincodeSpec.ChaincodeId.Version = ""
	}
	b := utils.MarshalOrPanic(cds)

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	var args [][]byte
	if len(collectionConfigBytes) > 0 {
		if bytes.Compare(collectionConfigBytes, []byte("nil")) == 0 {
			args = [][]byte{[]byte(DEPLOY), []byte("test"), b, nil, []byte("escc"), []byte("vscc"), nil}
		} else {
			args = [][]byte{[]byte(DEPLOY), []byte("test"), b, nil, []byte("escc"), []byte("vscc"), collectionConfigBytes}
		}
	} else {
		args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp2)

	if expectedErrorMsg == "" {
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)

		mockAclProvider.Reset()
		mockAclProvider.On("CheckACL", resources.Lscc_GetInstantiatedChaincodes, chainid, sProp).Return(nil)
		args = [][]byte{[]byte(GETCHAINCODES)}
		res = stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)

		mockAclProvider.Reset()
		mockAclProvider.On("CheckACL", resources.Lscc_ChaincodeExists, "test", sProp).Return(nil)
		args = [][]byte{[]byte(CCEXISTS), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
		res = stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)

		mockAclProvider.Reset()
		mockAclProvider.On("CheckACL", resources.Lscc_GetDeploymentSpec, "test", sProp).Return(nil)
		args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
		res = stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)
		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageErr = errors.New("barf")
		res = stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)
		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageErr = nil
		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageRv = &ccprovider.CDSPackage{}
		res = stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

		scc.support.(*lscc.MockSupport).GetChaincodeFromLocalStorageRv = nil
		mockAclProvider.Reset()
		mockAclProvider.On("CheckACL", resources.Lscc_GetChaincodeData, "test", sProp).Return(nil)
		args = [][]byte{[]byte(GETCCDATA), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
		res = stub.MockInvokeWithSignedProposal("1", args, sProp)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	} else {
		assert.Equal(t, expectedErrorMsg, string(res.Message))
	}

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(CCEXISTS)}, nil)
}

// TestUpgrade tests the upgrade function with various inputs for basic use cases
func TestUpgrade(t *testing.T) {
	path := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	testUpgrade(t, "example02", "0", "example02", "1", path, "", nil, nil, nil)
	testUpgrade(t, "example02", "0", "example02", "", path, EmptyVersionErr("example02").Error(), nil, nil, nil)
	testUpgrade(t, "example02", "0", "example02", "0", path, IdenticalVersionErr("example02").Error(), nil, nil, nil)
	testUpgrade(t, "example02", "0", "example03", "1", path, NotFoundErr("example03").Error(), nil, nil, nil)
	testUpgrade(t, "example02", "0", "example02", "1{}0", path, InvalidVersionErr("1{}0").Error(), nil, nil, nil)
	testUpgrade(t, "example02", "0", "example*02", "1{}0", path, InvalidChaincodeNameErr("example*02").Error(), nil, nil, nil)
	testUpgrade(t, "example02", "0", "", "1", path, EmptyChaincodeNameErr("").Error(), nil, nil, nil)

	scc := New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyErr = errors.New("barf")

	testUpgrade(t, "example02", "0", "example02", "1", path, "barf", scc, stub, nil)

	scc = New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	testUpgrade(t, "example02", "0", "example02", "1", path, "instantiation policy missing", scc, stub, nil)

	scc = New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyMap = map[string][]byte{}
	scc.support.(*lscc.MockSupport).CheckInstantiationPolicyMap = map[string]error{"example020": errors.New("barf")}

	testUpgrade(t, "example02", "0", "example02", "1", path, "barf", scc, stub, nil)

	scc = New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyMap = map[string][]byte{}
	scc.support.(*lscc.MockSupport).CheckInstantiationPolicyMap = map[string]error{"example021": errors.New("barf")}

	testUpgrade(t, "example02", "0", "example02", "1", path, "barf", scc, stub, nil)

	// Enable PrivateChannelData
	mocksccProvider := (&mscc.MocksccProviderFactory{
		ApplicationConfigBool: true,
		ApplicationConfigRv: &config.MockApplication{
			CapabilitiesRv: &config.MockApplicationCapabilities{
				PrivateChannelDataRv: true,
			},
		},
	}).NewSystemChaincodeProvider().(*mscc.MocksccProviderImpl)

	scc = New(mocksccProvider, mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")

	collName1 := "mycollection1"
	var signers = [][]byte{[]byte("signer0"), []byte("signer1")}
	policyEnvelope := cauthdsl.Envelope(cauthdsl.Or(cauthdsl.SignedBy(0), cauthdsl.SignedBy(1)), signers)
	var requiredPeerCount, maximumPeerCount int32
	requiredPeerCount = 1
	maximumPeerCount = 2
	coll1 := createCollectionConfig(collName1, policyEnvelope, requiredPeerCount, maximumPeerCount)

	ccp := &common.CollectionConfigPackage{[]*common.CollectionConfig{coll1}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	// As the PrivateChannelData is enabled and collectionConfigBytes is valid, no error is expected
	testUpgrade(t, "example02", "0", "example02", "1", path, "", scc, stub, ccpBytes)
	// Should contain an entry for the chaincodeData only as the V1_2Validation is disabled.
	// Only in V1_2Validation, collection upgrades are allowed. Note that V1_2Validation
	// would be replaced with CollectionUpgrade capability.
	assert.Equal(t, 1, len(stub.State))
	_, ok := stub.State["example02"]
	assert.Equal(t, true, ok)

	// Enable PrivateChannelData and V1_2Validation
	mocksccProvider = (&mscc.MocksccProviderFactory{
		ApplicationConfigBool: true,
		ApplicationConfigRv: &config.MockApplication{
			CapabilitiesRv: &config.MockApplicationCapabilities{
				PrivateChannelDataRv: true,
				CollectionUpgradeRv:  true,
			},
		},
	}).NewSystemChaincodeProvider().(*mscc.MocksccProviderImpl)

	scc = New(mocksccProvider, mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")

	// As the PrivateChannelData is enabled and collectionConfigBytes is valid, no error is expected
	testUpgrade(t, "example02", "0", "example02", "1", path, "", scc, stub, []byte("nil"))
	// Should contain an entry for the chaincodeData only as the collectionConfigBytes is nil
	assert.Equal(t, 1, len(stub.State))
	_, ok = stub.State["example02"]
	assert.Equal(t, true, ok)

	scc = New(mocksccProvider, mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub = shim.NewMockStub("lscc", scc)
	res = stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")

	// As the PrivateChannelData is enabled and collectionConfigBytes is valid, no error is expected
	testUpgrade(t, "example02", "0", "example02", "1", path, "", scc, stub, ccpBytes)
	// Should contain two entries: one for the chaincodeData and another for the collectionConfigBytes
	// as the V1_2Validation is enabled. Only in V1_2Validation, collection upgrades are allowed.
	// Note that V1_2Validation would be replaced with CollectionUpgrade capability.
	assert.Equal(t, 2, len(stub.State))
	_, ok = stub.State["example02"]
	assert.Equal(t, true, ok)
	actualccpBytes, ok := stub.State["example02~collection"]
	assert.Equal(t, true, ok)
	assert.Equal(t, ccpBytes, actualccpBytes)
}

func testUpgrade(t *testing.T, ccname string, version string, newccname string, newversion string, path string, expectedErrorMsg string, scc *lifeCycleSysCC, stub *shim.MockStub, collectionConfigBytes []byte) {
	if scc == nil {
		scc = New(NewMockProvider(), mockAclProvider)
		scc.support = &lscc.MockSupport{}
		stub = shim.NewMockStub("lscc", scc)
		res := stub.MockInit("1", nil)
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)
		scc.support.(*lscc.MockSupport).GetInstantiationPolicyRv = []byte("instantiation policy")
	}

	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false, true, scc)
	assert.NoError(t, err)
	b := utils.MarshalOrPanic(cds)

	sProp, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	saved1 := scc.support.(*lscc.MockSupport).GetInstantiationPolicyErr
	saved2 := scc.support.(*lscc.MockSupport).CheckInstantiationPolicyMap
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyErr = nil
	scc.support.(*lscc.MockSupport).CheckInstantiationPolicyMap = nil
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
	scc.support.(*lscc.MockSupport).GetInstantiationPolicyErr = saved1
	scc.support.(*lscc.MockSupport).CheckInstantiationPolicyMap = saved2

	newCds, err := constructDeploymentSpec(newccname, path, newversion, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false, true, scc)
	assert.NoError(t, err)
	newb := utils.MarshalOrPanic(newCds)

	if len(collectionConfigBytes) > 0 {
		if bytes.Compare(collectionConfigBytes, []byte("nil")) == 0 {
			args = [][]byte{[]byte(UPGRADE), []byte("test"), newb, nil, []byte("escc"), []byte("vscc"), nil}
		} else {
			args = [][]byte{[]byte(UPGRADE), []byte("test"), newb, nil, []byte("escc"), []byte("vscc"), collectionConfigBytes}
		}
	} else {
		args = [][]byte{[]byte(UPGRADE), []byte("test"), newb}
	}

	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if expectedErrorMsg == "" {
		assert.Equal(t, res.Status, int32(shim.OK), res.Message)

		cd := &ccprovider.ChaincodeData{}
		err = proto.Unmarshal(res.Payload, cd)
		assert.NoError(t, err)

		newVer := cd.Version

		expectVer := "1"
		assert.Equal(t, newVer, expectVer, fmt.Sprintf("Upgrade chaincode version error, expected %s, got %s", expectVer, newVer))

		chaincodeEvent := <-stub.ChaincodeEventsChannel
		assert.Equal(t, UPGRADE, chaincodeEvent.EventName)
		lifecycleEvent := &pb.LifecycleEvent{}
		err = proto.Unmarshal(chaincodeEvent.Payload, lifecycleEvent)
		assert.NoError(t, err)
		assert.Equal(t, newccname, lifecycleEvent.ChaincodeName)
	} else {
		assert.Equal(t, expectedErrorMsg, string(res.Message))
	}
}

func TestGETCCINFO(t *testing.T) {
	scc := New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(CCEXISTS), []byte("chain")}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Lscc_ChaincodeExists, "chain", sProp).Return(errors.New("Failed access control"))
	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(CCEXISTS), []byte("chain"), []byte("chaincode")}, sProp)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Lscc_ChaincodeExists, "chain", sProp).Return(nil)
	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(CCEXISTS), []byte("chain"), []byte("nonexistentchaincode")}, sProp)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)
}

func TestGETCHAINCODES(t *testing.T) {
	scc := New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub := shim.NewMockStub("lscc", scc)
	stub.ChannelID = "test"
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETCHAINCODES), []byte("barf")}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	sProp, _ := utils.MockSignedEndorserProposalOrPanic("test", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Lscc_GetInstantiatedChaincodes, "test", sProp).Return(errors.New("ACL Error"))
	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETCHAINCODES)}, sProp)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Lscc_GetInstantiatedChaincodes, "test", sProp).Return(nil)
	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETCHAINCODES)}, sProp)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
}

func TestGETINSTALLEDCHAINCODES(t *testing.T) {
	scc := New(NewMockProvider(), mockAclProvider)
	scc.support = &lscc.MockSupport{}
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETINSTALLEDCHAINCODES), []byte("barf")}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETINSTALLEDCHAINCODES)}, sProp)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	identityDeserializer = &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter = &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETINSTALLEDCHAINCODES)}, sProp)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	_, err := constructDeploymentSpec("ccname", "path", "version", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false, false, scc)
	assert.NoError(t, err)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETINSTALLEDCHAINCODES)}, sProp)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)

	_, err = constructDeploymentSpec("ccname", "path", "version", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false, true, scc)
	assert.NoError(t, err)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte(GETINSTALLEDCHAINCODES)}, sProp)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)
}

func TestNewLifeCycleSysCC(t *testing.T) {
	scc := New(NewMockProvider(), mockAclProvider)
	assert.NotNil(t, scc)
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	res = stub.MockInvokeWithSignedProposal("1", [][]byte{[]byte("barf")}, nil)
	assert.NotEqual(t, res.Status, int32(shim.OK), res.Message)
}

func TestGetChaincodeData(t *testing.T) {
	scc := New(NewMockProvider(), mockAclProvider)
	assert.NotNil(t, scc)
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	_, err := scc.getChaincodeData("barf", []byte("barf"))
	assert.Error(t, err)

	_, err = scc.getChaincodeData("barf", putils.MarshalOrPanic(&ccprovider.ChaincodeData{Name: "barf s'more"}))
	assert.Error(t, err)
	assert.True(t, len(err.Error()) > 0)
}

func TestExecuteInstall(t *testing.T) {
	scc := New(NewMockProvider(), mockAclProvider)
	assert.NotNil(t, scc)
	stub := shim.NewMockStub("lscc", scc)
	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), res.Message)

	err := scc.executeInstall(stub, []byte("barf"))
	assert.Error(t, err)
}

func TestErrors(t *testing.T) {
	// these errors are really hard (if
	// outright impossible without writing
	// tons of lines of mocking code) to
	// get in testing
	err1 := TXNotFoundErr("")
	assert.True(t, len(err1.Error()) > 0)

	err3 := MarshallErr("")
	assert.True(t, len(err3.Error()) > 0)
}

func TestPutChaincodeCollectionData(t *testing.T) {
	scc := new(lifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	err := scc.putChaincodeCollectionData(stub, nil, nil)
	assert.Error(t, err)

	cd := &ccprovider.ChaincodeData{Name: "foo"}

	err = scc.putChaincodeCollectionData(stub, cd, nil)
	assert.NoError(t, err)

	cc := &common.CollectionConfig{Payload: &common.CollectionConfig_StaticCollectionConfig{&common.StaticCollectionConfig{Name: "mycollection"}}}
	ccp := &common.CollectionConfigPackage{[]*common.CollectionConfig{cc}}
	ccpBytes, err := proto.Marshal(ccp)
	assert.NoError(t, err)
	assert.NotNil(t, ccpBytes)

	stub.MockTransactionStart("foo")
	err = scc.putChaincodeCollectionData(stub, cd, []byte("barf"))
	assert.Error(t, err)
	stub.MockTransactionEnd("foo")

	stub.MockTransactionStart("foo")
	err = scc.putChaincodeCollectionData(stub, cd, ccpBytes)
	assert.NoError(t, err)
	stub.MockTransactionEnd("foo")
}

var id msp.SigningIdentity
var chainid string = util.GetTestChainID()
var mockAclProvider *mocks.MockACLProvider

func NewMockProvider() *mscc.MocksccProviderImpl {
	return (&mscc.MocksccProviderFactory{
		ApplicationConfigBool: true,
		ApplicationConfigRv: &config.MockApplication{
			CapabilitiesRv: &config.MockApplicationCapabilities{},
		},
	}).NewSystemChaincodeProvider().(*mscc.MocksccProviderImpl)
}

func TestMain(m *testing.M) {
	var err error
	msptesttools.LoadMSPSetupForTesting()
	id, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("GetSigningIdentity failed with err %s", err)
		os.Exit(-1)
	}

	mockAclProvider = &mocks.MockACLProvider{}
	mockAclProvider.Reset()

	os.Exit(m.Run())
}
