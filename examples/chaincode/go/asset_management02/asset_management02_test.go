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

package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/crypto/attr"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/membersrvc/ca"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

const (
	chaincodeStartupTimeoutDefault int = 5000
)

var (
	testLogger = logging.MustGetLogger("test")

	lis net.Listener

	administrator crypto.Client
	alice         crypto.Client
	bob           crypto.Client

	server *grpc.Server
	aca    *ca.ACA
	eca    *ca.ECA
	tca    *ca.TCA
	tlsca  *ca.TLSCA
)

func TestMain(m *testing.M) {
	removeFolders()
	setup()
	go initMembershipSrvc()

	fmt.Println("Wait for some secs for OBCCA")
	time.Sleep(2 * time.Second)

	go initVP()

	fmt.Println("Wait for some secs for VP")
	time.Sleep(2 * time.Second)

	go initAssetManagementChaincode()

	fmt.Println("Wait for some secs for Chaincode")
	time.Sleep(2 * time.Second)

	if err := initClients(); err != nil {
		panic(err)
	}

	fmt.Println("Wait for 5 secs for chaincode to be started")
	time.Sleep(5 * time.Second)

	ret := m.Run()

	closeListenerAndSleep(lis)

	defer removeFolders()
	os.Exit(ret)
}

//Test if the test chaincode can be succsessfully deployed
func TestChaincodeDeploy(t *testing.T) {
	// Administrator deploy the chaicode
	adminCert, err := administrator.GetTCertificateHandlerNext()
	if err != nil {
		t.Fatal(err)
	}

	if err := deploy(adminCert); err != nil {
		t.Fatal(err)
	}
}

//TestAuthorization tests attribute based role access control by making sure
//only callers with "issuer" role are allowed to invoke the ```assign```
//method to allocate assets to investors.
func TestAuthorization(t *testing.T) {
	// test authorization, alice is not the issuer so she must not be allowed to

	// create certs carring account IDs for Alice
	aliceCert, err := alice.GetTCertificateHandlerNext("role", "account1", "contactInfo")
	if err != nil {
		t.Fatal(err)
	}

	//assign assets (This must fail)
	if err := assignOwnership(alice, aliceCert, "account1", "1000"); err == nil {
		t.Fatal("Alice doesn't have the assigner role. Assignment should fail.")
	} else {
		fmt.Println(err)
		fmt.Println("------------------------------------------------------------")
		fmt.Println("------------------------------------------------------------")
		fmt.Println("------------------------------PASS -------------------------")
		fmt.Println("------------------------------------------------------------")
		fmt.Println("------------------------------------------------------------")
	}
}

// TestAssigningAssets the tests the ```assign``` method by making sure
// authorized users (callers with 'issuer' role) can use the ```assign```
// method to allocate assets to its investors
func TestAssigningAssets(t *testing.T) {

	// create certs carring account IDs for Alice and Bob
	aliceCert1, err := alice.GetTCertificateHandlerNext("role", "account1", "contactInfo")
	if err != nil {
		t.Fatal(err)
	}

	aliceCert2, err := alice.GetTCertificateHandlerNext("role", "account2", "contactInfo")
	if err != nil {
		t.Fatal(err)
	}

	aliceCert3, err := alice.GetTCertificateHandlerNext("role", "account3", "contactInfo")
	if err != nil {
		t.Fatal(err)
	}

	bobCert1, err := bob.GetTCertificateHandlerNext("role", "account1", "contactInfo")
	if err != nil {
		t.Fatal(err)
	}

	//issuer assign balances to assets to first batch of owners
	if err := assignOwnership(administrator, aliceCert1, "account1", "100"); err != nil {
		t.Fatal(err)
	}

	if err := assignOwnership(administrator, aliceCert2, "account2", "200"); err != nil {
		t.Fatal(err)
	}

	if err := assignOwnership(administrator, aliceCert3, "account3", "300"); err != nil {
		t.Fatal(err)
	}

	if err := assignOwnership(administrator, bobCert1, "account1", "1000"); err != nil {
		t.Fatal(err)
	}

	aliceAccountID1, err := attr.GetValueFrom("account1", aliceCert1.GetCertificate())
	aliceAccountID2, err := attr.GetValueFrom("account2", aliceCert2.GetCertificate())
	aliceAccountID3, err := attr.GetValueFrom("account3", aliceCert3.GetCertificate())
	bobAccountID1, err := attr.GetValueFrom("account1", bobCert1.GetCertificate())

	// Check if balances are assigned correctly
	alice1BalanceRaw, err := getBalance(string(aliceAccountID1))
	if err != nil {
		t.Fatal(err)
	}

	alicebalance1 := binary.BigEndian.Uint64(alice1BalanceRaw)
	if alicebalance1 != 100 {
		t.Fatal("retreived balance does not equal to 100")
	}

	alice2BalanceRaw, err := getBalance(string(aliceAccountID2))
	if err != nil {
		t.Fatal(err)
	}

	alicebalance2 := binary.BigEndian.Uint64(alice2BalanceRaw)
	if alicebalance2 != 200 {
		t.Fatal("retreived balance does not equal to 200")
	}

	alice3BalanceRaw, err := getBalance(string(aliceAccountID3))
	if err != nil {
		t.Fatal(err)
	}

	alicebalance3 := binary.BigEndian.Uint64(alice3BalanceRaw)
	if alicebalance3 != 300 {
		t.Fatal("retreived balance does not equal to 300")
	}

	bob1BalanceRaw, err := getBalance(string(bobAccountID1))
	if err != nil {
		t.Fatal(err)
	}

	bobbalance1 := binary.BigEndian.Uint64(bob1BalanceRaw)
	if bobbalance1 != 1000 {
		t.Fatal("retreived balance does not equal to 1000")
	}

	//check if contact info is correctly saved into the chaincode state ledger
	aliceContactInfo, err := getOwnerContactInformation(string(aliceAccountID1))
	if err != nil {
		t.Fatal(err)
	}
	if string(aliceContactInfo) != "alice@gmail.com" {
		t.Fatal("retreived contact info does not equal to alice@gmail.com")
	}

	bobContactInfo, err := getOwnerContactInformation(string(bobAccountID1))
	if err != nil {
		t.Fatal(err)
	}

	if string(bobContactInfo) != "bob@yahoo.com" {
		t.Fatal("retreived contact info does not equal to bob@gmail.com")
	}
}

//test the ability to transfer assets from owner account IDs to new owner account ID
func TestAssetTransfer(t *testing.T) {

	//test transfer
	// create a new cert for alice used to transfer assets
	// (note this cert include multiple account Ids belong to alice)
	aliceCert, err := alice.GetTCertificateHandlerNext("role", "account1", "account2", "account3")
	if err != nil {
		t.Fatal(err)
	}

	// create a new cert for bob to recieve transfer from Alice
	// note "account2" is a new account ID that was never used before. Since this new account ID
	// will create a new record on the account ledger, you must also pass in all required parameters
	// required to create a new account record in chaincode state (such as user contact info)
	bobCert, err := bob.GetTCertificateHandlerNext("role", "account2", "contactInfo")
	if err != nil {
		t.Fatal(err)
	}

	//transfer 200 assets from Alice to Bob
	if nil != transferOwnership(alice, aliceCert, "account1,account2,account3", bobCert, "account2", "200") {
		t.Fatal(err)
	}

	/***********************
		Codes below check if assets are correctly transfered, first find the actual
		account IDs from cert attributes, then call the getBalance method on the
		chaincode for each account ID
	***********************/

	// check that 200 assets have been transfered to Bob; first step is to collect account Ids
	aliceAccountID1, err := attr.GetValueFrom("account1", aliceCert.GetCertificate())
	aliceAccountID2, err := attr.GetValueFrom("account2", aliceCert.GetCertificate())
	aliceAccountID3, err := attr.GetValueFrom("account3", aliceCert.GetCertificate())
	bobAccountID2, err := attr.GetValueFrom("account2", bobCert.GetCertificate())

	//account1 of alice shouldn't have any balance left, and the account should have been
	//deleted from the asset depository
	alice1BalanceRaw, err := getBalance(string(aliceAccountID1))
	fmt.Println("alice balance", alice1BalanceRaw)

	if alice1BalanceRaw != nil {
		t.Fatal(err)
	}
	fmt.Println("alice balance", alice1BalanceRaw)

	// account2 of alice should still have 100 left on its balance
	alice2BalanceRaw, err := getBalance(string(aliceAccountID2))
	if err != nil {
		t.Fatal(err)
	}

	alicebalance2 := binary.BigEndian.Uint64(alice2BalanceRaw)
	if alicebalance2 != 100 {
		t.Fatal("retreived balance for account1 (alice) does not equal to 100")
	}

	// account3 of alice should still have 300 left on its balance
	alice3BalanceRaw, err := getBalance(string(aliceAccountID3))
	if err != nil {
		t.Fatal(err)
	}

	alicebalance3 := binary.BigEndian.Uint64(alice3BalanceRaw)
	if alicebalance3 != 300 {
		t.Fatal("retreived balance for account1 (alice) does not equal to 300")
	}

	// account2 of bob should now have 200 on its balance
	bobBalanceRaw, err := getBalance(string(bobAccountID2))
	if err != nil {
		t.Fatal(err)
	}

	bobbalance := binary.BigEndian.Uint64(bobBalanceRaw)
	if bobbalance != 200 {
		t.Fatal("retreived balance for account2 (bob) does not equal to 200")
	}

}

func deploy(admCert crypto.CertificateHandler) error {
	// Prepare the spec. The metadata includes the role of the users allowed to assign assets
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              &pb.ChaincodeInput{Args: util.ToChaincodeArgs("init")},
		Metadata:             []byte("issuer"),
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	// First build and get the deployment spec
	var ctx = context.Background()
	chaincodeDeploymentSpec, err := getDeploymentSpec(ctx, spec)
	if err != nil {
		return err
	}

	tid := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeDeployTransaction(chaincodeDeploymentSpec, tid)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	_, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return err
}

func assignOwnership(assigner crypto.Client, newOwnerCert crypto.CertificateHandler, attributeName string, amount string) error {
	// Get a transaction handler to be used to submit the execute transaction
	// and bind the chaincode access control logic using the binding
	submittingCertHandler, err := assigner.GetTCertificateHandlerNext("role")
	if err != nil {
		return err
	}
	txHandler, err := submittingCertHandler.GetTransactionHandler()
	if err != nil {
		return err
	}

	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs("assignOwnership", base64.StdEncoding.EncodeToString(newOwnerCert.GetCertificate()), attributeName, amount)}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	_, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return err
}

func transferOwnership(owner crypto.Client, ownerCert crypto.CertificateHandler, fromAttributes string,
	newOwnerCert crypto.CertificateHandler, toAttributes string, amount string) error {
	// Get a transaction handler to be used to submit the execute transaction
	// and bind the chaincode access control logic using the binding

	submittingCertHandler, err := owner.GetTCertificateHandlerNext("role")
	if err != nil {
		return err
	}
	txHandler, err := submittingCertHandler.GetTransactionHandler()
	if err != nil {
		return err
	}

	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs(
		"transferOwnership",
		base64.StdEncoding.EncodeToString(ownerCert.GetCertificate()),
		fromAttributes,
		base64.StdEncoding.EncodeToString(newOwnerCert.GetCertificate()),
		toAttributes,
		amount)}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, tid)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	_, _, err = chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return err

}

func getOwnerContactInformation(accountID string) ([]byte, error) {
	return Query("getOwnerContactInformation", accountID)
}

func getBalance(accountID string) ([]byte, error) {
	return Query("getBalance", accountID)
}

func Query(function, accountID string) ([]byte, error) {
	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs(function, accountID)}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := administrator.NewChaincodeQuery(chaincodeInvocationSpec, tid)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	ledger, err := ledger.GetLedger()
	ledger.BeginTxBatch("1")
	result, _, err := chaincode.Execute(ctx, chaincode.GetChain(chaincode.DefaultChain), transaction)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s", err)
	}
	ledger.CommitTxBatch("1", []*pb.Transaction{transaction}, nil, nil)

	return result, err
}

func setup() {
	// Conf
	viper.SetConfigName("asset") // name of config file (without extension)
	viper.AddConfigPath(".")     // path to look for the config file in
	err := viper.ReadInConfig()  // Find and read the config file
	if err != nil {              // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file [%s] \n", err))
	}

	// Logging
	var formatter = logging.MustStringFormatter(
		`%{color}[%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)

	logging.SetLevel(logging.DEBUG, "peer")
	logging.SetLevel(logging.DEBUG, "chaincode")
	logging.SetLevel(logging.DEBUG, "cryptochain")

	// Init the crypto layer
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed initializing the crypto layer [%s]", err))
	}

	removeFolders()
}

func initMembershipSrvc() {
	// ca.LogInit seems to have been removed
	//ca.LogInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr, os.Stdout)
	ca.CacheConfiguration() // Cache configuration
	aca = ca.NewACA()
	eca = ca.NewECA(aca)
	tca = ca.NewTCA(eca)
	tlsca = ca.NewTLSCA(eca)

	var opts []grpc.ServerOption
	if viper.GetBool("peer.pki.tls.enabled") {
		// TLS configuration
		creds, err := credentials.NewServerTLSFromFile(
			filepath.Join(viper.GetString("server.rootpath"), "tlsca.cert"),
			filepath.Join(viper.GetString("server.rootpath"), "tlsca.priv"),
		)
		if err != nil {
			panic("Failed creating credentials for Membersrvc: " + err.Error())
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	fmt.Printf("open socket...\n")
	sockp, err := net.Listen("tcp", viper.GetString("server.port"))
	if err != nil {
		panic("Cannot open port: " + err.Error())
	}
	fmt.Printf("open socket...done\n")

	server = grpc.NewServer(opts...)

	aca.Start(server)
	eca.Start(server)
	tca.Start(server)
	tlsca.Start(server)

	fmt.Printf("start serving...\n")
	server.Serve(sockp)
}

func initVP() {
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	//lis, err := net.Listen("tcp", viper.GetString("peer.address"))

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress := "0.0.0.0:40404"
	var err error
	lis, err = net.Listen("tcp", peerAddress)
	if err != nil {
		return
	}

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(chaincodeStartupTimeoutDefault) * time.Millisecond
	userRunsCC := true

	// Install security object for peer
	var secHelper crypto.Peer
	if viper.GetBool("security.enabled") {
		enrollID := viper.GetString("security.enrollID")
		enrollSecret := viper.GetString("security.enrollSecret")
		var err error

		if viper.GetBool("peer.validator.enabled") {
			testLogger.Debugf("Registering validator with enroll ID: %s", enrollID)
			if err = crypto.RegisterValidator(enrollID, nil, enrollID, enrollSecret); nil != err {
				panic(err)
			}
			testLogger.Debugf("Initializing validator with enroll ID: %s", enrollID)
			secHelper, err = crypto.InitValidator(enrollID, nil)
			if nil != err {
				panic(err)
			}
		} else {
			testLogger.Debugf("Registering non-validator with enroll ID: %s", enrollID)
			if err = crypto.RegisterPeer(enrollID, nil, enrollID, enrollSecret); nil != err {
				panic(err)
			}
			testLogger.Debugf("Initializing non-validator with enroll ID: %s", enrollID)
			secHelper, err = crypto.InitPeer(enrollID, nil)
			if nil != err {
				panic(err)
			}
		}
	}

	pb.RegisterChaincodeSupportServer(grpcServer,
		chaincode.NewChaincodeSupport(chaincode.DefaultChain, getPeerEndpoint, userRunsCC,
			ccStartupTimeout, secHelper))

	grpcServer.Serve(lis)
}

func initAssetManagementChaincode() {
	err := shim.Start(new(AssetManagementChaincode))
	if err != nil {
		panic(err)
	}
}

func initClients() error {
	// Administrator
	if err := crypto.RegisterClient("admin", nil, "admin", "6avZQLwcUe9b"); err != nil {
		return err
	}
	var err error
	administrator, err = crypto.InitClient("admin", nil)
	if err != nil {
		return err
	}

	// Alice
	if err := crypto.RegisterClient("alice", nil, "alice", "NPKYL39uKbkj"); err != nil {
		return err
	}
	alice, err = crypto.InitClient("alice", nil)
	if err != nil {
		return err
	}

	// Bob
	if err := crypto.RegisterClient("bob", nil, "bob", "DRJ23pEQl16a"); err != nil {
		return err
	}
	bob, err = crypto.InitClient("bob", nil)
	if err != nil {
		return err
	}

	return nil
}

func closeListenerAndSleep(l net.Listener) {
	l.Close()
	time.Sleep(2 * time.Second)
}

func getDeploymentSpec(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	fmt.Printf("getting deployment spec for chaincode spec: %v\n", spec)
	var codePackageBytes []byte
	//if we have a name, we don't need to deploy (we are in userRunsCC mode)
	if spec.ChaincodeID.Name == "" {
		var err error
		codePackageBytes, err = container.GetChaincodePackageBytes(spec)
		if err != nil {
			return nil, err
		}
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func removeFolders() {
	fmt.Println("-------------------------")
	if err := os.RemoveAll(viper.GetString("peer.fileSystemPath")); err != nil {
		fmt.Printf("Failed removing [%s] [%s]\n", "hyperledger", err)
	}
}
