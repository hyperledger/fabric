/*
Copyright IBM Corp. 2016 All Rights Reserved.

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
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"path/filepath"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// attributes to request in the batch of tcerts while deploying, invoking
var attributes = []string{"company", "position"}

func getNowMillis() int64 {
	nanos := time.Now().UnixNano()
	return nanos / 1000000
}

//initialize peer and start up. If security==enabled, login as vp
func initPeer(chainIDs ...string) (net.Listener, error) {
	//start clean
	finitPeer(nil, chainIDs...)

	peer.MockInitialize()

	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			return nil, fmt.Errorf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	peerAddress, err := peer.GetLocalAddress()
	if err != nil {
		return nil, fmt.Errorf("Error obtaining peer address: %s", err)
	}
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return nil, fmt.Errorf("Error starting peer listener %s", err)
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout))

	scc.RegisterSysCCs()

	for _, id := range chainIDs {
		scc.DeDeploySysCCs(id)
		if err = peer.MockCreateChain(id); err != nil {
			closeListenerAndSleep(lis)
			return nil, err
		}
		scc.DeploySysCCs(id)
	}

	go grpcServer.Serve(lis)

	return lis, nil
}

func finitPeer(lis net.Listener, chainIDs ...string) {
	if lis != nil {
		for _, c := range chainIDs {
			scc.DeDeploySysCCs(c)
			if lgr := peer.GetLedger(c); lgr != nil {
				lgr.Close()
			}
		}
		closeListenerAndSleep(lis)
	}
	ledgermgmt.CleanupTestEnv()
	ledgerPath := viper.GetString("peer.fileSystemPath")
	os.RemoveAll(ledgerPath)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))
}

func startTxSimulation(ctxt context.Context, chainID string) (context.Context, ledger.TxSimulator, error) {
	lgr := peer.GetLedger(chainID)
	txsim, err := lgr.NewTxSimulator()
	if err != nil {
		return nil, nil, err
	}

	ctxt = context.WithValue(ctxt, TXSimulatorKey, txsim)
	return ctxt, txsim, nil
}

func endTxSimulationCDS(chainID string, txid string, txsim ledger.TxSimulator, payload []byte, commit bool, cds *pb.ChaincodeDeploymentSpec) error {
	// get serialized version of the signer
	ss, err := signer.Serialize()
	if err != nil {
		return err
	}
	// get a proposal - we need it to get a transaction
	prop, err := putils.CreateDeployProposalFromCDS(txid, chainID, cds, ss, nil, nil, nil)
	if err != nil {
		return err
	}

	return endTxSimulation(chainID, txsim, payload, commit, prop)
}

func endTxSimulationCIS(chainID string, txid string, txsim ledger.TxSimulator, payload []byte, commit bool, cis *pb.ChaincodeInvocationSpec) error {
	// get serialized version of the signer
	ss, err := signer.Serialize()
	if err != nil {
		return err
	}
	// get a proposal - we need it to get a transaction
	prop, err := putils.CreateProposalFromCIS(txid, common.HeaderType_ENDORSER_TRANSACTION, chainID, cis, ss)
	if err != nil {
		return err
	}

	return endTxSimulation(chainID, txsim, payload, commit, prop)
}

//getting a crash from ledger.Commit when doing concurrent invokes
//It is likely intentional that ledger.Commit is serial (ie, the real
//Committer will invoke this serially on each block). Mimic that here
//by forcing serialization of the ledger.Commit call.
//
//NOTE-this should NOT have any effect on the older serial tests.
//This affects only the tests in concurrent_test.go which call these
//concurrently (100 concurrent invokes followed by 100 concurrent queries)
var _commitLock_ sync.Mutex

func endTxSimulation(chainID string, txsim ledger.TxSimulator, payload []byte, commit bool, prop *pb.Proposal) error {
	txsim.Done()
	if lgr := peer.GetLedger(chainID); lgr != nil {
		if commit {
			var txSimulationResults []byte
			var err error

			//get simulation results
			if txSimulationResults, err = txsim.GetTxSimulationResults(); err != nil {
				return err
			}

			// assemble a (signed) proposal response message
			resp, err := putils.CreateProposalResponse(prop.Header, prop.Payload, &pb.Response{Status: 200}, txSimulationResults, nil, nil, signer)
			if err != nil {
				return err
			}

			// get the envelope
			env, err := putils.CreateSignedTx(prop, signer, resp)
			if err != nil {
				return err
			}

			envBytes, err := putils.GetBytesEnvelope(env)
			if err != nil {
				return err
			}

			//create the block with 1 transaction
			block := common.NewBlock(1, []byte{})
			block.Data.Data = [][]byte{envBytes}
			//commit the block

			//see comment on _commitLock_
			_commitLock_.Lock()
			defer _commitLock_.Unlock()
			if err := lgr.Commit(block); err != nil {
				return err
			}
		}
	}

	return nil
}

// Build a chaincode.
func getDeploymentSpec(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	fmt.Printf("getting deployment spec for chaincode spec: %v\n", spec)
	codePackageBytes, err := container.GetChaincodePackageBytes(spec)
	if err != nil {
		return nil, err
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

//getDeployLCCCSpec gets the spec for the chaincode deployment to be sent to LCCC
func getDeployLCCCSpec(chainID string, cds *pb.ChaincodeDeploymentSpec) (*pb.ChaincodeInvocationSpec, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	//wrap the deployment in an invocation spec to lccc...
	lcccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Name: "lccc"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte(chainID), b}}}}

	return lcccSpec, nil
}

// Deploy a chaincode - i.e., build and initialize.
func deploy(ctx context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeSpec) (b []byte, err error) {
	// First build and get the deployment spec
	chaincodeDeploymentSpec, err := getDeploymentSpec(ctx, spec)
	if err != nil {
		return nil, err
	}

	return deploy2(ctx, cccid, chaincodeDeploymentSpec)
}

func deploy2(ctx context.Context, cccid *ccprovider.CCContext, chaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec) (b []byte, err error) {
	cis, err := getDeployLCCCSpec(cccid.ChainID, chaincodeDeploymentSpec)
	if err != nil {
		return nil, fmt.Errorf("Error creating lccc spec : %s\n", err)
	}

	ctx, txsim, err := startTxSimulation(ctx, cccid.ChainID)
	if err != nil {
		return nil, fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	uuid := util.GenerateUUID()

	cccid.TxID = uuid

	defer func() {
		//no error, lets try commit
		if err == nil {
			//capture returned error from commit
			err = endTxSimulationCDS(cccid.ChainID, uuid, txsim, []byte("deployed"), true, chaincodeDeploymentSpec)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCDS(cccid.ChainID, uuid, txsim, []byte("deployed"), false, chaincodeDeploymentSpec)
		}
	}()

	sysCCVers := util.GetSysCCVersion()
	lcccid := ccprovider.NewCCContext(cccid.ChainID, cis.ChaincodeSpec.ChaincodeID.Name, sysCCVers, uuid, true, nil)

	//write to lccc
	if _, _, err = ExecuteWithErrorFilter(ctx, lcccid, cis); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}

	if b, _, err = ExecuteWithErrorFilter(ctx, cccid, chaincodeDeploymentSpec); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}

	return b, nil
}

// Invoke a chaincode.
func invoke(ctx context.Context, chainID string, spec *pb.ChaincodeSpec) (ccevt *pb.ChaincodeEvent, uuid string, retval []byte, err error) {
	return invokeWithVersion(ctx, chainID, "0", spec)
}

// Invoke a chaincode with version (needed for upgrade)
func invokeWithVersion(ctx context.Context, chainID string, version string, spec *pb.ChaincodeSpec) (ccevt *pb.ChaincodeEvent, uuid string, retval []byte, err error) {
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	uuid = util.GenerateUUID()

	var txsim ledger.TxSimulator
	ctx, txsim, err = startTxSimulation(ctx, chainID)
	if err != nil {
		return nil, uuid, nil, fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer func() {
		//no error, lets try commit
		if err == nil {
			//capture returned error from commit
			err = endTxSimulationCIS(chainID, uuid, txsim, []byte("invoke"), true, chaincodeInvocationSpec)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCIS(chainID, uuid, txsim, []byte("invoke"), false, chaincodeInvocationSpec)
		}
	}()

	cccid := ccprovider.NewCCContext(chainID, chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name, version, uuid, false, nil)
	retval, ccevt, err = ExecuteWithErrorFilter(ctx, cccid, chaincodeInvocationSpec)
	if err != nil {
		return nil, uuid, nil, fmt.Errorf("Error invoking chaincode: %s", err)
	}

	return ccevt, uuid, retval, err
}

func closeListenerAndSleep(l net.Listener) {
	if l != nil {
		l.Close()
		time.Sleep(2 * time.Second)
	}
}

func executeDeployTransaction(t *testing.T, chainID string, name string, url string) {
	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: name, Path: url}, Input: &pb.ChaincodeInput{Args: args}}

	cccid := ccprovider.NewCCContext(chainID, name, "0", "", false, nil)

	_, err = deploy(ctxt, cccid, spec)

	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		t.Fail()
		t.Logf("Error deploying <%s>: %s", chaincodeID, err)
		return
	}

	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

func chaincodeQueryChaincode(chainID string, user string) error {
	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Name: "example02", Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, Input: &pb.ChaincodeInput{Args: args}}

	cccid1 := ccprovider.NewCCContext(chainID, "example02", "0", "", false, nil)

	_, err := deploy(ctxt, cccid1, spec1)

	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		return fmt.Errorf("Error initializing chaincode %s(%s)", chaincodeID1, err)
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example05"

	cID2 := &pb.ChaincodeID{Name: "example05", Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f, "sum", "0")

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}

	cccid2 := ccprovider.NewCCContext(chainID, "example05", "0", "", false, nil)

	_, err = deploy(ctxt, cccid2, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Error initializing chaincode %s(%s)", chaincodeID2, err)
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn query the first chaincode
	f = "invoke"
	args = util.ToChaincodeArgs(f, chaincodeID1, "sum")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	var retVal []byte
	_, _, retVal, err = invoke(ctxt, chainID, spec2)

	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Error invoking <%s>: %s", chaincodeID2, err)
	}

	// Check the return value
	result, err := strconv.Atoi(string(retVal))
	if err != nil || result != 300 {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Incorrect final state after transaction for <%s>: %s", chaincodeID1, err)
	}

	// Query second chaincode, which will inturn query the first chaincode
	f = "query"
	args = util.ToChaincodeArgs(f, chaincodeID1, "sum")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, retVal, err = invoke(ctxt, chainID, spec2)

	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Error querying <%s>: %s", chaincodeID2, err)
	}

	// Check the return value
	result, err = strconv.Atoi(string(retVal))
	if err != nil || result != 300 {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Incorrect final value after query for <%s>: %s", chaincodeID1, err)
	}

	theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})

	return nil
}

// Test deploy of a transaction
func TestExecuteDeployTransaction(t *testing.T) {
	chainID := util.GetTestChainID()

	executeDeployTransaction(t, chainID, "example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01")
}

// Test deploy of a transaction with a GOPATH with multiple elements
func TestGopathExecuteDeployTransaction(t *testing.T) {
	chainID := util.GetTestChainID()

	// add a trailing slash to GOPATH
	// and a couple of elements - it doesn't matter what they are
	os.Setenv("GOPATH", os.Getenv("GOPATH")+string(os.PathSeparator)+string(os.PathListSeparator)+"/tmp/foo"+string(os.PathListSeparator)+"/tmp/bar")
	executeDeployTransaction(t, chainID, "example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01")
}

// Disable this temporarily.
// TODO: Need to enable this after update chaincode interface of chaincode repo.
// Test deploy of a transaction with a chaincode over HTTP.
//func TestHTTPExecuteDeployTransaction(t *testing.T) {
//	chainID := util.GetTestChainID()

//	// The chaincode used here cannot be from the fabric repo
//	// itself or it won't be downloaded because it will be found
//	// in GOPATH, which would defeat the test
//	executeDeployTransaction(t, chainID, "example01", "http://gopkg.in/mastersingh24/fabric-test-resources.v1")
//}

// Check the correctness of the final state after transaction execution.
func checkFinalState(cccid *ccprovider.CCContext) error {
	_, txsim, err := startTxSimulation(context.Background(), cccid.ChainID)
	if err != nil {
		return fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer txsim.Done()

	cName := cccid.GetCanonicalName()

	// Invoke ledger to get state
	var Aval, Bval int
	resbytes, resErr := txsim.GetState(cccid.Name, "a")
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	fmt.Printf("Got string: %s\n", string(resbytes))
	Aval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	if Aval != 90 {
		return fmt.Errorf("Incorrect result. Aval %d != 90 <%s>", Aval, cName)
	}

	resbytes, resErr = txsim.GetState(cccid.Name, "b")
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	Bval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	if Bval != 210 {
		return fmt.Errorf("Incorrect result. Bval %d != 210 <%s>", Bval, cName)
	}

	// Success
	fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)
	return nil
}

// Invoke chaincode_example02
func invokeExample02Transaction(ctxt context.Context, cccid *ccprovider.CCContext, cID *pb.ChaincodeID, args []string, destroyImage bool) error {

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: argsDeploy}}
	_, err := deploy(ctxt, cccid, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		return fmt.Errorf("Error deploying <%s>: %s", chaincodeID, err)
	}

	time.Sleep(time.Second)

	if destroyImage {
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		dir := container.DestroyImageReq{CCID: ccintf.CCID{ChaincodeSpec: spec, NetworkID: theChaincodeSupport.peerNetworkID, PeerID: theChaincodeSupport.peerID, ChainID: cccid.ChainID}, Force: true, NoPrune: true}

		_, err = container.VMCProcess(ctxt, container.DOCKER, dir)
		if err != nil {
			err = fmt.Errorf("Error destroying image: %s", err)
			return err
		}
	}

	f = "invoke"
	invokeArgs := append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	_, uuid, _, err := invoke(ctxt, cccid.ChainID, spec)
	if err != nil {
		return fmt.Errorf("Error invoking <%s>: %s", cccid.Name, err)
	}

	cccid.TxID = uuid
	err = checkFinalState(cccid)
	if err != nil {
		return fmt.Errorf("Incorrect final state after transaction for <%s>: %s", chaincodeID, err)
	}

	// Test for delete state
	f = "delete"
	delArgs := util.ToChaincodeArgs(f, "a")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: delArgs}}
	_, uuid, _, err = invoke(ctxt, cccid.ChainID, spec)
	if err != nil {
		return fmt.Errorf("Error deleting state in <%s>: %s", cccid.Name, err)
	}

	return nil
}

func TestExecuteInvokeTransaction(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	cccid := ccprovider.NewCCContext(chainID, "example02", "0", "", false, nil)
	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID := &pb.ChaincodeID{Name: "example02", Path: url}

	args := []string{"a", "b", "10"}
	err = invokeExample02Transaction(ctxt, cccid, chaincodeID, args, true)
	if err != nil {
		t.Fail()
		t.Logf("Error invoking transaction: %s", err)
	} else {
		fmt.Printf("Invoke test passed\n")
		t.Logf("Invoke test passed")
	}

	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
}

// Test the execution of an invalid transaction.
func TestExecuteInvokeInvalidTransaction(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID := &pb.ChaincodeID{Name: "example02", Path: url}

	cccid := ccprovider.NewCCContext(chainID, "example02", "0", "", false, nil)

	//FAIL, FAIL!
	args := []string{"x", "-1"}
	err = invokeExample02Transaction(ctxt, cccid, chaincodeID, args, false)

	//this HAS to fail with expectedDeltaStringPrefix
	if err != nil {
		errStr := err.Error()
		t.Logf("Got error %s\n", errStr)
		t.Logf("InvalidInvoke test passed")
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})

		return
	}

	t.Fail()
	t.Logf("Error invoking transaction %s", err)

	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
}

// Test the execution of a chaincode that invokes another chaincode.
func TestChaincodeInvokeChaincode(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	err = chaincodeInvokeChaincode(t, chainID, "")
	if err != nil {
		t.Fail()
		t.Logf("Failed chaincode invoke chaincode : %s", err)
		closeListenerAndSleep(lis)
		return
	}

	closeListenerAndSleep(lis)
}

func chaincodeInvokeChaincode(t *testing.T, chainID string, user string) (err error) {
	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Name: "example02", Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, Input: &pb.ChaincodeInput{Args: args}}

	cccid1 := ccprovider.NewCCContext(chainID, "example02", "0", "", false, nil)

	_, err = deploy(ctxt, cccid1, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID1, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		return
	}

	t.Logf("deployed chaincode_example02 got cID1:% s,\n chaincodeID1:% s", cID1, chaincodeID1)

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example04"

	cID2 := &pb.ChaincodeID{Name: "example04", Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f, "e", "0")

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}

	cccid2 := ccprovider.NewCCContext(chainID, "example04", "0", "", false, nil)

	_, err = deploy(ctxt, cccid2, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID2, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode passing the first chaincode's name as first param,
	// which will inturn invoke the first chaincode
	f = "invoke"
	cid := spec1.ChaincodeID.Name
	args = util.ToChaincodeArgs(f, cid, "e", "1")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	var uuid string
	_, uuid, _, err = invoke(ctxt, chainID, spec2)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID2, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return
	}

	cccid1.TxID = uuid

	// Check the state in the ledger
	err = checkFinalState(cccid1)
	if err != nil {
		t.Fail()
		t.Logf("Incorrect final state after transaction for <%s>: %s", chaincodeID1, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return
	}

	theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})

	return
}

// Test the execution of a chaincode that invokes another chaincode with wrong parameters. Should receive error from
// from the called chaincode
func TestChaincodeInvokeChaincodeErrorCase(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Name: "example02", Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, Input: &pb.ChaincodeInput{Args: args}}

	cccid1 := ccprovider.NewCCContext(chainID, "example02", "0", "", false, nil)

	_, err = deploy(ctxt, cccid1, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID1, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		return
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/passthru"

	cID2 := &pb.ChaincodeID{Name: "pthru", Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f)

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}

	cccid2 := ccprovider.NewCCContext(chainID, "pthru", "0", "", false, nil)

	_, err = deploy(ctxt, cccid2, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID2, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn invoke the first chaincode but pass bad params
	f = chaincodeID1
	args = util.ToChaincodeArgs(f, "invoke", "a")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, _, err = invoke(ctxt, chainID, spec2)

	if err == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID2, err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return
	}

	if strings.Index(err.Error(), "Error invoking chaincode: Incorrect number of arguments. Expecting 3") < 0 {
		t.Fail()
		t.Logf("Unexpected error %s", err)
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return
	}

	theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
}

// Test the invocation of a transaction.
func TestQueries(t *testing.T) {

	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/map"
	cID := &pb.ChaincodeID{Name: "tmap", Path: url}

	f := "init"
	args := util.ToChaincodeArgs(f)

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}

	cccid := ccprovider.NewCCContext(chainID, "tmap", "0", "", false, nil)

	_, err = deploy(ctxt, cccid, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	// Invoke second chaincode, which will inturn invoke the first chaincode
	f = "put"
	args = util.ToChaincodeArgs(f, "key1", "{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, chainID, spec)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	f = "put"
	args = util.ToChaincodeArgs(f, "key2", "{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, chainID, spec)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	f = "put"
	args = util.ToChaincodeArgs(f, "key3", "{\"shipmentID\":\"161003PKC7300\",\"customsInvoice\":{\"methodOfTransport\":\"GROUND\",\"invoiceNumber\":\"00091622\"},\"weightUnitOfMeasure\":\"KGM\",\"volumeUnitOfMeasure\": \"CO\",\"dimensionUnitOfMeasure\":\"CM\",\"currency\":\"USD\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, chainID, spec)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	f = "keys"
	args = util.ToChaincodeArgs(f, "key0", "key3")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, chainID, spec)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	if ledgerconfig.IsCouchDBEnabled() == true {
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"currency\":\"USD\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(ctxt, chainID, spec)

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", chaincodeID, err)
			theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
			return
		}
	}
	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

func TestGetEvent(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/eventsender"

	cID := &pb.ChaincodeID{Name: "esender", Path: url}
	f := "init"
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(f)}}

	cccid := ccprovider.NewCCContext(chainID, "esender", "0", "", false, nil)

	_, err = deploy(ctxt, cccid, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	time.Sleep(time.Second)

	args := util.ToChaincodeArgs("invoke", "i", "am", "satoshi")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, Input: &pb.ChaincodeInput{Args: args}}

	var ccevt *pb.ChaincodeEvent
	ccevt, _, _, err = invoke(ctxt, chainID, spec)

	if err != nil {
		t.Logf("Error invoking chaincode %s(%s)", chaincodeID, err)
		t.Fail()
	}

	if ccevt == nil {
		t.Logf("Error ccevt is nil %s(%s)", chaincodeID, err)
		t.Fail()
	}

	if ccevt.ChaincodeID != chaincodeID {
		t.Logf("Error ccevt id(%s) != cid(%s)", ccevt.ChaincodeID, chaincodeID)
		t.Fail()
	}

	if strings.Index(string(ccevt.Payload), "i,am,satoshi") < 0 {
		t.Logf("Error expected event not found (%s)", string(ccevt.Payload))
		t.Fail()
	}

	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

// Test the execution of a chaincode that queries another chaincode
// example02 implements "query" as a function in Invoke. example05 calls example02
func TestChaincodeQueryChaincodeUsingInvoke(t *testing.T) {
	chainID := util.GetTestChainID()

	var peerLis net.Listener
	var err error
	if peerLis, err = initPeer(chainID); err != nil {
		t.Fail()
		t.Logf("Error registering user  %s", err)
		return
	}

	defer finitPeer(peerLis, chainID)

	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Name: "example02", Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, Input: &pb.ChaincodeInput{Args: args}}

	cccid1 := ccprovider.NewCCContext(chainID, "example02", "0", "", false, nil)

	_, err = deploy(ctxt, cccid1, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID1, err)
		return
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example05"

	cID2 := &pb.ChaincodeID{Name: "example05", Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f, "sum", "0")

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}

	cccid2 := ccprovider.NewCCContext(chainID, "example05", "0", "", false, nil)

	_, err = deploy(ctxt, cccid2, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID2, err)
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn query the first chaincode
	f = "invoke"
	args = util.ToChaincodeArgs(f, chaincodeID1, "sum")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	var retVal []byte
	_, _, retVal, err = invoke(ctxt, chainID, spec2)

	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID2, err)
		return
	}

	// Check the return value
	result, err := strconv.Atoi(string(retVal))
	if err != nil || result != 300 {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		t.Fail()
		t.Logf("Incorrect final state after transaction for <%s>: %s", chaincodeID1, err)
		return
	}

	// Query second chaincode, which will inturn query the first chaincode
	f = "query"
	args = util.ToChaincodeArgs(f, chaincodeID1, "sum")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, retVal, err = invoke(ctxt, chainID, spec2)

	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		t.Fail()
		t.Logf("Error querying <%s>: %s", chaincodeID2, err)
		return
	}

	// Check the return value
	result, err = strconv.Atoi(string(retVal))
	if err != nil || result != 300 {
		theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		t.Fail()
		t.Logf("Incorrect final value after query for <%s>: %s", chaincodeID1, err)
		return
	}

	theChaincodeSupport.Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
}

var signer msp.SigningIdentity

func TestMain(m *testing.M) {
	var err error

	// setup the MSP manager so that we can sign/verify
	mspMgrConfigDir := "../../msp/sampleconfig/"
	mspmgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)
	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize msp/signer")
		return
	}

	SetupTestConfig()
	os.Exit(m.Run())
}
