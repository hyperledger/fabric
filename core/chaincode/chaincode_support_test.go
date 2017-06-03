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

package chaincode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	mocklgr "github.com/hyperledger/fabric/common/mocks/ledger"
	mockpeer "github.com/hyperledger/fabric/common/mocks/peer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/scc"
	plgr "github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"golang.org/x/net/context"

	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
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
	return meqe.txsim.GetTxSimulationResults()
}

//initialize peer and start up. If security==enabled, login as vp
func initMockPeer(chainIDs ...string) error {
	peer.MockInitialize()

	mspGetter := func(cid string) []string {
		return []string{"DEFAULT"}
	}

	peer.MockSetMSPIDGetter(mspGetter)

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{Id: &pb.PeerID{Name: "testpeer"}}, nil
	}

	ccStartupTimeout := time.Duration(2) * time.Second
	NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout)
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

func startTx(t *testing.T, chainID string, cis *pb.ChaincodeInvocationSpec) (context.Context, ledger.TxSimulator, *pb.SignedProposal, *pb.Proposal) {
	ctxt := context.Background()

	creator := []byte([]byte("Alice"))
	sprop, prop := putils.MockSignedEndorserProposalOrPanic(chainID, cis.ChaincodeSpec, creator, []byte("msg1"))
	var txsim ledger.TxSimulator
	var err error
	if ctxt, txsim, err = startTxSimulation(ctxt, chainID); err != nil {
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
func startCC(t *testing.T, ccname string) (*mockpeer.MockCCComm, *mockpeer.MockCCComm) {
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
			&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}, nil},
			&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY}, nil}}}
		ccSide.SetResponses(respSet)
		ccSide.Run()
	}()

	ccSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER, Payload: putils.MarshalOrPanic(&pb.ChaincodeID{Name: ccname + ":0"}), Txid: "0"})

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
	ci := &pb.ChaincodeInput{[][]byte{[]byte("init"), []byte("A"), []byte("100"), []byte("B"), []byte("200")}}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	ctxt, txsim, sprop, prop := startTx(t, chainID, cis)

	//bad txid in response (should be "1"), should fail
	resp := &mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("init succeeded")}), Txid: "unknowntxid"}}
	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{resp}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", "1", false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	//set the right TxID in response now
	resp.RespMsg.(*pb.ChaincodeMessage).Txid = "1"

	badcccid := ccprovider.NewCCContext(chainID, ccname, "unknownver", "1", false, sprop, prop)

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
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutStateInfo{Key: "A", Value: []byte("100")}), Txid: "1"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutStateInfo{Key: "B", Value: []byte("200")}), Txid: "1"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), ChaincodeEvent: &pb.ChaincodeEvent{ChaincodeId: ccname}, Txid: "1"}}}}

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
	ci := &pb.ChaincodeInput{[][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	ctxt, txsim, sprop, prop := startTx(t, chainID, cis)

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: []byte("A"), Txid: "2"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: []byte("B"), Txid: "2"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutStateInfo{Key: "A", Value: []byte("90")}), Txid: "2"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutStateInfo{Key: "B", Value: []byte("210")}), Txid: "2"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: putils.MarshalOrPanic(&pb.PutStateInfo{Key: "TODEL", Value: []byte("-to-be-deleted-")}), Txid: "2"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "2"}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", "2", false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	//delete the extra var
	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: []byte("TODEL"), Txid: "3"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: []byte("TODEL"), Txid: "3"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "3"}}}}

	cccid.TxID = "3"
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	//get the extra var and delete it
	//NOTE- we are calling ExecuteWithErrorFilter which returns error if chaincode returns ERROR response
	respSet = &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: []byte("TODEL"), Txid: "4"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "variable not found"}), Txid: "4"}}}}

	cccid.TxID = "4"
	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func getQueryStateByRange(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{[][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	ctxt, txsim, sprop, prop := startTx(t, chainID, cis)

	//create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: putils.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: "5"}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: putils.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: "5"}
	}

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Payload: putils.MarshalOrPanic(&pb.GetStateByRange{StartKey: "A", EndKey: "B"}), Txid: "5"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, queryStateCloseFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "5"}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", "5", false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func cc2cc(t *testing.T, chainID, chainID2, ccname string, ccSide *mockpeer.MockCCComm) error {
	calledCC := "calledCC"
	//starts and registers the CC
	_, calledCCSide := startCC(t, calledCC)
	if calledCCSide == nil {
		t.Fatalf("start up failed for called CC")
	}
	defer calledCCSide.Quit()

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: calledCC, Version: "0"}
	ci := &pb.ChaincodeInput{[][]byte{[]byte("deploycc")}}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	//first deploy the new cc to LSCC
	ctxt, txsim, sprop, prop := startTx(t, chainID, cis)
	cccid := ccprovider.NewCCContext(chainID, calledCC, "0", "6", false, sprop, prop)

	deployCC(t, ctxt, cccid, cis.ChaincodeSpec)

	//commit
	endTx(t, cccid, txsim, cis)

	//now do the cc2cc
	chaincodeID = &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci = &pb.ChaincodeInput{[][]byte{[]byte("invokecc")}}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	ctxt, txsim, sprop, prop = startTx(t, chainID, cis)

	if _, _, err := ccprovider.GetChaincodeProvider().GetCCValidationInfoFromLSCC(ctxt, "getccdata", sprop, prop, chainID, calledCC); err != nil {
		t.Fatalf("Could not get chaincode data from lscc for %s", calledCC)
	}

	sysCCVers := util.GetSysCCVersion()
	//call a callable system CC, a regular cc, a regular cc on a different chain and an uncallable system cc and expect an error inthe last one
	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "lscc:" + sysCCVers}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte{}}}}), Txid: "7"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte{}}}}), Txid: "7"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID2}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte{}}}}), Txid: "7"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: putils.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "vscc:" + sysCCVers}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte{}}}}), Txid: "7"}}}}

	respSet2 := &mockpeer.MockResponseSet{nil, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "7"}}}}
	calledCCSide.SetResponses(respSet2)

	cccid = ccprovider.NewCCContext(chainID, ccname, "0", "7", false, sprop, prop)

	execCC(t, ctxt, ccSide, cccid, false, true, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func getQueryResult(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{[][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	ctxt, txsim, sprop, prop := startTx(t, chainID, cis)

	kvs := make([]*plgr.KV, 1000)
	for i := 0; i < 1000; i++ {
		kvs[i] = &plgr.KV{chainID, fmt.Sprintf("%d", i), []byte(fmt.Sprintf("%d", i))}
	}

	queryExec := &mockExecQuerySimulator{resultsIter: make(map[string]map[string]*mockResultsIterator)}
	queryExec.resultsIter[ccname] = map[string]*mockResultsIterator{"goodquery": &mockResultsIterator{kvs: kvs}}

	queryExec.txsim = ctxt.Value(TXSimulatorKey).(ledger.TxSimulator)
	ctxt = context.WithValue(ctxt, TXSimulatorKey, queryExec)

	//create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: putils.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: "8"}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: putils.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: "8"}
	}

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Payload: putils.MarshalOrPanic(&pb.GetQueryResult{Query: "goodquery"}), Txid: "8"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, queryStateCloseFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "8"}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", "8", false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
}

func getHistory(t *testing.T, chainID, ccname string, ccSide *mockpeer.MockCCComm) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{[][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	ctxt, txsim, sprop, prop := startTx(t, chainID, cis)

	kvs := make([]*plgr.KV, 1000)
	for i := 0; i < 1000; i++ {
		kvs[i] = &plgr.KV{chainID, fmt.Sprintf("%d", i), []byte(fmt.Sprintf("%d", i))}
	}

	queryExec := &mockExecQuerySimulator{resultsIter: make(map[string]map[string]*mockResultsIterator)}
	queryExec.resultsIter[ccname] = map[string]*mockResultsIterator{"goodquery": &mockResultsIterator{kvs: kvs}}

	queryExec.txsim = ctxt.Value(TXSimulatorKey).(ledger.TxSimulator)
	ctxt = context.WithValue(ctxt, TXSimulatorKey, queryExec)

	//create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: putils.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: "8"}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: putils.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: "8"}
	}

	respSet := &mockpeer.MockResponseSet{errorFunc, nil, []*mockpeer.MockResponse{
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Payload: putils.MarshalOrPanic(&pb.GetQueryResult{Query: "goodquery"}), Txid: "8"}},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, queryStateNextFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, queryStateCloseFunc},
		&mockpeer.MockResponse{&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: putils.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "8"}}}}

	cccid := ccprovider.NewCCContext(chainID, ccname, "0", "8", false, sprop, prop)
	execCC(t, ctxt, ccSide, cccid, false, false, done, cis, respSet)

	endTx(t, cccid, txsim, cis)

	return nil
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
	_, ccSide := startCC(t, ccname)
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

	//call's query state range
	getQueryStateByRange(t, chainID, ccname, ccSide)

	//call's cc2cc (variation with syscc calls)
	cc2cc(t, chainID, chainID2, ccname, ccSide)

	//call's query result
	getQueryResult(t, chainID, ccname, ccSide)

	//call's history result
	getHistory(t, chainID, ccname, ccSide)

	ccSide.Quit()
}
