/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	mc "github.com/hyperledger/fabric/common/mocks/config"
	mocklgr "github.com/hyperledger/fabric/common/mocks/ledger"
	mockpeer "github.com/hyperledger/fabric/common/mocks/peer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	cmp "github.com/hyperledger/fabric/core/mocks/peer"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/scc"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	plgr "github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"golang.org/x/net/context"
)

var globalBlockNum map[string]uint64

type mockResultsIterator struct {
	current int
	kvs     []*plgr.KV
}

func (mri *mockResultsIterator) Next() (commonledger.QueryResult, error) {
	if mri.current == len(mri.kvs) {
		return nil, nil
	}
	kv := mri.kvs[mri.current]
	mri.current = mri.current + 1

	return kv, nil
}

func (mri *mockResultsIterator) Close() {
	mri.current = len(mri.kvs)
}

type mockExecQuerySimulator struct {
	txsim ledger.TxSimulator
	mocklgr.MockQueryExecutor
	resultsIter map[string]map[string]*mockResultsIterator
}

func (meqe *mockExecQuerySimulator) GetHistoryForKey(namespace, query string) (commonledger.ResultsIterator, error) {
	return meqe.commonQuery(namespace, query)
}

func (meqe *mockExecQuerySimulator) ExecuteQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	return meqe.commonQuery(namespace, query)
}

func (meqe *mockExecQuerySimulator) commonQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	if meqe.resultsIter == nil {
		return nil, fmt.Errorf("query executor not initialized")
	}
	nsiter := meqe.resultsIter[namespace]
	if nsiter == nil {
		return nil, fmt.Errorf("namespace %v not found for %s", namespace, query)
	}
	iter := nsiter[query]
	if iter == nil {
		fmt.Printf("iter not found for query %s\n", query)
	}
	return iter, nil
}

func (meqe *mockExecQuerySimulator) SetState(namespace string, key string, value []byte) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.SetState(namespace, key, value)
}

func (meqe *mockExecQuerySimulator) DeleteState(namespace string, key string) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.DeleteState(namespace, key)
}

func (meqe *mockExecQuerySimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.SetStateMultipleKeys(namespace, kvs)
}

func (meqe *mockExecQuerySimulator) ExecuteUpdate(query string) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.ExecuteUpdate(query)
}

func (meqe *mockExecQuerySimulator) GetTxSimulationResults() ([]byte, error) {
	if meqe.txsim == nil {
		return nil, fmt.Errorf("SetState txsimulator not initialed")
	}
	simRes, err := meqe.txsim.GetTxSimulationResults()
	if err != nil {
		return nil, err
	}
	return simRes.GetPubSimulationBytes()
}

var mockAclProvider *mocks.MockACLProvider

//initialize peer and start up. If security==enabled, login as vp
func initMockPeer(chainIDs ...string) error {
	msi := &cmp.MockSupportImpl{
		GetApplicationConfigRv:     &mc.MockApplication{&mc.MockApplicationCapabilities{}},
		GetApplicationConfigBoolRv: true,
	}
	peer.RegisterSupportFactory(&cmp.MockSupportFactoryImpl{msi})

	mockAclProvider = &mocks.MockACLProvider{}
	mockAclProvider.Reset()

	aclmgmt.RegisterACLProvider(mockAclProvider)

	peer.MockInitialize()

	mspGetter := func(cid string) []string {
		return []string{"DEFAULT"}
	}

	peer.MockSetMSPIDGetter(mspGetter)

	ccStartupTimeout := time.Duration(10) * time.Second
	ca, _ := accesscontrol.NewCA()
	NewChaincodeSupport("0.0.0.0:7052", false, ccStartupTimeout, ca)
	theChaincodeSupport.executetimeout = time.Duration(1) * time.Second

	// Mock policy checker
	policy.RegisterPolicyCheckerFactory(&mockPolicyCheckerFactory{})

	scc.RegisterSysCCs()

	globalBlockNum = make(map[string]uint64, len(chainIDs))
	for _, id := range chainIDs {
		scc.DeDeploySysCCs(id)
		if err := peer.MockCreateChain(id); err != nil {
			return err
		}
		scc.DeploySysCCs(id)
		// any chain other than the default testchainid does not have a MSP set up -> create one
		if id != util.GetTestChainID() {
			mspmgmt.XXXSetMSPManager(id, mspmgmt.GetManagerForChain(util.GetTestChainID()))
		}
		globalBlockNum[id] = 1
	}

	return nil
}

func finitMockPeer(chainIDs ...string) {
	for _, c := range chainIDs {
		scc.DeDeploySysCCs(c)
		if lgr := peer.GetLedger(c); lgr != nil {
			lgr.Close()
		}
	}
	ledgermgmt.CleanupTestEnv()
	ledgerPath := config.GetPath("peer.fileSystemPath")
	os.RemoveAll(ledgerPath)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))
}

//store the stream CC mappings here
var mockPeerCCSupport = mockpeer.NewMockPeerSupport()

func mockChaincodeStreamGetter(name string) (shim.PeerChaincodeStream, error) {
	return mockPeerCCSupport.GetCC(name)
}

type mockCCLauncher struct {
	execTime *time.Duration
	resp     container.VMCResp
	retErr   error
	notfyb   bool
}

func (ccl *mockCCLauncher) launch(ctxt context.Context, notfy chan bool) (interface{}, error) {
	if ccl.execTime != nil {
		time.Sleep(*ccl.execTime)
	}

	//no error on launch, notify
	if ccl.resp.Err == nil {
		notfy <- ccl.notfyb
	}

	return ccl.resp, ccl.retErr
}

func setupcc(name string) (*mockpeer.MockCCComm, *mockpeer.MockCCComm) {
	send := make(chan *pb.ChaincodeMessage)
	recv := make(chan *pb.ChaincodeMessage)
	peerSide, _ := mockPeerCCSupport.AddCC(name, recv, send)
	peerSide.SetName("peer")
	ccSide := mockPeerCCSupport.GetCCMirror(name)
	ccSide.SetPong(true)
	return peerSide, ccSide
}

//assign this to done and failNow and keep using them
func setuperror() chan error {
	return make(chan error)
}

func processDone(t *testing.T, done chan error, expecterr bool) {
	var err error
	if done != nil {
		err = <-done
	}
	if expecterr != (err != nil) {
		if err == nil {
			t.Fatalf("Expected error but got success")
		} else {
			t.Fatalf("Expected success but got error %s", err)
		}
	}
}

func startTx(t *testing.T, chainID string, cis *pb.ChaincodeInvocationSpec, txId string) (context.Context, ledger.TxSimulator, *pb.SignedProposal, *pb.Proposal) {
	ctxt := context.Background()

	creator := []byte([]byte("Alice"))
	sprop, prop := putils.MockSignedEndorserProposalOrPanic(chainID, cis.ChaincodeSpec, creator, []byte("msg1"))
	var txsim ledger.TxSimulator
	var err error
	if ctxt, txsim, err = startTxSimulation(ctxt, chainID, txId); err != nil {
		t.Fatalf("getting txsimulator failed %s", err)
	}
	return ctxt, txsim, sprop, prop
}

func endTx(t *testing.T, cccid *ccprovider.CCContext, txsim ledger.TxSimulator, cis *pb.ChaincodeInvocationSpec) {
	if err := endTxSimulationCIS(cccid.ChainID, cis.ChaincodeSpec.ChaincodeId, cccid.TxID, txsim, []byte("invoke"), true, cis, globalBlockNum[cccid.ChainID]); err != nil {
		t.Fatalf("simulation failed with error %s", err)
	}
	globalBlockNum[cccid.ChainID] = globalBlockNum[cccid.ChainID] + 1
}

func execCC(t *testing.T, ctxt context.Context, ccSide *mockpeer.MockCCComm, cccid *ccprovider.CCContext, waitForERROR bool, expectExecErr bool, done chan error, cis *pb.ChaincodeInvocationSpec, respSet *mockpeer.MockResponseSet) error {
	ccSide.SetResponses(respSet)

	_, _, err := ExecuteWithErrorFilter(ctxt, cccid, cis)

	if err == nil && expectExecErr {
		t.Fatalf("expected error but succeeded")
	} else if err != nil && !expectExecErr {
		t.Fatalf("exec failed with %s", err)
	}

	//wait
	processDone(t, done, waitForERROR)

	return nil
}

//initialize cc support env and startup the chaincode
func startCC(t *testing.T, channelID string, ccname string) (*mockpeer.MockCCComm, *mockpeer.MockCCComm) {
	peerSide, ccSide := setupcc(ccname)
	defer mockPeerCCSupport.RemoveCC(ccname)
	theChaincodeSupport.userRunsCC = true
	flogging.SetModuleLevel("chaincode", "debug")
	//register peer side with ccsupport
	go func() {
		theChaincodeSupport.HandleChaincodeStream(context.Background(), peerSide)
	}()

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	//start the mock peer
	go func() {
		respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
			{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}, nil},
			{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY}, nil}}}
		ccSide.SetResponses(respSet)
		ccSide.Run()
	}()

	ccSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER, Payload: putils.MarshalOrPanic(&pb.ChaincodeID{Name: ccname + ":0"}), Txid: "0", ChannelId: channelID})

	//wait for init
	processDone(t, done, false)

	return peerSide, ccSide
}

func getTarGZ(t *testing.T, name string, contents []byte) []byte {
	startTime := time.Now()
	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tr := tar.NewWriter(gw)
	size := int64(len(contents))

	tr.WriteHeader(&tar.Header{Name: name, Size: size, ModTime: startTime, AccessTime: startTime, ChangeTime: startTime})
	tr.Write(contents)
	tr.Close()
	gw.Close()
	ioutil.WriteFile("/tmp/t.gz", inputbuf.Bytes(), 0644)
	return inputbuf.Bytes()
}

// Deploy a chaincode - i.e., build and initialize.
func deployCC(t *testing.T, ctx context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeSpec) {
	// First build and get the deployment spec
	code := getTarGZ(t, "src/dummy/dummy.go", []byte("code"))
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: code}

	//ignore existence errors
	ccprovider.PutChaincodeIntoFS(cds)

	b := putils.MarshalOrPanic(cds)

	sysCCVers := util.GetSysCCVersion()

	//wrap the deployment in an invocation spec to lscc...
	lsccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: "lscc", Version: sysCCVers}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte(cccid.ChainID), b}}}}

	sprop, prop := putils.MockSignedEndorserProposal2OrPanic(cccid.ChainID, lsccSpec.ChaincodeSpec, signer)
	lsccid := ccprovider.NewCCContext(cccid.ChainID, lsccSpec.ChaincodeSpec.ChaincodeId.Name, sysCCVers, cccid.TxID, true, sprop, prop)

	//write to lscc
	if _, _, err := ExecuteWithErrorFilter(ctx, lsccid, lsccSpec); err != nil {
		t.Fatalf("Error deploying chaincode %v (err: %s)", cccid, err)
	}
}

func initializeCC(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("A"), []byte("100"), []byte("B"), []byte("200")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	//setup CheckACL calls
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	//bad txid in response (should be "1"), should fail
	resp := &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("init succeeded")}), Txid: "unknowntxid", ChannelId: chainID}}
	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{resp}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	//set the right TxID in response now
	resp.RespMsg.(*pb.ChaincodeMessage).Txid = txid

	badcccid := ccprovider.NewCCContext(chainID, ccname, "unknownver", txid, false, sprop, prop)

	//we are not going to reach the chaincode and so won't get a response from it. processDone will not
	//be triggered by the chaincode stream.  We just expect an error from fabric. Hence pass nil for done
	execCC(t, ctxt, ccSide, badcccid, false, true, nil, cis, respSet)

	//---------try a successful init at last-------
	//everything lined up
	//    correct registered chaincode version
	//    matching txid
	//    txsim context
	//    full response
	//    correct block number for ending sim

	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "", Key: "A", Value: []byte("100")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "", Key: "B", Value: []byte("200")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "c1", Key: "C", Value: []byte("300")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "c2", Key: "C", Value: []byte("300")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), ChaincodeEvent: &pb.ChaincodeEvent{ChaincodeId: ccname}, Txid: txid, ChannelId: chainID}}}}

	cccid.Version = "1"
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func invokeCC(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	//setup CheckACL calls
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "", Key: "A"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "", Key: "B"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "", Key: "A", Value: []byte("90")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "", Key: "B", Value: []byte("210")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "", Key: "TODEL", Value: []byte("-to-be-deleted-")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	//delete the extra var
	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "", Key: "TODEL"}), Txid: "3", ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: putils.MarshalOrPanic(&pb.DelState{Collection: "", Key: "TODEL"}), Txid: "3", ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "3", ChannelId: chainID}}}}

	cccid.TxID = "3"
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	//get the extra var and delete it
	//NOTE- we are calling ExecuteWithErrorFilter which returns error if chaincode returns ERROR response
	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "", Key: "TODEL"}), Txid: "4", ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "variable not found"}), Txid: "4", ChannelId: chainID}}}}

	cccid.TxID = "4"
	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func invokePrivateDataGetPutDelCC(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invokePrivateData")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "", Key: "C"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "variable not found"}), Txid: txid, ChannelId: chainID}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "c1", Key: "C"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "c1", Key: "C", Value: []byte("310")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "c1", Key: "A", Value: []byte("100")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutState{Collection: "c1", Key: "B", Value: []byte("100")}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: putils.MarshalOrPanic(&pb.DelState{Collection: "c2", Key: "C"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}}}}

	cccid = ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	txid = util.GenerateUUID()
	ctxt, txsim, sprop, prop = startTx(t, chainID, cis, txid)

	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: putils.MarshalOrPanic(&pb.GetState{Collection: "c2", Key: "C"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "variable not found"}), Txid: txid, ChannelId: chainID}}}}

	cccid = ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func getQueryStateByRange(t *testing.T, collection, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	//setup CheckACL calls
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	//create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: putils.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: putils.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}

	var mkpeer []*mockpeer.MockResponse

	mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Payload: putils.MarshalOrPanic(&pb.GetStateByRange{Collection: collection, StartKey: "A", EndKey: "B"}), Txid: txid, ChannelId: chainID}})

	if collection == "" {
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc})
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, queryStateCloseFunc})
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}})
	} else {
		// Range queries on private data is not yet implemented.
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "Not Yet Supported"}), Txid: txid, ChannelId: chainID}})
	}

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, mkpeer}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	if collection == "" {
		execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)
	} else {
		execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)
	}

	endTx(t, cccid, txsim, cis)

	return nil
}

func cc2cc(t *testing.T, chainID, chainID2, ccname string, ccSide *mockpeer.MockCCComm) error {
	calledCC := "calledCC"
	//starts and registers the CC
	_, calledCCSide := startCC(t, chainID2, calledCC)
	if calledCCSide == nil {
		t.Fatalf("start up failed for called CC")
	}
	defer calledCCSide.Quit()

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: calledCC, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("deploycc")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	//first deploy the new cc to LSCC
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	//setup CheckACL calls
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.CC2CC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	cccid := ccprovider.NewCCContext(chainID, calledCC, "0", txid, false, sprop, prop)

	deployCC(t, ctxt, cccid, cis.ChaincodeSpec)

	//commit
	endTx(t, cccid, txsim, cis)

	//now do the cc2cc
	chaincodeID = &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invokecc")}, Decorations: nil}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid = util.GenerateUUID()
	ctxt, txsim, sprop, prop = startTx(t, chainID, cis, txid)

	//we want hooks for both chaincodes
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.CC2CC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.CC2CC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	sysCCVers := util.GetSysCCVersion()
	//call a callable system CC, a regular cc, a regular but different cc on a different chain, a regular but same cc on a different chain,  and an uncallable system cc and expect an error inthe last one
	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "lscc:" + sysCCVers}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID2}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "vscc:" + sysCCVers}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}}}}

	respSet2 := &mockpeer.MockResponseSet{nil, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}}}}
	calledCCSide.SetResponses(respSet2)

	cccid = ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)

	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	//finally lets try a Bad ACL with CC-calling-CC
	chaincodeID = &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invokecc")}, Decorations: nil}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid = util.GenerateUUID()
	ctxt, txsim, sprop, prop = startTx(t, chainID, cis, txid)

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.CC2CC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.CC2CC, chainID, sprop).Return(errors.New("Bad ACL calling CC"))
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)
	//call regular cc but without ACL on called CC
	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}}}}

	respSet2 = &mockpeer.MockResponseSet{nil, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}}}}

	calledCCSide.SetResponses(respSet2)

	cccid = ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)

	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func getQueryResult(t *testing.T, collection, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	kvs := make([]*plgr.KV, 1000)
	for i := 0; i < 1000; i++ {
		kvs[i] = &plgr.KV{Namespace: chainID, Key: fmt.Sprintf("%d", i), Value: []byte(fmt.Sprintf("%d", i))}
	}

	queryExec := &mockExecQuerySimulator{resultsIter: make(map[string]map[string]*mockResultsIterator)}
	queryExec.resultsIter[ccname] = map[string]*mockResultsIterator{"goodquery": {kvs: kvs}}

	queryExec.txsim = ctxt.Value(TXSimulatorKey).(ledger.TxSimulator)
	ctxt = context.WithValue(ctxt, TXSimulatorKey, queryExec)

	//create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: putils.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: putils.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}

	var mkpeer []*mockpeer.MockResponse

	mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Payload: putils.MarshalOrPanic(&pb.GetQueryResult{Collection: "", Query: "goodquery"}), Txid: txid, ChannelId: chainID}})

	if collection == "" {
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc})
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, queryStateCloseFunc})
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}})
	} else {
		// Get query results on private data is not yet implemented.
		mkpeer = append(mkpeer, &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "Not Yet Supported"}), Txid: txid, ChannelId: chainID}})
	}

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, mkpeer}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	if collection == "" {
		execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)
	} else {
		execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)
	}

	endTx(t, cccid, txsim, cis)

	return nil
}

func getHistory(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis, txid)

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	kvs := make([]*plgr.KV, 1000)
	for i := 0; i < 1000; i++ {
		kvs[i] = &plgr.KV{Namespace: chainID, Key: fmt.Sprintf("%d", i), Value: []byte(fmt.Sprintf("%d", i))}
	}

	queryExec := &mockExecQuerySimulator{resultsIter: make(map[string]map[string]*mockResultsIterator)}
	queryExec.resultsIter[ccname] = map[string]*mockResultsIterator{"goodquery": {kvs: kvs}}

	queryExec.txsim = ctxt.Value(TXSimulatorKey).(ledger.TxSimulator)
	ctxt = context.WithValue(ctxt, TXSimulatorKey, queryExec)

	//create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: putils.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: txid}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: putils.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: txid}
	}

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Payload: putils.MarshalOrPanic(&pb.GetQueryResult{Query: "goodquery"}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, queryStateCloseFunc},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func getLaunchConfigs(t *testing.T, auth accesscontrol.Authenticator) {
	newCCSupport := &ChaincodeSupport{peerTLS: true, chaincodeLogLevel: "debug", shimLogLevel: "info"}

	//set the authenticator for generating TLS stuff
	newCCSupport.auth = auth

	ccContext := ccprovider.NewCCContext("dummyChannelId", "mycc", "v0", "dummyTxid", false, nil, nil)
	args, envs, filesToUpload, err := newCCSupport.getLaunchConfigs(ccContext, pb.ChaincodeSpec_GOLANG)
	if err != nil {
		t.Fatalf("calling getLaunchConfigs() failed with error %s", err)
	}

	if len(args) != 2 {
		t.Fatalf("calling getLaunchConfigs() for golang chaincode should have returned an array of 2 elements for Args, but got %v", args)
	}
	if args[0] != "chaincode" || !strings.HasPrefix(args[1], "-peer.address") {
		t.Fatalf("calling getLaunchConfigs() should have returned the start command for golang chaincode, but got %v", args)
	}
	if len(envs) != 7 {
		t.Fatalf("calling getLaunchConfigs() with TLS enabled should have returned an array of 7 elements for Envs, but got %v", envs)
	}
	if envs[0] != "CORE_CHAINCODE_ID_NAME=mycc:v0" || envs[1] != "CORE_PEER_TLS_ENABLED=true" ||
		envs[2] != "CORE_TLS_CLIENT_KEY_PATH=/etc/hyperledger/fabric/client.key" || envs[3] != "CORE_TLS_CLIENT_CERT_PATH=/etc/hyperledger/fabric/client.crt" ||
		envs[4] != "CORE_PEER_TLS_ROOTCERT_FILE=/etc/hyperledger/fabric/peer.crt" ||
		envs[5] != "CORE_CHAINCODE_LOGGING_LEVEL=debug" || envs[6] != "CORE_CHAINCODE_LOGGING_SHIM=info" {
		t.Fatalf("calling getLaunchConfigs() with TLS enabled should have returned the proper environment variables, but got %v", envs)
	}
	if len(filesToUpload) != 3 {
		t.Fatalf("calling getLaunchConfigs() with TLS enabled should have returned an array of 3 elements for filesToUpload, but got %v", len(filesToUpload))
	}

	args, envs, _, err = newCCSupport.getLaunchConfigs(ccContext, pb.ChaincodeSpec_NODE)
	if len(args) != 3 {
		t.Fatalf("calling getLaunchConfigs() for node chaincode should have returned an array of 3 elements for Args, but got %v", args)
	}

	if args[0] != "/bin/sh" || args[1] != "-c" || !strings.HasPrefix(args[2], "cd /usr/local/src; npm start -- --peer.address") {
		t.Fatalf("calling getLaunchConfigs() should have returned the start command for node.js chaincode, but got %v", args)
	}

	newCCSupport.peerTLS = false
	args, envs, _, err = newCCSupport.getLaunchConfigs(ccContext, pb.ChaincodeSpec_GOLANG)
	if len(envs) != 4 {
		t.Fatalf("calling getLaunchConfigs() with TLS disabled should have returned an array of 4 elements for Envs, but got %v", envs)
	}
	if envs[0] != "CORE_CHAINCODE_ID_NAME=mycc:v0" || envs[1] != "CORE_PEER_TLS_ENABLED=false" ||
		envs[2] != "CORE_CHAINCODE_LOGGING_LEVEL=debug" || envs[3] != "CORE_CHAINCODE_LOGGING_SHIM=info" {
		t.Fatalf("calling getLaunchConfigs() with TLS disabled should have returned the proper environment variables, but got %v", envs)
	}
}

//success case
func TestLaunchAndWaitSuccess(t *testing.T) {
	newCCSupport := &ChaincodeSupport{peerTLS: false, chaincodeLogLevel: "debug", shimLogLevel: "info", ccStartupTimeout: time.Duration(10) * time.Second, runningChaincodes: &runningChaincodes{chaincodeMap: make(map[string]*chaincodeRTEnv), launchStarted: make(map[string]bool)}, peerNetworkID: "networkID", peerID: "peerID"}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}}
	code := getTarGZ(t, "src/dummy/dummy.go", []byte("code"))
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: code}
	cccid := ccprovider.NewCCContext("testchannel", "testcc", "0", "landwtimertest_txid", false, nil, nil)

	//actual test - everythings good
	launcher := &mockCCLauncher{execTime: nil, resp: container.VMCResp{}, retErr: nil, notfyb: true}
	err := newCCSupport.launchAndWaitForRegister(context.Background(), cccid, cds, launcher)
	if err != nil {
		t.Fatalf("expected success but failed with error %s", err)
	}
}

//test timeout error
func TestLaunchAndWaitTimeout(t *testing.T) {
	newCCSupport := &ChaincodeSupport{peerTLS: false, chaincodeLogLevel: "debug", shimLogLevel: "info", ccStartupTimeout: time.Duration(500) * time.Millisecond, runningChaincodes: &runningChaincodes{chaincodeMap: make(map[string]*chaincodeRTEnv), launchStarted: make(map[string]bool)}, peerNetworkID: "networkID", peerID: "peerID"}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}}
	code := getTarGZ(t, "src/dummy/dummy.go", []byte("code"))
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: code}
	cccid := ccprovider.NewCCContext("testchannel", "testcc", "0", "landwtimertest_txid", false, nil, nil)

	//the actual test - timeout 1000 > 500
	sleepTime := time.Duration(1) * time.Second
	launcher := &mockCCLauncher{execTime: &sleepTime, resp: container.VMCResp{}, retErr: nil, notfyb: true}
	err := newCCSupport.launchAndWaitForRegister(context.Background(), cccid, cds, launcher)
	if err == nil {
		t.Fatalf("expected error but succeeded")
	}
}

//test notification error case
func TestLaunchAndWaitNotificationError(t *testing.T) {
	newCCSupport := &ChaincodeSupport{peerTLS: false, chaincodeLogLevel: "debug", shimLogLevel: "info", ccStartupTimeout: time.Duration(10) * time.Second, runningChaincodes: &runningChaincodes{chaincodeMap: make(map[string]*chaincodeRTEnv), launchStarted: make(map[string]bool)}, peerNetworkID: "networkID", peerID: "peerID"}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}}
	code := getTarGZ(t, "src/dummy/dummy.go", []byte("code"))
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: code}
	cccid := ccprovider.NewCCContext("testchannel", "testcc", "0", "landwtimertest_txid", false, nil, nil)

	//the actual test - notify false
	launcher := &mockCCLauncher{execTime: nil, resp: container.VMCResp{}, retErr: nil, notfyb: false}
	err := newCCSupport.launchAndWaitForRegister(context.Background(), cccid, cds, launcher)
	if err == nil {
		t.Fatalf("expected error but succeeded")
	}
}

//test container return error
func TestLaunchAndWaitLaunchError(t *testing.T) {
	newCCSupport := &ChaincodeSupport{peerTLS: false, chaincodeLogLevel: "debug", shimLogLevel: "info", ccStartupTimeout: time.Duration(10) * time.Second, runningChaincodes: &runningChaincodes{chaincodeMap: make(map[string]*chaincodeRTEnv), launchStarted: make(map[string]bool)}, peerNetworkID: "networkID", peerID: "peerID"}
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}}
	code := getTarGZ(t, "src/dummy/dummy.go", []byte("code"))
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: code}
	cccid := ccprovider.NewCCContext("testchannel", "testcc", "0", "landwtimertest_txid", false, nil, nil)

	//actual test - container launch gives error
	launcher := &mockCCLauncher{execTime: nil, resp: container.VMCResp{Err: errors.New("Bad launch")}, retErr: nil}
	err := newCCSupport.launchAndWaitForRegister(context.Background(), cccid, cds, launcher)
	if err == nil {
		t.Fatalf("expected error but succeeded")
	}
}

func TestGetTxContextFromHandler(t *testing.T) {
	h := Handler{txCtxs: map[string]*transactionContext{}}

	chnl := "test"
	txid := "1"
	// test getTxContext for TEST channel, tx=1, msgType=IVNOKE_CHAINCODE and empty payload - empty payload => expect to return empty txContext
	txContext, _ := h.getTxContextForMessage(chnl, "1", pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), []byte(""), "[%s]No ledger context for %s. Sending %s", 12345, "TestCC", pb.ChaincodeMessage_ERROR)
	if txContext != nil {
		t.Fatalf("expected empty txContext for empty payload")
	}

	// mock a peer ldger for our channel
	peer.MockInitialize()

	err := peer.MockCreateChain(chnl)
	if err != nil {
		t.Fatalf("failed to create Peer Ledger %s", err)
	}

	pldgr := peer.GetLedger(chnl)

	// prepare a payload and generate a TxContext in the handler to be used in the following getTxContextFroMessage with a normal UCC
	txCtxGenerated, payload := genNewPldAndCtxFromLdgr(t, "shimTestCC", chnl, txid, pldgr, &h)

	// test getTxContext for TEST channel, tx=1, msgType=IVNOKE_CHAINCODE and non empty payload => must return a non empty txContext
	txContext, ccMsg := h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// test for another msgType (PUT_STATE) with the same payload ==> must return a non empty txContext
	txContext, ccMsg = h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_PUT_STATE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_PUT_STATE.String(), pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and PUT_STATE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// get a new txContext for our SCC tests
	txid = "2"
	// reset channel to "" to test getting a context for an SCC without a channel
	chnl = ""
	txCtxGenerated, payload = genNewPldAndCtxFromLdgr(t, "lscc", chnl, txid, pldgr, &h)

	// test getting a TxContext with an SCC without a channel => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// now reset back to non empty channel and test with an SCC
	txid = "3"
	chnl = "TEST"
	txCtxGenerated, payload = genNewPldAndCtxFromLdgr(t, "lscc", chnl, txid, pldgr, &h)

	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// now test getting a context with an empty channel and a UCC instead of an SCC
	txid = "4"
	chnl = ""
	txCtxGenerated, payload = genNewPldAndCtxFromLdgr(t, "shimTestCC", chnl, txid, pldgr, &h)
	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// new test getting a context with an empty channel without the ledger creating a new context for a UCC
	txid = "5"
	payload = genNewPld(t, "shimTestCC")
	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), pb.ChaincodeMessage_ERROR)
	if txContext != nil || ccMsg == nil {
		t.Fatal("expected nil txContext for non empty payload and INVOKE_CHAINCODE msgType without the ledger generating a TxContext . unexpected non nil tcContext")
	}

	// test same scenario as above but for an SCC this time
	txid = "6"
	payload = genNewPld(t, "lscc")
	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForMessage(chnl, txid, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), pb.ChaincodeMessage_ERROR)
	if txContext != nil || ccMsg == nil {
		t.Fatal("expected nil txContext for non empty payload and INVOKE_CHAINCODE msgType without the ledger generating a TxContext . unexpected non nil tcContext")
	}
}

func genNewPldAndCtxFromLdgr(t *testing.T, ccName string, chnl string, txid string, pldgr ledger.PeerLedger, h *Handler) (*transactionContext, []byte) {
	// create a new TxSimulator for the received txid
	txsim, err := pldgr.NewTxSimulator(txid)
	if err != nil {
		t.Fatalf("failed to create TxSimulator %s", err)
	}
	// get a context for this txsim
	ctxt := context.WithValue(context.Background(), TXSimulatorKey, txsim)
	// create a new txContext in the handler to be retrieved by the tested function (ie: getTxContextForMessage)
	newTxCtxt, err := h.createTxContext(ctxt, chnl, txid, nil, nil)
	if err != nil {
		t.Fatalf("Error creating TxContext by the handler for cc %s and channel '%s': %s", ccName, chnl, err)
	}
	if newTxCtxt == nil {
		t.Fatalf("Error creating TxContext: newTxCtxt created by the handler is nil for cc %s and channel '%s'.", ccName, chnl)
	}
	// build a new cds and payload for the CC called ccName
	payload := genNewPld(t, ccName)
	return newTxCtxt, payload
}

func genNewPld(t *testing.T, ccName string) []byte {
	// build a new cds and payload for the CC called ccName
	chaincodeID := &pb.ChaincodeID{Name: ccName, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("deploycc")}, Decorations: nil}
	cds := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}
	payload, err := proto.Marshal(cds)
	if err != nil {
		t.Fatalf("failed to marshal CDS %s", err)
	}
	return payload
}

func cc2SameCC(t *testing.T, chainID, chainID2, ccname string, ccSide *mockpeer.MockCCComm) {
	//first deploy the CC on chainID2
	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("deploycc")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	txid := util.GenerateUUID()
	ctxt, txsim, sprop, prop := startTx(t, chainID2, cis, txid)

	//setup CheckACL calls
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.CC2CC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID2, sprop).Return(nil)

	cccid := ccprovider.NewCCContext(chainID2, ccname, "0", txid, false, sprop, prop)

	deployCC(t, ctxt, cccid, cis.ChaincodeSpec)

	//commit
	endTx(t, cccid, txsim, cis)

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	//now for the test - call the same cc on a different channel(should succeed), call the same cc on the same channel(should fail)
	//Note the error "Another request pending for this Txid. Cannot process." in the logs under TX "cctosamecctx"
	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid = util.GenerateUUID()
	ctxt, txsim, sprop, prop = startTx(t, chainID, cis, txid)

	mockAclProvider.On("CheckACL", resources.CC2CC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID, sprop).Return(nil)

	mockAclProvider.On("CheckACL", resources.CC2CC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETDEPSPEC, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.LSCC_GETCCDATA, chainID2, sprop).Return(nil)
	mockAclProvider.On("CheckACL", resources.PROPOSE, chainID2, sprop).Return(nil)

	txid = "cctosamecctx"
	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: ccname + ":0/" + chainID2}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: ccname + ":0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}}}}

	cccid = ccprovider.NewCCContext(chainID, ccname, "0", txid, false, sprop, prop)

	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)
}

func TestCCFramework(t *testing.T) {
	//register 2 channels
	chainID := "mockchainid"
	chainID2 := "secondchain"
	if err := initMockPeer(chainID, chainID2); err != nil {
		t.Fatalf("%s", err)
	}
	defer finitMockPeer(chainID, chainID2)

	//create a chaincode
	ccname := "shimTestCC"

	//starts and registers the CC
	_, ccSide := startCC(t, chainID, ccname)
	if ccSide == nil {
		t.Fatalf("start up failed")
	}

	//call's init and does some PUT (after doing some negative testing)
	initializeCC(t, chainID, ccname, ccSide)

	//chaincode support should not allow dups
	if err := theChaincodeSupport.registerHandler(&Handler{ChaincodeID: &pb.ChaincodeID{Name: ccname + ":0"}}); err == nil {
		t.Fatalf("expected re-register to fail")
	} else if err, _ := err.(*DuplicateChaincodeHandlerError); err == nil {
		t.Fatalf("expected DuplicateChaincodeHandlerError")
	}

	//call's init and does some PUT (after doing some negative testing)
	initializeCC(t, chainID2, ccname, ccSide)

	//call's invoke and do some GET
	invokeCC(t, chainID, ccname, ccSide)

	//call's invoke and do some GET/PUT/DEL on private data
	invokePrivateDataGetPutDelCC(t, chainID, ccname, ccSide)

	//call's query state range
	getQueryStateByRange(t, "", chainID, ccname, ccSide)

	//call's cc2cc on the same chaincode only call to chainID2 should succeed
	cc2SameCC(t, chainID, chainID2, ccname, ccSide)

	//call's cc2cc (variation with syscc calls)
	cc2cc(t, chainID, chainID2, ccname, ccSide)

	//call's query result
	getQueryResult(t, "", chainID, ccname, ccSide)

	//call's history result
	getHistory(t, chainID, ccname, ccSide)

	//just use the previous authhandler for generating TLS key/pair
	getLaunchConfigs(t, theChaincodeSupport.auth)

	ccSide.Quit()
}
