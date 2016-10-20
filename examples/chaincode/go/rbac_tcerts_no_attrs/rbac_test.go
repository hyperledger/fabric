/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package main

import (
	"fmt"
	"net"
	"testing"
	"time"

	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
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

	"encoding/asn1"
	"reflect"
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
	charlie       crypto.Client

	server *grpc.Server
	eca    *ca.ECA
	tca    *ca.TCA
	tlsca  *ca.TLSCA
)

func TestMain(m *testing.M) {
	removeFolders()
	setup()
	go initMemershipServices()

	fmt.Println("Wait for some secs for OBCCA")
	time.Sleep(2 * time.Second)

	go initVP()

	fmt.Println("Wait for some secs for VP")
	time.Sleep(2 * time.Second)

	go initChaincode()

	fmt.Println("Wait for some secs for Chaincode")
	time.Sleep(2 * time.Second)

	if err := initClients(); err != nil {
		panic(err)
	}

	fmt.Println("Wait for 10 secs for chaincode to be started")
	time.Sleep(10 * time.Second)

	ret := m.Run()

	closeListenerAndSleep(lis)

	removeFolders()
	os.Exit(ret)
}

func TestChaincode(t *testing.T) {
	// Administrator deploy the chaincode
	adminCert, err := administrator.GetTCertificateHandlerNext()
	if err != nil {
		t.Fatal(err)
	}

	if err := deploy(adminCert); err != nil {
		t.Fatal(err)
	}

	// Administrator assign role 'writer' to Alice
	aliceCert, err := alice.GetTCertificateHandlerNext()
	if err != nil {
		t.Fatal(err)
	}

	// This must fail
	if err := addRole(aliceCert, aliceCert, "writer"); err == nil {
		t.Fatal(err)
	}

	// This must succeed
	if err := addRole(adminCert, aliceCert, "writer"); err != nil {
		t.Fatal(err)
	}

	// Administrator assign role 'reader' to Bob
	bobCert, err := bob.GetTCertificateHandlerNext()
	if err != nil {
		t.Fatal(err)
	}

	// This must fail
	if err := addRole(bobCert, bobCert, "reader"); err == nil {
		t.Fatal(err)
	}

	// This must succeed
	if err := addRole(adminCert, bobCert, "reader"); err != nil {
		t.Fatal(err)
	}

	// Alice writes
	wValue := []byte("a")
	if err := write(alice, aliceCert, wValue); err != nil {
		t.Fatal(err)
	}

	// Bob reads
	var rValue []byte
	if rValue, err = read(bob, bobCert); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wValue, rValue) {
		t.Fatal(fmt.Sprintf("Read value is different from what written [%s]!=[%s]", string(wValue), string(rValue)))
	}

	// Alice tries to read
	if _, err = read(alice, aliceCert); err == nil {
		t.Fatal(err)
	}

	// Bob tries to write
	if err = write(bob, bobCert, []byte("b")); err == nil {
		t.Fatal(err)
	}
}

func deploy(admCert crypto.CertificateHandler) error {
	// Prepare the spec. The metadata includes the identity of the administrator
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              &pb.ChaincodeInput{Args: util.ToChaincodeArgs("init")},
		Metadata:             admCert.GetCertificate(),
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

func addRole(admCert crypto.CertificateHandler, idCert crypto.CertificateHandler, role string) error {
	// Get a transaction handler to be used to submit the execute transaction
	// and bind the chaincode access control logic using the binding
	submittingCertHandler, err := administrator.GetTCertificateHandlerNext()
	if err != nil {
		return err
	}
	txHandler, err := submittingCertHandler.GetTransactionHandler()
	if err != nil {
		return err
	}
	binding, err := txHandler.GetBinding()
	if err != nil {
		return err
	}

	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs("addRole", string(idCert.GetCertificate()), role)}
	chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
	if err != nil {
		return err
	}

	// Access control:
	// admCert signs admCert.GetCertificate() || chaincodeInputRaw || binding to confirm his identity
	sigma, err := admCert.Sign(append(admCert.GetCertificate(), append(chaincodeInputRaw, binding...)...))
	if err != nil {
		return err
	}

	rbacMetadata := RBACMetadata{admCert.GetCertificate(), sigma}
	rbacMetadataRaw, err := asn1.Marshal(rbacMetadata)
	if err != nil {
		return err
	}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		Metadata:             rbacMetadataRaw, // Proof of identity
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

func write(invoker crypto.Client, invokerCert crypto.CertificateHandler, value []byte) error {
	// Get a transaction handler to be used to submit the execute transaction
	// and bind the chaincode access control logic using the binding

	submittingCertHandler, err := invoker.GetTCertificateHandlerNext()
	if err != nil {
		return err
	}
	txHandler, err := submittingCertHandler.GetTransactionHandler()
	if err != nil {
		return err
	}
	binding, err := txHandler.GetBinding()
	if err != nil {
		return err
	}

	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs("write", string(value))}
	chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
	if err != nil {
		return err
	}

	// Access control:
	// invokerCert signs invokerCert.GetCertificate() || chaincodeInputRaw || binding to confirm his identity
	sigma, err := invokerCert.Sign(append(invokerCert.GetCertificate(), append(chaincodeInputRaw, binding...)...))
	if err != nil {
		return err
	}

	rbacMetadata := RBACMetadata{invokerCert.GetCertificate(), sigma}
	rbacMetadataRaw, err := asn1.Marshal(rbacMetadata)
	if err != nil {
		return err
	}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		Metadata:             rbacMetadataRaw,
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

func read(invoker crypto.Client, invokerCert crypto.CertificateHandler) ([]byte, error) {
	// Get a transaction handler to be used to submit the query transaction
	// and bind the chaincode access control logic using the binding

	submittingCertHandler, err := invoker.GetTCertificateHandlerNext()
	if err != nil {
		return nil, err
	}
	txHandler, err := submittingCertHandler.GetTransactionHandler()
	if err != nil {
		return nil, err
	}
	binding, err := txHandler.GetBinding()
	if err != nil {
		return nil, err
	}

	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs("read")}
	chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
	if err != nil {
		return nil, err
	}

	// Access control:
	// invokerCert signs invokerCert.GetCertificate() || chaincodeInputRaw || binding to confirm his identity
	sigma, err := invokerCert.Sign(append(invokerCert.GetCertificate(), append(chaincodeInputRaw, binding...)...))
	if err != nil {
		return nil, err
	}

	rbacMetadata := RBACMetadata{invokerCert.GetCertificate(), sigma}
	rbacMetadataRaw, err := asn1.Marshal(rbacMetadata)
	if err != nil {
		return nil, err
	}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: "mycc"},
		CtorMsg:              chaincodeInput,
		Metadata:             rbacMetadataRaw,
		ConfidentialityLevel: pb.ConfidentialityLevel_PUBLIC,
	}

	var ctx = context.Background()
	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	tid := chaincodeInvocationSpec.ChaincodeSpec.ChaincodeID.Name

	// Now create the Transactions message and send to Peer.
	transaction, err := txHandler.NewChaincodeQuery(chaincodeInvocationSpec, tid)
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
	viper.SetConfigName("rbac") // name of config file (without extension)
	viper.AddConfigPath(".")    // path to look for the config file in
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file [%s] \n", err))
	}

	// Logging
	var formatter = logging.MustStringFormatter(
		`%{color}[%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)

	logging.SetLevel(logging.DEBUG, "peer")
	logging.SetLevel(logging.DEBUG, "chaincode")
	logging.SetLevel(logging.DEBUG, "cryptoain")

	// Init the crypto layer
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed initializing the crypto layer [%s]", err))
	}

	hl := filepath.Join(os.TempDir(), "hyperledger")

	viper.Set("peer.fileSystemPath", filepath.Join(hl, "production"))
	viper.Set("server.rootpath", filepath.Join(hl, "ca"))

	removeFolders()
}

func initMemershipServices() {
	ca.CacheConfiguration() // Cache configuration
	eca = ca.NewECA(nil)
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
			panic("Failed creating credentials for OBC-CA: " + err.Error())
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
			testLogger.Debug("Registering validator with enroll ID: %s", enrollID)
			if err = crypto.RegisterValidator(enrollID, nil, enrollID, enrollSecret); nil != err {
				panic(err)
			}
			testLogger.Debug("Initializing validator with enroll ID: %s", enrollID)
			secHelper, err = crypto.InitValidator(enrollID, nil)
			if nil != err {
				panic(err)
			}
		} else {
			testLogger.Debug("Registering non-validator with enroll ID: %s", enrollID)
			if err = crypto.RegisterPeer(enrollID, nil, enrollID, enrollSecret); nil != err {
				panic(err)
			}
			testLogger.Debug("Initializing non-validator with enroll ID: %s", enrollID)
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

func initChaincode() {
	err := shim.Start(new(RBACChaincode))
	if err != nil {
		panic(err)
	}
}

func initClients() error {
	// Administrator
	if err := crypto.RegisterClient("jim", nil, "jim", "6avZQLwcUe9b"); err != nil {
		return err
	}
	var err error
	administrator, err = crypto.InitClient("jim", nil)
	if err != nil {
		return err
	}

	// Alice
	if err := crypto.RegisterClient("lukas", nil, "lukas", "NPKYL39uKbkj"); err != nil {
		return err
	}
	alice, err = crypto.InitClient("lukas", nil)
	if err != nil {
		return err
	}

	// Bob
	if err := crypto.RegisterClient("diego", nil, "diego", "DRJ23pEQl16a"); err != nil {
		return err
	}
	bob, err = crypto.InitClient("diego", nil)
	if err != nil {
		return err
	}

	// Charlie
	if err := crypto.RegisterClient("charlie", nil, "charlie", "eriovioh309v"); err != nil {
		return err
	}
	charlie, err = crypto.InitClient("charlie", nil)
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
	if err := os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger")); err != nil {
		fmt.Printf("Failed removing [%s] [%s]\n", "heperledger", err)
	}
}
