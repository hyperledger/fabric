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

	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/membersrvc/ca"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

// attributes to request in the batch of tcerts while deploying, invoking or querying
var attributes = []string{"company", "position"}

var testDBWrapper = db.NewTestDBWrapper()

func getNowMillis() int64 {
	nanos := time.Now().UnixNano()
	return nanos / 1000000
}

//initialize memberservices and startup
func initMemSrvc() (net.Listener, error) {
	//start clean
	finitMemSrvc(nil)

	ca.CacheConfiguration() // Cache configuration

	aca := ca.NewACA()
	eca := ca.NewECA(aca)
	tca := ca.NewTCA(eca)
	tlsca := ca.NewTLSCA(eca)

	sockp, err := net.Listen("tcp", viper.GetString("server.port"))
	if err != nil {
		return nil, err
	}

	var opts []grpc.ServerOption
	server := grpc.NewServer(opts...)

	aca.Start(server)
	eca.Start(server)
	tca.Start(server)
	tlsca.Start(server)

	go server.Serve(sockp)

	return sockp, nil
}

//cleanup memberservice debris
func finitMemSrvc(lis net.Listener) {
	closeListenerAndSleep(lis)
	os.RemoveAll(filepath.Join(os.TempDir(), "ca"))
}

//initialize peer and start up. If security==enabled, login as vp
func initPeer() (net.Listener, error) {
	//start clean
	finitPeer(nil)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			return nil, fmt.Errorf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	peerAddress := viper.GetString("peer.address")
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return nil, fmt.Errorf("Error starting peer listener %s", err)
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	// Install security object for peer
	var secHelper crypto.Peer
	if viper.GetBool("security.enabled") {
		enrollID := viper.GetString("security.enrollID")
		enrollSecret := viper.GetString("security.enrollSecret")
		if err = crypto.RegisterValidator(enrollID, nil, enrollID, enrollSecret); nil != err {
			return nil, err
		}
		secHelper, err = crypto.InitValidator(enrollID, nil)
		if nil != err {
			return nil, err
		}
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, secHelper))

	go grpcServer.Serve(lis)

	return lis, nil
}

func finitPeer(lis net.Listener) {
	closeListenerAndSleep(lis)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))
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

func createDeployTransaction(dspec *pb.ChaincodeDeploymentSpec, uuid string) (*pb.Transaction, error) {
	var tx *pb.Transaction
	var err error
	var sec crypto.Client
	if dspec.ChaincodeSpec.SecureContext != "" {
		sec, err = crypto.InitClient(dspec.ChaincodeSpec.SecureContext, nil)
		defer crypto.CloseClient(sec)

		if nil != err {
			return nil, err
		}

		tx, err = sec.NewChaincodeDeployTransaction(dspec, uuid, attributes...)
		if nil != err {
			return nil, err
		}
	} else {
		tx, err = pb.NewChaincodeDeployTransaction(dspec, uuid)
		if err != nil {
			return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
		}
	}
	return tx, nil
}

func createTransaction(invokeTx bool, spec *pb.ChaincodeInvocationSpec, uuid string) (*pb.Transaction, error) {
	var tx *pb.Transaction
	var err error
	var sec crypto.Client
	if nil != sec {
		sec, err = crypto.InitClient(spec.ChaincodeSpec.SecureContext, nil)
		defer crypto.CloseClient(sec)
		if nil != err {
			return nil, err
		}
		if invokeTx {
			tx, err = sec.NewChaincodeExecute(spec, uuid, attributes...)
		} else {
			tx, err = sec.NewChaincodeQuery(spec, uuid, attributes...)
		}
		if nil != err {
			return nil, err
		}
	} else {
		var t pb.Transaction_Type
		if invokeTx {
			t = pb.Transaction_CHAINCODE_INVOKE
		} else {
			t = pb.Transaction_CHAINCODE_QUERY
		}
		tx, err = pb.NewChaincodeExecute(spec, uuid, t)
		if nil != err {
			return nil, err
		}
	}
	return tx, nil
}

// Deploy a chaincode - i.e., build and initialize.
func deploy(ctx context.Context, spec *pb.ChaincodeSpec) ([]byte, error) {
	// First build and get the deployment spec
	chaincodeDeploymentSpec, err := getDeploymentSpec(ctx, spec)
	if err != nil {
		return nil, err
	}

	tid := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := createDeployTransaction(chaincodeDeploymentSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Failed to get handle to ledger: %s ", err)
	}
	ledger.BeginTxBatch("1")
	b, _, err := Execute(ctx, GetChain(DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return b, err
}

func deploy2(ctx context.Context, chaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec) ([]byte, error) {
	tid := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := createDeployTransaction(chaincodeDeploymentSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	b, _, err := Execute(ctx, GetChain(DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return b, err
}

// Invoke or query a chaincode.
func invoke(ctx context.Context, spec *pb.ChaincodeSpec, typ pb.Transaction_Type) (*pb.ChaincodeEvent, string, []byte, error) {
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	uuid := util.GenerateUUID()

	var transaction *pb.Transaction
	var err error
	if typ == pb.Transaction_CHAINCODE_QUERY {
		transaction, err = createTransaction(false, chaincodeInvocationSpec, uuid)
	} else {
		transaction, err = createTransaction(true, chaincodeInvocationSpec, uuid)
	}
	if err != nil {
		return nil, uuid, nil, fmt.Errorf("Error invoking chaincode: %s ", err)
	}

	var retval []byte
	var execErr error
	var ccevt *pb.ChaincodeEvent
	if typ == pb.Transaction_CHAINCODE_QUERY {
		retval, ccevt, execErr = Execute(ctx, GetChain(DefaultChain), transaction)
	} else {
		ledger, _ := ledger.GetLedger()
		ledger.BeginTxBatch("1")
		retval, ccevt, execErr = Execute(ctx, GetChain(DefaultChain), transaction)
		if execErr != nil {
			return nil, uuid, nil, fmt.Errorf("Error invoking chaincode: %s ", execErr)
		}
		ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)
	}

	return ccevt, uuid, retval, execErr
}

func closeListenerAndSleep(l net.Listener) {
	if l != nil {
		l.Close()
		time.Sleep(2 * time.Second)
	}
}

func executeDeployTransaction(t *testing.T, url string) {
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//lis, err := net.Listen("tcp", viper.GetString("peer.address"))

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Path: url}, CtorMsg: &pb.ChaincodeInput{Args: args}}
	_, err = deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		t.Fail()
		t.Logf("Error deploying <%s>: %s", chaincodeID, err)
		return
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	closeListenerAndSleep(lis)
}

// Test deploy of a transaction
func TestExecuteDeployTransaction(t *testing.T) {
	testDBWrapper.CleanDB(t)
	executeDeployTransaction(t, "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01")
}

// Test deploy of a transaction with a GOPATH with multiple elements
func TestGopathExecuteDeployTransaction(t *testing.T) {
	// add a trailing slash to GOPATH
	// and a couple of elements - it doesn't matter what they are
	os.Setenv("GOPATH", os.Getenv("GOPATH")+string(os.PathSeparator)+string(os.PathListSeparator)+"/tmp/foo"+string(os.PathListSeparator)+"/tmp/bar")
	fmt.Printf("set GOPATH to: \"%s\"\n", os.Getenv("GOPATH"))
	testDBWrapper.CleanDB(t)
	executeDeployTransaction(t, "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01")
}

// Test deploy of a transaction with a chaincode over HTTP.
func TestHTTPExecuteDeployTransaction(t *testing.T) {
	// The chaincode used here cannot be from the fabric repo
	// itself or it won't be downloaded because it will be found
	// in GOPATH, which would defeat the test
	testDBWrapper.CleanDB(t)
	executeDeployTransaction(t, "http://gopkg.in/mastersingh24/fabric-test-resources.v0")
}

// Check the correctness of the final state after transaction execution.
func checkFinalState(uuid string, chaincodeID string) error {
	// Check the state in the ledger
	ledgerObj, ledgerErr := ledger.GetLedger()
	if ledgerErr != nil {
		return fmt.Errorf("Error checking ledger for <%s>: %s", chaincodeID, ledgerErr)
	}

	// Invoke ledger to get state
	var Aval, Bval int
	resbytes, resErr := ledgerObj.GetState(chaincodeID, "a", false)
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", chaincodeID, resErr)
	}
	fmt.Printf("Got string: %s\n", string(resbytes))
	Aval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", chaincodeID, resErr)
	}
	if Aval != 90 {
		return fmt.Errorf("Incorrect result. Aval is wrong for <%s>", chaincodeID)
	}

	resbytes, resErr = ledgerObj.GetState(chaincodeID, "b", false)
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", chaincodeID, resErr)
	}
	Bval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", chaincodeID, resErr)
	}
	if Bval != 210 {
		return fmt.Errorf("Incorrect result. Bval is wrong for <%s>", chaincodeID)
	}

	// Success
	fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)
	return nil
}

// Invoke chaincode_example02
func invokeExample02Transaction(ctxt context.Context, cID *pb.ChaincodeID, args []string, destroyImage bool) error {

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: argsDeploy}}
	_, err := deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		return fmt.Errorf("Error deploying <%s>: %s", chaincodeID, err)
	}

	time.Sleep(time.Second)

	if destroyImage {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		dir := container.DestroyImageReq{CCID: ccintf.CCID{ChaincodeSpec: spec, NetworkID: GetChain(DefaultChain).peerNetworkID, PeerID: GetChain(DefaultChain).peerID}, Force: true, NoPrune: true}

		_, err = container.VMCProcess(ctxt, container.DOCKER, dir)
		if err != nil {
			err = fmt.Errorf("Error destroying image: %s", err)
			return err
		}
	}

	f = "invoke"
	invokeArgs := append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	_, uuid, _, err := invoke(ctxt, spec, pb.Transaction_CHAINCODE_INVOKE)
	if err != nil {
		return fmt.Errorf("Error invoking <%s>: %s", chaincodeID, err)
	}

	err = checkFinalState(uuid, chaincodeID)
	if err != nil {
		return fmt.Errorf("Incorrect final state after transaction for <%s>: %s", chaincodeID, err)
	}

	// Test for delete state
	f = "delete"
	delArgs := util.ToChaincodeArgs(f, "a")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: delArgs}}
	_, uuid, _, err = invoke(ctxt, spec, pb.Transaction_CHAINCODE_INVOKE)
	if err != nil {
		return fmt.Errorf("Error deleting state in <%s>: %s", chaincodeID, err)
	}

	return nil
}

func TestExecuteInvokeTransaction(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption

	//TLS is on by default. This is the ONLY test that does NOT use TLS
	viper.Set("peer.tls.enabled", false)

	//turn OFF keepalive. All other tests use keepalive
	viper.Set("peer.chaincode.keepalive", "0")

	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID := &pb.ChaincodeID{Path: url}

	args := []string{"a", "b", "10"}
	err = invokeExample02Transaction(ctxt, chaincodeID, args, true)
	if err != nil {
		t.Fail()
		t.Logf("Error invoking transaction: %s", err)
	} else {
		fmt.Printf("Invoke test passed\n")
		t.Logf("Invoke test passed")
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})

	closeListenerAndSleep(lis)
}

// Execute multiple transactions and queries.
func exec(ctxt context.Context, chaincodeID string, numTrans int, numQueries int) []error {
	var wg sync.WaitGroup
	errs := make([]error, numTrans+numQueries)

	e := func(qnum int, typ pb.Transaction_Type) {
		defer wg.Done()
		var spec *pb.ChaincodeSpec
		if typ == pb.Transaction_CHAINCODE_INVOKE {
			f := "invoke"
			args := util.ToChaincodeArgs(f, "a", "b", "10")

			spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: chaincodeID}, CtorMsg: &pb.ChaincodeInput{Args: args}}
		} else {
			f := "query"
			args := util.ToChaincodeArgs(f, "a")

			spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: chaincodeID}, CtorMsg: &pb.ChaincodeInput{Args: args}}
		}

		_, _, _, err := invoke(ctxt, spec, typ)

		if err != nil {
			errs[qnum] = fmt.Errorf("Error executing <%s>: %s", chaincodeID, err)
			return
		}
	}
	wg.Add(numTrans + numQueries)

	//execute transactions sequentially..
	go func() {
		for i := 0; i < numTrans; i++ {
			e(i, pb.Transaction_CHAINCODE_INVOKE)
		}
	}()

	//...but queries in parallel
	for i := numTrans; i < numTrans+numQueries; i++ {
		go e(i, pb.Transaction_CHAINCODE_QUERY)
	}

	wg.Wait()
	return errs
}

// Test the execution of a query.
func TestExecuteQuery(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID := &pb.ChaincodeID{Path: url}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(2 * time.Second)

	//start := getNowMillis()
	//fmt.Fprintf(os.Stderr, "Starting: %d\n", start)
	numTrans := 2
	numQueries := 10
	errs := exec(ctxt, chaincodeID, numTrans, numQueries)

	var numerrs int
	for i := 0; i < numTrans+numQueries; i++ {
		if errs[i] != nil {
			t.Logf("Error doing query on %d %s", i, errs[i])
			numerrs++
		}
	}

	if numerrs == 0 {
		t.Logf("Query test passed")
	} else {
		t.Logf("Query test failed(total errors %d)", numerrs)
		t.Fail()
	}

	//end := getNowMillis()
	//fmt.Fprintf(os.Stderr, "Ending: %d\n", end)
	//fmt.Fprintf(os.Stderr, "Elapsed : %d millis\n", end-start)
	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	closeListenerAndSleep(lis)
}

// Test the execution of an invalid transaction.
func TestExecuteInvokeInvalidTransaction(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID := &pb.ChaincodeID{Path: url}

	//FAIL, FAIL!
	args := []string{"x", "-1"}
	err = invokeExample02Transaction(ctxt, chaincodeID, args, false)

	//this HAS to fail with expectedDeltaStringPrefix
	if err != nil {
		errStr := err.Error()
		t.Logf("Got error %s\n", errStr)
		t.Logf("InvalidInvoke test passed")
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})

		closeListenerAndSleep(lis)
		return
	}

	t.Fail()
	t.Logf("Error invoking transaction %s", err)

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})

	closeListenerAndSleep(lis)
}

// Test the execution of an invalid query.
func TestExecuteInvalidQuery(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example03"

	cID := &pb.ChaincodeID{Path: url}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	f = "query"
	args = util.ToChaincodeArgs(f, "b", "200")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}
	// This query should fail as it attempts to put state
	_, _, _, err = invoke(ctxt, spec, pb.Transaction_CHAINCODE_QUERY)

	if err == nil {
		t.Fail()
		t.Logf("This query should not have succeeded as it attempts to put state")
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	closeListenerAndSleep(lis)
}

// Test the execution of a chaincode that invokes another chaincode.
func TestChaincodeInvokeChaincode(t *testing.T) {
	t.Logf("TestChaincodeInvokeChaincode starting")
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID1, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		closeListenerAndSleep(lis)
		return
	}

	t.Logf("deployed chaincode_example02 got cID1:% s,\n chaincodeID1:% s", cID1, chaincodeID1)

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example04"

	cID2 := &pb.ChaincodeID{Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f, "e", "0")

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID2, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn invoke the first chaincode
	f = "invoke"
	args = util.ToChaincodeArgs(f, "e", "1")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	var uuid string
	_, uuid, _, err = invoke(ctxt, spec2, pb.Transaction_CHAINCODE_INVOKE)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID2, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	// Check the state in the ledger
	err = checkFinalState(uuid, chaincodeID1)
	if err != nil {
		t.Fail()
		t.Logf("Incorrect final state after transaction for <%s>: %s", chaincodeID1, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
	closeListenerAndSleep(lis)
}

// Test the execution of a chaincode that invokes another chaincode with wrong parameters. Should receive error from
// from the called chaincode
func TestChaincodeInvokeChaincodeErrorCase(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID1, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/passthru"

	cID2 := &pb.ChaincodeID{Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f)

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID2, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn invoke the first chaincode but pass bad params
	f = chaincodeID1
	args = util.ToChaincodeArgs(f, "invoke", "a")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, _, err = invoke(ctxt, spec2, pb.Transaction_CHAINCODE_INVOKE)

	if err == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID2, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	if strings.Index(err.Error(), "Incorrect number of arguments. Expecting 3") < 0 {
		t.Fail()
		t.Logf("Unexpected error %s", err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
	closeListenerAndSleep(lis)
}

func chaincodeQueryChaincode(user string) error {
	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, CtorMsg: &pb.ChaincodeInput{Args: args}, SecureContext: user}

	_, err := deploy(ctxt, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		return fmt.Errorf("Error initializing chaincode %s(%s)", chaincodeID1, err)
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example05"

	cID2 := &pb.ChaincodeID{Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f, "sum", "0")

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}, SecureContext: user}

	_, err = deploy(ctxt, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Error initializing chaincode %s(%s)", chaincodeID2, err)
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn query the first chaincode
	f = "invoke"
	args = util.ToChaincodeArgs(f, chaincodeID1, "sum")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}, SecureContext: user}
	// Invoke chaincode
	var retVal []byte
	_, _, retVal, err = invoke(ctxt, spec2, pb.Transaction_CHAINCODE_INVOKE)

	if err != nil {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Error invoking <%s>: %s", chaincodeID2, err)
	}

	// Check the return value
	result, err := strconv.Atoi(string(retVal))
	if err != nil || result != 300 {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Incorrect final state after transaction for <%s>: %s", chaincodeID1, err)
	}

	// Query second chaincode, which will inturn query the first chaincode
	f = "query"
	args = util.ToChaincodeArgs(f, chaincodeID1, "sum")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}, SecureContext: user}
	// Invoke chaincode
	_, _, retVal, err = invoke(ctxt, spec2, pb.Transaction_CHAINCODE_QUERY)

	if err != nil {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Error querying <%s>: %s", chaincodeID2, err)
	}

	// Check the return value
	result, err = strconv.Atoi(string(retVal))
	if err != nil || result != 300 {
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		return fmt.Errorf("Incorrect final value after query for <%s>: %s", chaincodeID1, err)
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})

	return nil
}

// Test the execution of a chaincode query that queries another chaincode without security enabled
func TestChaincodeQueryChaincode(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var peerLis net.Listener
	var err error
	if peerLis, err = initPeer(); err != nil {
		t.Fail()
		t.Logf("Error registering user  %s", err)
		return
	}

	if err = chaincodeQueryChaincode(""); err != nil {
		finitPeer(peerLis)
		t.Fail()
		t.Logf("Error executing test %s", err)
		return
	}

	finitPeer(peerLis)
}

// Test the execution of a chaincode that queries another chaincode with invalid parameter. Should receive error from
// from the called chaincode
func TestChaincodeQueryChaincodeErrorCase(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	// Deploy first chaincode
	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	cID1 := &pb.ChaincodeID{Path: url1}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID1, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec1)
	chaincodeID1 := spec1.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID1, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/passthru"

	cID2 := &pb.ChaincodeID{Path: url2}
	f = "init"
	args = util.ToChaincodeArgs(f)

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec2)
	chaincodeID2 := spec2.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID2, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn invoke the first chaincode but pass bad params
	f = chaincodeID1
	args = util.ToChaincodeArgs(f, "query", "c")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID2, CtorMsg: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, _, err = invoke(ctxt, spec2, pb.Transaction_CHAINCODE_QUERY)

	if err == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID2, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	if strings.Index(err.Error(), "Nil amount for c") < 0 {
		t.Fail()
		t.Logf("Unexpected error %s", err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
		closeListenerAndSleep(lis)
		return
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec1})
	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec2})
	closeListenerAndSleep(lis)
}

// Test the execution of a chaincode query that queries another chaincode with security enabled
// NOTE: this really needs to be a behave test. Remove when we have support in behave for multiple chaincodes
func TestChaincodeQueryChaincodeWithSec(t *testing.T) {
	testDBWrapper.CleanDB(t)
	viper.Set("security.enabled", "true")

	//Initialize crypto
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed initializing the crypto layer [%s]", err))
	}

	//set paths for memberservice to pick up
	viper.Set("peer.fileSystemPath", filepath.Join(os.TempDir(), "hyperledger", "production"))
	viper.Set("server.rootpath", filepath.Join(os.TempDir(), "ca"))

	var err error
	var memSrvcLis net.Listener
	if memSrvcLis, err = initMemSrvc(); err != nil {
		t.Fail()
		t.Logf("Error registering user  %s", err)
		return
	}

	time.Sleep(2 * time.Second)

	var peerLis net.Listener
	if peerLis, err = initPeer(); err != nil {
		finitMemSrvc(memSrvcLis)
		t.Fail()
		t.Logf("Error registering user  %s", err)
		return
	}

	if err = crypto.RegisterClient("jim", nil, "jim", "6avZQLwcUe9b"); err != nil {
		finitMemSrvc(memSrvcLis)
		finitPeer(peerLis)
		t.Fail()
		t.Logf("Error registering user  %s", err)
		return
	}

	//login as jim and test chaincode-chaincode interaction with security
	if err = chaincodeQueryChaincode("jim"); err != nil {
		finitMemSrvc(memSrvcLis)
		finitPeer(peerLis)
		t.Fail()
		t.Logf("Error executing test %s", err)
		return
	}

	//cleanup
	finitMemSrvc(memSrvcLis)
	finitPeer(peerLis)
}

// Test the invocation of a transaction.
func TestRangeQuery(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/map"
	cID := &pb.ChaincodeID{Path: url}

	f := "init"
	args := util.ToChaincodeArgs(f)

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	// Invoke second chaincode, which will inturn invoke the first chaincode
	f = "keys"
	args = util.ToChaincodeArgs(f)

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(ctxt, spec, pb.Transaction_CHAINCODE_QUERY)

	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}
	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	closeListenerAndSleep(lis)
}

func TestGetEvent(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/eventsender"

	cID := &pb.ChaincodeID{Path: url}
	f := "init"
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(f)}}

	_, err = deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	time.Sleep(time.Second)

	args := util.ToChaincodeArgs("", "i", "am", "satoshi")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}

	var ccevt *pb.ChaincodeEvent
	ccevt, _, _, err = invoke(ctxt, spec, pb.Transaction_CHAINCODE_INVOKE)

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

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	closeListenerAndSleep(lis)
}

// TestGetRows gets large and small rows from a table and tests border conditions (FAB-860)
func TestGetRows(t *testing.T) {
	testDBWrapper.CleanDB(t)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/tmpdb")

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:21212"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		t.Fail()
		t.Logf("Error starting peer listener %s", err)
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(DefaultChain, getPeerEndpoint, false, ccStartupTimeout, nil))

	go grpcServer.Serve(lis)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/largerowsiter"
	cID := &pb.ChaincodeID{Path: url}

	f := "init"
	args := util.ToChaincodeArgs(f, "")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}

	_, err = deploy(ctxt, spec)
	chaincodeID := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	//query using GetRows 1000, ie all rows
	f = "query"
	args = util.ToChaincodeArgs(f, "1000")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}

	var retval []byte
	_, _, retval, err = invoke(ctxt, spec, pb.Transaction_CHAINCODE_QUERY)

	if err != nil || retval == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	if string(retval) != "1000" {
		t.Fail()
		t.Logf("Invalid return value <%s>: %s", chaincodeID, string(retval))
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	//query just 10 rows
	f = "query"
	args = util.ToChaincodeArgs(f, "10")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: cID, CtorMsg: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(ctxt, spec, pb.Transaction_CHAINCODE_QUERY)

	if err != nil || retval == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", chaincodeID, err)
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	if string(retval) != "10" {
		t.Fail()
		t.Logf("Invalid return value <%s>: %s", chaincodeID, string(retval))
		GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		closeListenerAndSleep(lis)
		return
	}

	GetChain(DefaultChain).Stop(ctxt, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	closeListenerAndSleep(lis)
}

func TestMain(m *testing.M) {
	SetupTestConfig()
	os.Exit(m.Run())
}
