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

package crypto

import (
	obc "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"

	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"crypto/rand"

	"runtime"
	"time"

	"github.com/hyperledger/fabric/core/crypto/attributes"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/membersrvc/ca"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type createTxFunc func(t *testing.T) (*obc.Transaction, *obc.Transaction, error)

var (
	validator Peer

	peer Peer

	deployer Client
	invoker  Client

	server *grpc.Server
	aca    *ca.ACA
	eca    *ca.ECA
	tca    *ca.TCA
	tlsca  *ca.TLSCA

	deployTxCreators  []createTxFunc
	executeTxCreators []createTxFunc
	queryTxCreators   []createTxFunc

	ksPwd = []byte("This is a very very very long pw")

	attrs = []string{"company", "position"}
)

func TestMain(m *testing.M) {
	// Setup the test
	setup()

	//Define a map to store the scenarios properties
	properties := make(map[string]interface{})
	ret := 0

	//First scenario with crypto_test.yaml
	ret = runTestsOnScenario(m, properties, "Using crypto_test.yaml properties")
	if ret != 0 {
		os.Exit(ret)
	}

	//Second scenario with multithread
	properties["security.multithreading.enabled"] = "true"
	ret = runTestsOnScenario(m, properties, "Using multithread enabled")
	if ret != 0 {
		os.Exit(ret)
	}

	// Third scenario (repeat the above) now also with 'security.multithreading.multichannel' enabled.
	properties["security.multithreading.multichannel"] = "true"
	ret = runTestsOnScenario(m, properties, "Using multithread + multichannel enabled")
	if ret != 0 {
		os.Exit(ret)
	}
	properties["security.multithreading.enabled"] = "false"

	//Fourth scenario with security level = 384
	properties["security.hashAlgorithm"] = "SHA3"
	properties["security.level"] = "384"
	ret = runTestsOnScenario(m, properties, "Using SHA3-384")
	if ret != 0 {
		os.Exit(ret)
	}

	//Fifth scenario with SHA2
	properties["security.hashAlgorithm"] = "SHA2"
	properties["security.level"] = "256"
	ret = runTestsOnScenario(m, properties, "Using SHA2-256")
	if ret != 0 {
		os.Exit(ret)
	}

	//Sixth scenario with SHA2
	properties["security.hashAlgorithm"] = "SHA2"
	properties["security.level"] = "384"
	ret = runTestsOnScenario(m, properties, "Using SHA2-384")
	if ret != 0 {
		os.Exit(ret)
	}

	os.Exit(ret)
}

//loadConfigScennario loads the properties in the viper and returns the current values.
func loadConfigScenario(properties map[string]interface{}) map[string]interface{} {
	currentValues := make(map[string]interface{})
	for k, v := range properties {
		currentValues[k] = viper.Get(k)
		viper.Set(k, v)
	}
	return currentValues
}

func before() {
	// Init PKI
	initPKI()
	go startPKI()
}

func after() {
	cleanup()
}

func runTestsOnScenario(m *testing.M, properties map[string]interface{}, scenarioName string) int {
	fmt.Printf("=== Start tests for scenario '%v' ===\n", scenarioName)
	currentValues := make(map[string]interface{})
	if len(properties) > 0 {
		currentValues = loadConfigScenario(properties)
	}
	primitives.SetSecurityLevel(viper.GetString("security.hashAlgorithm"), viper.GetInt("security.level"))

	before()
	ret := m.Run()
	after()

	if len(properties) > 0 {
		_ = loadConfigScenario(currentValues)
	}
	fmt.Printf("=== End tests for scenario '%v'  ===\n", scenarioName)
	return ret
}

func TestParallelInitClose(t *testing.T) {
	clientConf := utils.NodeConfiguration{Type: "client", Name: "userthread"}
	peerConf := utils.NodeConfiguration{Type: "peer", Name: "peerthread"}
	validatorConf := utils.NodeConfiguration{Type: "validator", Name: "validatorthread"}

	if err := RegisterClient(clientConf.Name, nil, clientConf.GetEnrollmentID(), clientConf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed registerting userthread.")
	}

	if err := RegisterPeer(peerConf.Name, nil, peerConf.GetEnrollmentID(), peerConf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed registerting peerthread")
	}

	if err := RegisterValidator(validatorConf.Name, nil, validatorConf.GetEnrollmentID(), validatorConf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed registerting validatorthread")
	}

	done := make(chan bool)

	n := 10
	var peer Peer
	var validator Peer
	var client Client

	var err error
	for i := 0; i < n; i++ {
		go func() {
			if err := RegisterPeer(peerConf.Name, nil, peerConf.GetEnrollmentID(), peerConf.GetEnrollmentPWD()); err != nil {
				t.Logf("Failed registerting peerthread")
			}
			peer, err = InitPeer(peerConf.Name, nil)
			if err != nil {
				t.Logf("Failed peer initialization [%s]", err)
			}

			if err := RegisterValidator(validatorConf.Name, nil, validatorConf.GetEnrollmentID(), validatorConf.GetEnrollmentPWD()); err != nil {
				t.Logf("Failed registerting validatorthread")
			}
			validator, err = InitValidator(validatorConf.Name, nil)
			if err != nil {
				t.Logf("Failed validator initialization [%s]", err)
			}

			if err := RegisterClient(clientConf.Name, nil, clientConf.GetEnrollmentID(), clientConf.GetEnrollmentPWD()); err != nil {
				t.Logf("Failed registerting userthread.")
			}
			client, err = InitClient(clientConf.Name, nil)
			if err != nil {
				t.Logf("Failed client initialization [%s]", err)
			}

			for i := 0; i < 5; i++ {
				client, err := InitClient(clientConf.Name, nil)
				if err != nil {
					t.Logf("Failed client initialization [%s]", err)
				}

				runtime.Gosched()
				time.Sleep(500 * time.Millisecond)

				err = CloseClient(client)
				if err != nil {
					t.Logf("Failed client closing [%s]", err)
				}
			}
			done <- true
		}()
	}

	for i := 0; i < n; i++ {
		log.Info("Waiting")
		<-done
		log.Info("+1")
	}

	// Close Client, Peer and Validator n times
	for i := 0; i < n; i++ {
		if err := CloseClient(client); err != nil {
			t.Fatalf("Client should be still closable. [%d][%d]", i, n)
		}

		if err := ClosePeer(peer); err != nil {
			t.Fatalf("Peer should be still closable. [%d][%d]", i, n)
		}

		if err := CloseValidator(validator); err != nil {
			t.Fatalf("Validator should be still closable. [%d][%d]", i, n)
		}
	}
}

func TestRegistrationSameEnrollIDDifferentRole(t *testing.T) {
	conf := utils.NodeConfiguration{Type: "client", Name: "TestRegistrationSameEnrollIDDifferentRole"}
	if err := RegisterClient(conf.Name, nil, conf.GetEnrollmentID(), conf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed client registration [%s]", err)
	}

	if err := RegisterValidator(conf.Name, nil, conf.GetEnrollmentID(), conf.GetEnrollmentPWD()); err == nil {
		t.Fatal("Reusing the same enrollment id must be forbidden", err)
	}

	if err := RegisterPeer(conf.Name, nil, conf.GetEnrollmentID(), conf.GetEnrollmentPWD()); err == nil {
		t.Fatal("Reusing the same enrollment id must be forbidden", err)
	}
}

func TestTLSCertificateDeletion(t *testing.T) {
	conf := utils.NodeConfiguration{Type: "peer", Name: "peer"}

	peer, err := registerAndReturnPeer(conf.Name, nil, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		t.Fatalf("Failed peer registration [%s]", err)
	}

	if peer.ks.certMissing(peer.conf.getTLSCertFilename()) {
		t.Fatal("TLS shouldn't be missing after peer registration")
	}

	if err := peer.deleteTLSCertificate(conf.GetEnrollmentID(), conf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed deleting TLS certificate [%s]", err)
	}

	if !peer.ks.certMissing(peer.conf.getTLSCertFilename()) {
		t.Fatal("TLS certificate should be missing after deletion")
	}
}

func TestRegistrationAfterDeletingTLSCertificate(t *testing.T) {
	conf := utils.NodeConfiguration{Type: "peer", Name: "peer"}

	peer, err := registerAndReturnPeer(conf.Name, nil, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		t.Fatalf("Failed peer registration [%s]", err)
	}

	if err := peer.deleteTLSCertificate(conf.GetEnrollmentID(), conf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed deleting TLS certificate [%s]", err)
	}

	if _, err := registerAndReturnPeer(conf.Name, nil, conf.GetEnrollmentID(), conf.GetEnrollmentPWD()); err != nil {
		t.Fatalf("Failed peer registration [%s]", err)
	}
}

func registerAndReturnPeer(name string, pwd []byte, enrollID, enrollPWD string) (*peerImpl, error) {
	peer := newPeer()
	if err := peer.register(NodePeer, name, pwd, enrollID, enrollPWD, nil); err != nil {
		return nil, err
	}
	if err := peer.close(); err != nil {
		return nil, err
	}
	return peer, nil
}

func TestInitialization(t *testing.T) {
	// Init fake client
	client, err := InitClient("", nil)
	if err == nil || client != nil {
		t.Fatal("Init should fail")
	}
	err = CloseClient(client)
	if err == nil {
		t.Fatal("Close should fail")
	}

	// Init fake peer
	peer, err = InitPeer("", nil)
	if err == nil || peer != nil {
		t.Fatal("Init should fail")
	}
	err = ClosePeer(peer)
	if err == nil {
		t.Fatal("Close should fail")
	}

	// Init fake validator
	validator, err = InitValidator("", nil)
	if err == nil || validator != nil {
		t.Fatal("Init should fail")
	}
	err = CloseValidator(validator)
	if err == nil {
		t.Fatal("Close should fail")
	}
}

func TestClientDeployTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()
	for i, createTx := range deployTxCreators {
		t.Logf("TestClientDeployTransaction with [%d]\n", i)

		_, tx, err := createTx(t)

		if err != nil {
			t.Fatalf("Failed creating deploy transaction [%s].", err)
		}

		if tx == nil {
			t.Fatalf("Result must be different from nil")
		}

		// Check transaction. For test purposes only
		err = deployer.(*clientImpl).checkTransaction(tx)
		if err != nil {
			t.Fatalf("Failed checking transaction [%s].", err)
		}
	}
}

func TestClientExecuteTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()
	for i, createTx := range executeTxCreators {
		t.Logf("TestClientExecuteTransaction with [%d]\n", i)

		_, tx, err := createTx(t)

		if err != nil {
			t.Fatalf("Failed creating deploy transaction [%s].", err)
		}

		if tx == nil {
			t.Fatalf("Result must be different from nil")
		}

		// Check transaction. For test purposes only
		err = invoker.(*clientImpl).checkTransaction(tx)
		if err != nil {
			t.Fatalf("Failed checking transaction [%s].", err)
		}
	}
}

func TestClientQueryTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()
	for i, createTx := range queryTxCreators {
		t.Logf("TestClientQueryTransaction with [%d]\n", i)

		_, tx, err := createTx(t)

		if err != nil {
			t.Fatalf("Failed creating deploy transaction [%s].", err)
		}

		if tx == nil {
			t.Fatalf("Result must be different from nil")
		}

		// Check transaction. For test purposes only
		err = invoker.(*clientImpl).checkTransaction(tx)
		if err != nil {
			t.Fatalf("Failed checking transaction [%s].", err)
		}
	}
}

func TestClientMultiExecuteTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()
	for i := 0; i < 24; i++ {
		_, tx, err := createConfidentialExecuteTransaction(t)

		if err != nil {
			t.Fatalf("Failed creating execute transaction [%s].", err)
		}

		if tx == nil {
			t.Fatalf("Result must be different from nil")
		}

		// Check transaction. For test purposes only
		err = invoker.(*clientImpl).checkTransaction(tx)
		if err != nil {
			t.Fatalf("Failed checking transaction [%s].", err)
		}
	}
}

func TestClientGetNextTCerts(t *testing.T) {

	initNodes()
	defer closeNodes()

	// Some positive flow tests here
	var nCerts int = 1
	for i := 1; i < 3; i++ {
		nCerts *= 10
		t.Logf("Calling GetNextTCerts(%d)", nCerts)
		rvCerts, err := deployer.GetNextTCerts(nCerts)
		if err != nil {
			t.Fatalf("Could not receive %d TCerts", nCerts)
		}
		if len(rvCerts) != nCerts {
			t.Fatalf("Expected exactly '%d' TCerts as a return from GetNextTCert(%d)", nCerts, nCerts)
		}

		for nPos, cert := range rvCerts {
			if cert == nil {
				t.Fatalf("Returned TCert (at position %d) cannot be nil", nPos)
			}
		}
	}

	// Some negative flow tests here
	_, err := deployer.GetNextTCerts(0)
	if err == nil {
		t.Fatalf("Requesting 0 TCerts: expected an error when calling GetNextTCerts(0)")
	}

	_, err = deployer.GetNextTCerts(-1)
	if err == nil {
		t.Fatalf("Requesting -1 TCerts: expected an error when calling GetNextTCerts(-1)")
	}

}

//TestClientGetAttributesFromTCert verifies that the value read from the TCert is the expected value "ACompany".
func TestClientGetAttributesFromTCert(t *testing.T) {
	initNodes()
	defer closeNodes()

	tCerts, err := deployer.GetNextTCerts(1, attrs...)

	if err != nil {
		t.Fatalf("Failed getting TCert by calling GetNextTCerts(1): [%s]", err)
	}
	if tCerts == nil {
		t.Fatalf("TCert should be different from nil")
	}
	if len(tCerts) != 1 {
		t.Fatalf("Expected one TCert returned from GetNextTCerts(1)")
	}

	tcertDER := tCerts[0].GetCertificate().Raw

	if tcertDER == nil {
		t.Fatalf("Cert should be different from nil")
	}
	if len(tcertDER) == 0 {
		t.Fatalf("Cert should have length > 0")
	}

	attributeBytes, err := attributes.GetValueForAttribute("company", tCerts[0].GetPreK0(), tCerts[0].GetCertificate())
	if err != nil {
		t.Fatalf("Error retrieving attribute from TCert: [%s]", err)
	}

	attributeValue := string(attributeBytes[:])

	if attributeValue != "ACompany" {
		t.Fatalf("Wrong attribute retrieved from TCert. Expected [%s], Actual [%s]", "ACompany", attributeValue)
	}
}

func TestClientGetAttributesFromTCertWithUnusedTCerts(t *testing.T) {
	initNodes()
	defer closeNodes()

	_, _ = deployer.GetNextTCerts(1, attrs...)

	CloseAllClients() // Remove client and store unused TCerts.
	initClients()     // Restart fresh client.

	tcerts, err := deployer.GetNextTCerts(1, attrs...)

	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}
	if tcerts == nil {
		t.Fatalf("Returned TCerts slice should be different from nil")
	}
	if tcerts[0] == nil {
		t.Fatalf("Returned TCerts slice's first entry should be different from nil")
	}

	tcertDER := tcerts[0].GetCertificate().Raw

	if tcertDER == nil {
		t.Fatalf("Cert should be different from nil")
	}
	if len(tcertDER) == 0 {
		t.Fatalf("Cert should have length > 0")
	}

	attributeBytes, err := attributes.GetValueForAttribute("company", tcerts[0].GetPreK0(), tcerts[0].GetCertificate())
	if err != nil {
		t.Fatalf("Error retrieving attribute from TCert: [%s]", err)
	}

	attributeValue := string(attributeBytes[:])

	if attributeValue != "ACompany" {
		t.Fatalf("Wrong attribute retrieved from TCert. Expected [%s], Actual [%s]", "ACompany", attributeValue)
	}
}

func TestClientGetTCertHandlerNext(t *testing.T) {
	initNodes()
	defer closeNodes()

	handler, err := deployer.GetTCertificateHandlerNext(attrs...)

	if err != nil {
		t.Fatalf("Failed getting handler: [%s]", err)
	}
	if handler == nil {
		t.Fatalf("Handler should be different from nil")
	}

	certDER := handler.GetCertificate()

	if certDER == nil {
		t.Fatalf("Cert should be different from nil")
	}
	if len(certDER) == 0 {
		t.Fatalf("Cert should have length > 0")
	}
}

func TestClientGetTCertHandlerFromDER(t *testing.T) {
	initNodes()
	defer closeNodes()

	handler, err := deployer.GetTCertificateHandlerNext(attrs...)
	if err != nil {
		t.Fatalf("Failed getting handler: [%s]", err)
	}

	handler2, err := deployer.GetTCertificateHandlerFromDER(handler.GetCertificate())
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}
	if handler == nil {
		t.Fatalf("Handler should be different from nil")
	}
	tCertDER := handler2.GetCertificate()
	if tCertDER == nil {
		t.Fatalf("TCert should be different from nil")
	}
	if len(tCertDER) == 0 {
		t.Fatalf("TCert should have length > 0")
	}

	if !reflect.DeepEqual(handler.GetCertificate(), tCertDER) {
		t.Fatalf("TCerts must be the same")
	}
}

func TestClientTCertHandlerSign(t *testing.T) {
	initNodes()
	defer closeNodes()

	handlerDeployer, err := deployer.GetTCertificateHandlerNext(attrs...)
	if err != nil {
		t.Fatalf("Failed getting handler: [%s]", err)
	}

	msg := []byte("Hello World!!!")
	signature, err := handlerDeployer.Sign(msg)
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}
	if signature == nil || len(signature) == 0 {
		t.Fatalf("Failed getting non-nil signature")
	}

	err = handlerDeployer.Verify(signature, msg)
	if err != nil {
		t.Fatalf("Failed verifying signature: [%s]", err)
	}

	// Check that deployer can reload the cert handler from DER and sign
	handlerDeployer2, err := deployer.GetTCertificateHandlerFromDER(handlerDeployer.GetCertificate())
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}

	msg = []byte("Hello World!!!")
	signature, err = handlerDeployer2.Sign(msg)
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}
	if signature == nil || len(signature) == 0 {
		t.Fatalf("Failed getting non-nil signature")
	}

	err = handlerDeployer2.Verify(signature, msg)
	if err != nil {
		t.Fatalf("Failed verifying signature: [%s]", err)
	}

	// Check that invoker (another party) can verify the signature
	handlerInvoker, err := invoker.GetTCertificateHandlerFromDER(handlerDeployer.GetCertificate())
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}

	err = handlerInvoker.Verify(signature, msg)
	if err != nil {
		t.Fatalf("Failed verifying signature: [%s]", err)
	}

	// Check that invoker cannot sign using a tcert obtained by the deployer
	signature, err = handlerInvoker.Sign(msg)
	if err == nil {
		t.Fatalf("Bob should not be able to use Alice's tcert to sign")
	}
	if signature != nil {
		t.Fatalf("Signature should be nil")
	}
}

func TestClientGetEnrollmentCertHandler(t *testing.T) {
	initNodes()
	defer closeNodes()

	handler, err := deployer.GetEnrollmentCertificateHandler()

	if err != nil {
		t.Fatalf("Failed getting handler: [%s]", err)
	}
	if handler == nil {
		t.Fatalf("Handler should be different from nil")
	}

	certDER := handler.GetCertificate()

	if certDER == nil {
		t.Fatalf("Cert should be different from nil")
	}
	if len(certDER) == 0 {
		t.Fatalf("Cert should have length > 0")
	}
}

func TestClientGetEnrollmentCertHandlerSign(t *testing.T) {
	initNodes()
	defer closeNodes()

	handlerDeployer, err := deployer.GetEnrollmentCertificateHandler()
	if err != nil {
		t.Fatalf("Failed getting handler: [%s]", err)
	}

	msg := []byte("Hello World!!!")
	signature, err := handlerDeployer.Sign(msg)
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}
	if signature == nil || len(signature) == 0 {
		t.Fatalf("Failed getting non-nil signature")
	}

	err = handlerDeployer.Verify(signature, msg)
	if err != nil {
		t.Fatalf("Failed verifying signature: [%s]", err)
	}

	// Check that invoker (another party) can verify the signature
	handlerInvoker, err := invoker.GetEnrollmentCertificateHandler()
	if err != nil {
		t.Fatalf("Failed getting tcert: [%s]", err)
	}

	err = handlerInvoker.Verify(signature, msg)
	if err == nil {
		t.Fatalf("Failed verifying signature: [%s]", err)
	}

}

func TestPeerID(t *testing.T) {
	initNodes()
	defer closeNodes()

	// Verify that any id modification doesn't change
	id := peer.GetID()

	if id == nil {
		t.Fatalf("Id is nil.")
	}

	if len(id) == 0 {
		t.Fatalf("Id length is zero.")
	}

	id[0] = id[0] + 1
	id2 := peer.GetID()
	if id2[0] == id[0] {
		t.Fatalf("Invariant not respected.")
	}
}

func TestPeerDeployTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()

	for i, createTx := range deployTxCreators {
		t.Logf("TestPeerDeployTransaction with [%d]\n", i)

		_, tx, err := createTx(t)
		if err != nil {
			t.Fatalf("TransactionPreValidation: failed creating transaction [%s].", err)
		}

		res, err := peer.TransactionPreValidation(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		res, err = peer.TransactionPreExecution(tx)
		if err != utils.ErrNotImplemented {
			t.Fatalf("Error must be ErrNotImplemented [%s].", err)
		}
		if res != nil {
			t.Fatalf("Result must nil")
		}

		// Test no Cert
		oldCert := tx.Cert
		tx.Cert = nil
		_, err = peer.TransactionPreValidation(tx)
		if err == nil {
			t.Fatalf("Pre Validatiotn should fail. No Cert. %s", err)
		}
		tx.Cert = oldCert

		// Test no Signature
		oldSig := tx.Signature
		tx.Signature = nil
		_, err = peer.TransactionPreValidation(tx)
		if err == nil {
			t.Fatalf("Pre Validatiotn should fail. No Signature. %s", err)
		}
		tx.Signature = oldSig

		// Test Invalid Cert
		oldCert = tx.Cert
		tx.Cert = []byte{0, 1, 2, 3, 4}
		_, err = peer.TransactionPreValidation(tx)
		if err == nil {
			t.Fatalf("Pre Validatiotn should fail. Invalid Cert. %s", err)
		}
		tx.Cert = oldCert

		// Test self signed certificate Cert
		oldCert = tx.Cert
		rawSelfSignedCert, _, err := primitives.NewSelfSignedCert()
		if err != nil {
			t.Fatalf("Failed creating self signed cert [%s]", err)
		}
		tx.Cert = rawSelfSignedCert
		_, err = peer.TransactionPreValidation(tx)
		if err == nil {
			t.Fatalf("Pre Validatiotn should fail. Invalid Cert. %s", err)
		}
		tx.Cert = oldCert

		// Test invalid Signature
		oldSig = tx.Signature
		tx.Signature = []byte{0, 1, 2, 3, 4}
		_, err = peer.TransactionPreValidation(tx)
		if err == nil {
			t.Fatalf("Pre Validatiotn should fail. Invalid Signature. %s", err)
		}
		tx.Signature = oldSig
	}
}

func TestPeerExecuteTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()

	for i, createTx := range executeTxCreators {
		t.Logf("TestPeerExecuteTransaction with [%d]\n", i)

		_, tx, err := createTx(t)
		if err != nil {
			t.Fatalf("TransactionPreValidation: failed creating transaction [%s].", err)
		}

		res, err := peer.TransactionPreValidation(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		res, err = peer.TransactionPreExecution(tx)
		if err != utils.ErrNotImplemented {
			t.Fatalf("Error must be ErrNotImplemented [%s].", err)
		}
		if res != nil {
			t.Fatalf("Result must nil")
		}
	}
}

func TestPeerQueryTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()

	for i, createTx := range queryTxCreators {
		t.Logf("TestPeerQueryTransaction with [%d]\n", i)

		_, tx, err := createTx(t)
		if err != nil {
			t.Fatalf("Failed creating query transaction [%s].", err)
		}

		res, err := peer.TransactionPreValidation(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		res, err = peer.TransactionPreExecution(tx)
		if err != utils.ErrNotImplemented {
			t.Fatalf("Error must be ErrNotImplemented [%s].", err)
		}
		if res != nil {
			t.Fatalf("Result must nil")
		}
	}
}

func TestPeerStateEncryptor(t *testing.T) {
	initNodes()
	defer closeNodes()

	_, deployTx, err := createConfidentialDeployTransaction(t)
	if err != nil {
		t.Fatalf("Failed creating deploy transaction [%s].", err)
	}
	_, invokeTxOne, err := createConfidentialExecuteTransaction(t)
	if err != nil {
		t.Fatalf("Failed creating invoke transaction [%s].", err)
	}

	res, err := peer.GetStateEncryptor(deployTx, invokeTxOne)
	if err != utils.ErrNotImplemented {
		t.Fatalf("Error must be ErrNotImplemented [%s].", err)
	}
	if res != nil {
		t.Fatalf("Result must be nil")
	}
}

func TestPeerSignVerify(t *testing.T) {
	initNodes()
	defer closeNodes()

	msg := []byte("Hello World!!!")
	signature, err := peer.Sign(msg)
	if err != nil {
		t.Fatalf("TestSign: failed generating signature [%s].", err)
	}

	err = peer.Verify(peer.GetID(), signature, msg)
	if err != nil {
		t.Fatalf("TestSign: failed validating signature [%s].", err)
	}

	signature, err = validator.Sign(msg)
	if err != nil {
		t.Fatalf("TestSign: failed generating signature [%s].", err)
	}

	err = peer.Verify(validator.GetID(), signature, msg)
	if err != nil {
		t.Fatalf("TestSign: failed validating signature [%s].", err)
	}
}

func TestPeerVerify(t *testing.T) {
	initNodes()
	defer closeNodes()

	msg := []byte("Hello World!!!")
	signature, err := validator.Sign(msg)
	if err != nil {
		t.Fatalf("Failed generating signature [%s].", err)
	}

	err = peer.Verify(nil, signature, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an empty id.", err)
	}

	err = peer.Verify(msg, signature, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid id.", err)
	}

	err = peer.Verify(validator.GetID(), nil, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid signature.", err)
	}

	err = peer.Verify(validator.GetID(), msg, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid signature.", err)
	}

	err = peer.Verify(validator.GetID(), signature, nil)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid messahe.", err)
	}
}

func TestValidatorID(t *testing.T) {
	initNodes()
	defer closeNodes()

	// Verify that any id modification doesn't change
	id := validator.GetID()

	if id == nil {
		t.Fatalf("Id is nil.")
	}

	if len(id) == 0 {
		t.Fatalf("Id length is zero.")
	}

	id[0] = id[0] + 1
	id2 := validator.GetID()
	if id2[0] == id[0] {
		t.Fatalf("Invariant not respected.")
	}
}

func TestValidatorDeployTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()

	for i, createTx := range deployTxCreators {
		t.Logf("TestValidatorDeployTransaction with [%d]\n", i)

		otx, tx, err := createTx(t)
		if err != nil {
			t.Fatalf("Failed creating deploy transaction [%s].", err)
		}

		res, err := validator.TransactionPreValidation(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		res, err = validator.TransactionPreExecution(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		// Test invalid ConfidentialityLevel
		oldConfidentialityLevel := tx.ConfidentialityLevel
		tx.ConfidentialityLevel = -1
		_, err = validator.TransactionPreExecution(tx)
		if err == nil {
			t.Fatalf("TransactionPreExecution should fail. Invalid ConfidentialityLevel. %s", err)
		}
		if err != utils.ErrInvalidConfidentialityLevel {
			t.Fatalf("TransactionPreExecution should with ErrInvalidConfidentialityLevel rather than [%s]", err)
		}
		tx.ConfidentialityLevel = oldConfidentialityLevel

		if tx.ConfidentialityLevel == obc.ConfidentialityLevel_CONFIDENTIAL {
			if reflect.DeepEqual(res, tx) {
				t.Fatalf("Src and Dest Transaction should be different after PreExecution")
			}
			if err := isEqual(otx, res); err != nil {
				t.Fatalf("Decrypted transaction differs from the original: [%s]", err)
			}

			// Test no ToValidators
			oldToValidators := tx.ToValidators
			tx.ToValidators = nil
			_, err = validator.TransactionPreExecution(tx)
			if err == nil {
				t.Fatalf("TransactionPreExecution should fail. No ToValidators. %s", err)
			}
			tx.ToValidators = oldToValidators

			// Test invalid ToValidators
			oldToValidators = tx.ToValidators
			tx.ToValidators = []byte{0, 1, 2, 3, 4}
			_, err = validator.TransactionPreExecution(tx)
			if err == nil {
				t.Fatalf("TransactionPreExecution should fail. Invalid ToValidators. %s", err)
			}
			tx.ToValidators = oldToValidators

			// Test no Payload
			oldPayload := tx.Payload
			tx.Payload = nil
			_, err = validator.TransactionPreExecution(tx)
			if err == nil {
				t.Fatalf("TransactionPreExecution should fail. No Payload. %s", err)
			}
			tx.Payload = oldPayload

			// Test invalid Payload
			oldPayload = tx.Payload
			tx.Payload = []byte{0, 1, 2, 3, 4}
			_, err = validator.TransactionPreExecution(tx)
			if err == nil {
				t.Fatalf("TransactionPreExecution should fail. Invalid Payload. %s", err)
			}
			tx.Payload = oldPayload

			// Test no Payload
			oldChaincodeID := tx.ChaincodeID
			tx.ChaincodeID = nil
			_, err = validator.TransactionPreExecution(tx)
			if err == nil {
				t.Fatalf("TransactionPreExecution should fail. No ChaincodeID. %s", err)
			}
			tx.ChaincodeID = oldChaincodeID

			// Test invalid Payload
			oldChaincodeID = tx.ChaincodeID
			tx.ChaincodeID = []byte{0, 1, 2, 3, 4}
			_, err = validator.TransactionPreExecution(tx)
			if err == nil {
				t.Fatalf("TransactionPreExecution should fail. Invalid ChaincodeID. %s", err)
			}
			tx.ChaincodeID = oldChaincodeID
		}
	}
}

func TestValidatorExecuteTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()

	for i, createTx := range executeTxCreators {
		t.Logf("TestValidatorExecuteTransaction with [%d]\n", i)

		otx, tx, err := createTx(t)
		if err != nil {
			t.Fatalf("Failed creating execute transaction [%s].", err)
		}

		res, err := validator.TransactionPreValidation(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		res, err = validator.TransactionPreExecution(tx)
		if err != nil {
			t.Fatalf("Error must be nil [%s].", err)
		}
		if res == nil {
			t.Fatalf("Result must be diffrent from nil")
		}

		if tx.ConfidentialityLevel == obc.ConfidentialityLevel_CONFIDENTIAL {
			if reflect.DeepEqual(res, tx) {
				t.Fatalf("Src and Dest Transaction should be different after PreExecution")
			}
			if err := isEqual(otx, res); err != nil {
				t.Fatalf("Decrypted transaction differs from the original: [%s]", err)
			}
		}
	}
}

func TestValidatorQueryTransaction(t *testing.T) {
	initNodes()
	defer closeNodes()

	for i, createTx := range queryTxCreators {
		t.Logf("TestValidatorConfidentialQueryTransaction with [%d]\n", i)

		_, deployTx, err := deployTxCreators[i](t)
		if err != nil {
			t.Fatalf("Failed creating deploy transaction [%s].", err)
		}
		_, invokeTxOne, err := executeTxCreators[i](t)
		if err != nil {
			t.Fatalf("Failed creating invoke transaction [%s].", err)
		}
		_, invokeTxTwo, err := executeTxCreators[i](t)
		if err != nil {
			t.Fatalf("Failed creating invoke transaction [%s].", err)
		}
		otx, queryTx, err := createTx(t)
		if err != nil {
			t.Fatalf("Failed creating query transaction [%s].", err)
		}

		if queryTx.ConfidentialityLevel == obc.ConfidentialityLevel_CONFIDENTIAL {

			// Transactions must be PreExecuted by the validators before getting the StateEncryptor
			if _, err = validator.TransactionPreValidation(deployTx); err != nil {
				t.Fatalf("Failed pre-validating deploty transaction [%s].", err)
			}
			if deployTx, err = validator.TransactionPreExecution(deployTx); err != nil {
				t.Fatalf("Failed pre-executing deploty transaction [%s].", err)
			}
			if _, err = validator.TransactionPreValidation(invokeTxOne); err != nil {
				t.Fatalf("Failed pre-validating exec1 transaction [%s].", err)
			}
			if invokeTxOne, err = validator.TransactionPreExecution(invokeTxOne); err != nil {
				t.Fatalf("Failed pre-executing exec1 transaction [%s].", err)
			}
			if _, err = validator.TransactionPreValidation(invokeTxTwo); err != nil {
				t.Fatalf("Failed pre-validating exec2 transaction [%s].", err)
			}
			if invokeTxTwo, err = validator.TransactionPreExecution(invokeTxTwo); err != nil {
				t.Fatalf("Failed pre-executing exec2 transaction [%s].", err)
			}
			if _, err = validator.TransactionPreValidation(queryTx); err != nil {
				t.Fatalf("Failed pre-validating query transaction [%s].", err)
			}
			if queryTx, err = validator.TransactionPreExecution(queryTx); err != nil {
				t.Fatalf("Failed pre-executing query transaction [%s].", err)
			}
			if err := isEqual(otx, queryTx); err != nil {
				t.Fatalf("Decrypted transaction differs from the original: [%s]", err)
			}

			// First invokeTx
			seOne, err := validator.GetStateEncryptor(deployTx, invokeTxOne)
			if err != nil {
				t.Fatalf("Failed creating state encryptor [%s].", err)
			}
			pt := []byte("Hello World")
			aCt, err := seOne.Encrypt(pt)
			if err != nil {
				t.Fatalf("Failed encrypting state [%s].", err)
			}
			aPt, err := seOne.Decrypt(aCt)
			if err != nil {
				t.Fatalf("Failed decrypting state [%s].", err)
			}
			if !bytes.Equal(pt, aPt) {
				t.Fatalf("Failed decrypting state [%s != %s]: %s", string(pt), string(aPt), err)
			}
			// Try to decrypt nil. It should return nil with no error
			out, err := seOne.Decrypt(nil)
			if err != nil {
				t.Fatal("Decrypt should not fail on nil input")
			}
			if out != nil {
				t.Fatal("Nil input should decrypt to nil")
			}

			// Second invokeTx
			seTwo, err := validator.GetStateEncryptor(deployTx, invokeTxTwo)
			if err != nil {
				t.Fatalf("Failed creating state encryptor [%s].", err)
			}
			aPt2, err := seTwo.Decrypt(aCt)
			if err != nil {
				t.Fatalf("Failed decrypting state [%s].", err)
			}
			if !bytes.Equal(pt, aPt2) {
				t.Fatalf("Failed decrypting state [%s != %s]: %s", string(pt), string(aPt), err)
			}
			// Reencrypt the state
			aCt, err = seTwo.Encrypt(pt)
			if err != nil {
				t.Fatalf("Failed encrypting state [%s].", err)
			}

			// Try to decrypt nil. It should return nil with no error
			out, err = seTwo.Decrypt(nil)
			if err != nil {
				t.Fatal("Decrypt should not fail on nil input")
			}
			if out != nil {
				t.Fatal("Nil input should decrypt to nil")
			}

			// queryTx
			seThree, err := validator.GetStateEncryptor(deployTx, queryTx)
			aPt2, err = seThree.Decrypt(aCt)
			if err != nil {
				t.Fatalf("Failed decrypting state [%s].", err)
			}
			if !bytes.Equal(pt, aPt2) {
				t.Fatalf("Failed decrypting state [%s != %s]: %s", string(pt), string(aPt), err)
			}

			ctQ, err := seThree.Encrypt(aPt2)
			if err != nil {
				t.Fatalf("Failed encrypting query result [%s].", err)
			}
			aPt3, err := invoker.DecryptQueryResult(queryTx, ctQ)
			if err != nil {
				t.Fatalf("Failed decrypting query result [%s].", err)
			}
			if !bytes.Equal(aPt2, aPt3) {
				t.Fatalf("Failed decrypting query result [%s != %s]: %s", string(aPt2), string(aPt3), err)
			}
		}
	}
}

func TestValidatorStateEncryptor(t *testing.T) {
	initNodes()
	defer closeNodes()

	_, deployTx, err := createConfidentialDeployTransaction(t)
	if err != nil {
		t.Fatalf("Failed creating deploy transaction [%s]", err)
	}
	_, invokeTxOne, err := createConfidentialExecuteTransaction(t)
	if err != nil {
		t.Fatalf("Failed creating invoke transaction [%s]", err)
	}
	_, invokeTxTwo, err := createConfidentialExecuteTransaction(t)
	if err != nil {
		t.Fatalf("Failed creating invoke transaction [%s]", err)
	}

	// Transactions must be PreExecuted by the validators before getting the StateEncryptor
	if _, err = validator.TransactionPreValidation(deployTx); err != nil {
		t.Fatalf("Failed pre-validating deploty transaction [%s].", err)
	}
	if deployTx, err = validator.TransactionPreExecution(deployTx); err != nil {
		t.Fatalf("Failed pre-validating deploty transaction [%s].", err)
	}
	if _, err = validator.TransactionPreValidation(invokeTxOne); err != nil {
		t.Fatalf("Failed pre-validating exec1 transaction [%s].", err)
	}
	if invokeTxOne, err = validator.TransactionPreExecution(invokeTxOne); err != nil {
		t.Fatalf("Failed pre-validating exec1 transaction [%s].", err)
	}
	if _, err = validator.TransactionPreValidation(invokeTxTwo); err != nil {
		t.Fatalf("Failed pre-validating exec2 transaction [%s].", err)
	}
	if invokeTxTwo, err = validator.TransactionPreExecution(invokeTxTwo); err != nil {
		t.Fatalf("Failed pre-validating exec2 transaction [%s].", err)
	}

	seOne, err := validator.GetStateEncryptor(deployTx, invokeTxOne)
	if err != nil {
		t.Fatalf("Failed creating state encryptor [%s].", err)
	}
	pt := []byte("Hello World")
	aCt, err := seOne.Encrypt(pt)
	if err != nil {
		t.Fatalf("Failed encrypting state [%s].", err)
	}
	aPt, err := seOne.Decrypt(aCt)
	if err != nil {
		t.Fatalf("Failed decrypting state [%s].", err)
	}
	if !bytes.Equal(pt, aPt) {
		t.Fatalf("Failed decrypting state [%s != %s]: %s", string(pt), string(aPt), err)
	}

	// Try to decrypt nil. It should return nil with no error
	out, err := seOne.Decrypt(nil)
	if err != nil {
		t.Fatal("Decrypt should not fail on nil input")
	}
	if out != nil {
		t.Fatal("Nil input should decrypt to nil")
	}

	seTwo, err := validator.GetStateEncryptor(deployTx, invokeTxTwo)
	if err != nil {
		t.Fatalf("Failed creating state encryptor [%s].", err)
	}
	aPt2, err := seTwo.Decrypt(aCt)
	if err != nil {
		t.Fatalf("Failed decrypting state [%s].", err)
	}
	if !bytes.Equal(pt, aPt2) {
		t.Fatalf("Failed decrypting state [%s != %s]: %s", string(pt), string(aPt), err)
	}

	// Try to decrypt nil. It should return nil with no error
	out, err = seTwo.Decrypt(nil)
	if err != nil {
		t.Fatal("Decrypt should not fail on nil input")
	}
	if out != nil {
		t.Fatal("Nil input should decrypt to nil")
	}

}

func TestValidatorSignVerify(t *testing.T) {
	initNodes()
	defer closeNodes()

	msg := []byte("Hello World!!!")
	signature, err := validator.Sign(msg)
	if err != nil {
		t.Fatalf("TestSign: failed generating signature [%s].", err)
	}

	err = validator.Verify(validator.GetID(), signature, msg)
	if err != nil {
		t.Fatalf("TestSign: failed validating signature [%s].", err)
	}
}

func TestValidatorVerify(t *testing.T) {
	initNodes()
	defer closeNodes()

	msg := []byte("Hello World!!!")
	signature, err := validator.Sign(msg)
	if err != nil {
		t.Fatalf("Failed generating signature [%s].", err)
	}

	err = validator.Verify(nil, signature, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an empty id.", err)
	}

	err = validator.Verify(msg, signature, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid id.", err)
	}

	err = validator.Verify(validator.GetID(), nil, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid signature.", err)
	}

	err = validator.Verify(validator.GetID(), msg, msg)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid signature.", err)
	}

	err = validator.Verify(validator.GetID(), signature, nil)
	if err == nil {
		t.Fatal("Verify should fail when given an invalid messahe.", err)
	}
}

func BenchmarkTransactionCreation(b *testing.B) {
	initNodes()
	defer closeNodes()

	b.StopTimer()
	b.ResetTimer()
	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
	}
	invoker.GetTCertificateHandlerNext(attrs...)

	for i := 0; i < b.N; i++ {
		uuid := util.GenerateUUID()
		b.StartTimer()
		invoker.NewChaincodeExecute(cis, uuid, attrs...)
		b.StopTimer()
	}
}

func BenchmarkTransactionValidation(b *testing.B) {
	initNodes()
	defer closeNodes()

	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, tx, _ := createConfidentialTCertHExecuteTransaction(nil)

		b.StartTimer()
		validator.TransactionPreValidation(tx)
		validator.TransactionPreExecution(tx)
		b.StopTimer()
	}
}

func BenchmarkSign(b *testing.B) {
	b.StopTimer()
	b.ResetTimer()

	//b.Logf("#iterations %d\n", b.N)
	signKey, _ := primitives.NewECDSAKey()
	hash := make([]byte, 48)

	for i := 0; i < b.N; i++ {
		rand.Read(hash)
		b.StartTimer()
		primitives.ECDSASign(signKey, hash)
		b.StopTimer()
	}
}

func BenchmarkVerify(b *testing.B) {
	b.StopTimer()
	b.ResetTimer()

	//b.Logf("#iterations %d\n", b.N)
	signKey, _ := primitives.NewECDSAKey()
	verKey := signKey.PublicKey
	hash := make([]byte, 48)

	for i := 0; i < b.N; i++ {
		rand.Read(hash)
		sigma, _ := primitives.ECDSASign(signKey, hash)
		b.StartTimer()
		primitives.ECDSAVerify(&verKey, hash, sigma)
		b.StopTimer()
	}
}

func setup() {
	// Conf
	viper.SetConfigName("crypto_test") // name of config file (without extension)
	viper.AddConfigPath(".")           // path to look for the config file in
	err := viper.ReadInConfig()        // Find and read the config file
	if err != nil {                    // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file [%s] \n", err))
	}

	// Set Default properties
	viper.Set("peer.fileSystemPath", filepath.Join(os.TempDir(), "obc-crypto-tests", "peers"))
	viper.Set("server.rootpath", filepath.Join(os.TempDir(), "obc-crypto-tests", "ca"))
	viper.Set("peer.pki.tls.rootcert.file", filepath.Join(os.TempDir(), "obc-crypto-tests", "ca", "tlsca.cert"))

	// Logging
	var formatter = logging.MustStringFormatter(
		`%{color}[%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)

	// TX creators
	deployTxCreators = []createTxFunc{
		createPublicDeployTransaction,
		createConfidentialDeployTransaction,
		createConfidentialTCertHDeployTransaction,
		createConfidentialECertHDeployTransaction,
	}
	executeTxCreators = []createTxFunc{
		createPublicExecuteTransaction,
		createConfidentialExecuteTransaction,
		createConfidentialTCertHExecuteTransaction,
		createConfidentialECertHExecuteTransaction,
	}
	queryTxCreators = []createTxFunc{
		createPublicQueryTransaction,
		createConfidentialQueryTransaction,
		createConfidentialTCertHQueryTransaction,
		createConfidentialECertHQueryTransaction,
	}

	// Init crypto layer
	Init()

	// Clenaup folders
	removeFolders()
}

func initPKI() {
	ca.CacheConfiguration() // Need cache the configuration first
	aca = ca.NewACA()
	eca = ca.NewECA(aca)
	tca = ca.NewTCA(eca)
	tlsca = ca.NewTLSCA(eca)
}

func startPKI() {
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
	aca.Start(server)
	eca.Start(server)
	tca.Start(server)
	tlsca.Start(server)

	fmt.Printf("start serving...\n")
	server.Serve(sockp)
}

func initNodes() {
	// Init clients
	err := initClients()
	if err != nil {
		panic(fmt.Errorf("Failed initializing clients [%s].", err))
	}

	// Init peer
	err = initPeers()
	if err != nil {
		panic(fmt.Errorf("Failed initializing peers [%s].", err))
	}

	// Init validators
	err = initValidators()
	if err != nil {
		panic(fmt.Errorf("Failed initializing validators [%s].", err))
	}

}

func closeNodes() {
	ok, errs := CloseAllClients()
	if !ok {
		for _, err := range errs {
			log.Errorf("Failed closing clients [%s]", err)
		}
	}
	ok, errs = CloseAllPeers()
	if !ok {
		for _, err := range errs {
			log.Errorf("Failed closing clients [%s]", err)
		}
	}
	ok, errs = CloseAllValidators()
}

func initClients() error {
	// Deployer
	deployerConf := utils.NodeConfiguration{Type: "client", Name: "user1"}
	if err := RegisterClient(deployerConf.Name, ksPwd, deployerConf.GetEnrollmentID(), deployerConf.GetEnrollmentPWD()); err != nil {
		return err
	}
	var err error
	deployer, err = InitClient(deployerConf.Name, ksPwd)
	if err != nil {
		return err
	}

	// Invoker
	invokerConf := utils.NodeConfiguration{Type: "client", Name: "user2"}
	if err = RegisterClient(invokerConf.Name, ksPwd, invokerConf.GetEnrollmentID(), invokerConf.GetEnrollmentPWD()); err != nil {
		return err
	}
	invoker, err = InitClient(invokerConf.Name, ksPwd)
	if err != nil {
		return err
	}

	return nil
}

func initPeers() error {
	// Register
	conf := utils.NodeConfiguration{Type: "peer", Name: "peer"}
	err := RegisterPeer(conf.Name, ksPwd, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		return err
	}

	// Verify that a second call to Register fails
	err = RegisterPeer(conf.Name, ksPwd, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		return err
	}

	// Init
	peer, err = InitPeer(conf.Name, ksPwd)
	if err != nil {
		return err
	}

	err = RegisterPeer(conf.Name, ksPwd, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		return err
	}

	return err
}

func initValidators() error {
	// Register
	conf := utils.NodeConfiguration{Type: "validator", Name: "validator"}
	err := RegisterValidator(conf.Name, ksPwd, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		return err
	}

	// Verify that a second call to Register fails
	err = RegisterValidator(conf.Name, ksPwd, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		return err
	}

	// Init
	validator, err = InitValidator(conf.Name, ksPwd)
	if err != nil {
		return err
	}

	err = RegisterValidator(conf.Name, ksPwd, conf.GetEnrollmentID(), conf.GetEnrollmentPWD())
	if err != nil {
		return err
	}

	return err
}

func createConfidentialDeployTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cds := &obc.ChaincodeDeploymentSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
			Metadata:             []byte("Hello World"),
		},
		EffectiveDate: nil,
		CodePackage:   nil,
	}

	otx, err := obc.NewChaincodeDeployTransaction(cds, uuid)
	otx.Metadata = cds.ChaincodeSpec.Metadata
	if err != nil {
		return nil, nil, err
	}
	tx, err := deployer.NewChaincodeDeployTransaction(cds, uuid, attrs...)
	return otx, tx, err
}

func createConfidentialExecuteTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
			Metadata:             []byte("Hello World"),
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_INVOKE)
	otx.Metadata = cis.ChaincodeSpec.Metadata
	if err != nil {
		return nil, nil, err
	}
	tx, err := invoker.NewChaincodeExecute(cis, uuid, attrs...)
	return otx, tx, err
}

func createConfidentialQueryTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
			Metadata:             []byte("Hello World"),
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_QUERY)
	otx.Metadata = cis.ChaincodeSpec.Metadata
	if err != nil {
		return nil, nil, err
	}
	tx, err := invoker.NewChaincodeQuery(cis, uuid, attrs...)
	return otx, tx, err
}

func createConfidentialTCertHDeployTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cds := &obc.ChaincodeDeploymentSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
		EffectiveDate: nil,
		CodePackage:   nil,
	}

	otx, err := obc.NewChaincodeDeployTransaction(cds, uuid)
	if err != nil {
		return nil, nil, err
	}
	handler, err := deployer.GetTCertificateHandlerNext(attrs...)
	if err != nil {
		return nil, nil, err
	}
	txHandler, err := handler.GetTransactionHandler()
	if err != nil {
		return nil, nil, err
	}
	tx, err := txHandler.NewChaincodeDeployTransaction(cds, uuid, attrs...)

	// Check binding consistency
	binding, err := txHandler.GetBinding()
	if err != nil {
		t.Fatal("Failed getting binding from transaction handler.")
	}

	txBinding, err := validator.GetTransactionBinding(tx)
	if err != nil {
		t.Fatal("Failed getting transaction binding.")
	}

	if !reflect.DeepEqual(binding, txBinding) {
		t.Fatal("Binding is malformed!")
	}

	// Check confidentiality level
	if tx.ConfidentialityLevel != cds.ChaincodeSpec.ConfidentialityLevel {
		t.Fatal("Failed setting confidentiality level")
	}

	// Check metadata
	if !reflect.DeepEqual(cds.ChaincodeSpec.Metadata, tx.Metadata) {
		t.Fatal("Failed copying metadata")
	}

	return otx, tx, err
}

func createConfidentialTCertHExecuteTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_INVOKE)
	if err != nil {
		return nil, nil, err
	}
	handler, err := invoker.GetTCertificateHandlerNext(attrs...)

	if err != nil {
		return nil, nil, err
	}
	txHandler, err := handler.GetTransactionHandler()
	if err != nil {
		return nil, nil, err
	}
	tx, err := txHandler.NewChaincodeExecute(cis, uuid, attrs...)

	// Check binding consistency
	binding, _ := txHandler.GetBinding()
	if !reflect.DeepEqual(binding, primitives.Hash(append(handler.GetCertificate(), tx.Nonce...))) {
		t.Fatal("Binding is malformed!")
	}

	// Check confidentiality level
	if tx.ConfidentialityLevel != cis.ChaincodeSpec.ConfidentialityLevel {
		t.Fatal("Failed setting confidentiality level")
	}

	// Check metadata
	if !reflect.DeepEqual(cis.ChaincodeSpec.Metadata, tx.Metadata) {
		t.Fatal("Failed copying metadata")
	}

	return otx, tx, err
}

func createConfidentialTCertHQueryTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_QUERY)
	if err != nil {
		return nil, nil, err
	}
	handler, err := invoker.GetTCertificateHandlerNext(attrs...)
	if err != nil {
		return nil, nil, err
	}
	txHandler, err := handler.GetTransactionHandler()
	if err != nil {
		return nil, nil, err
	}
	tx, err := txHandler.NewChaincodeQuery(cis, uuid, attrs...)

	// Check binding consistency
	binding, _ := txHandler.GetBinding()
	if !reflect.DeepEqual(binding, primitives.Hash(append(handler.GetCertificate(), tx.Nonce...))) {
		t.Fatal("Binding is malformed!")
	}

	// Check confidentiality level
	if tx.ConfidentialityLevel != cis.ChaincodeSpec.ConfidentialityLevel {
		t.Fatal("Failed setting confidentiality level")
	}

	// Check metadata
	if !reflect.DeepEqual(cis.ChaincodeSpec.Metadata, tx.Metadata) {
		t.Fatal("Failed copying metadata")
	}

	return otx, tx, err
}

func createConfidentialECertHDeployTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cds := &obc.ChaincodeDeploymentSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
		EffectiveDate: nil,
		CodePackage:   nil,
	}

	otx, err := obc.NewChaincodeDeployTransaction(cds, uuid)
	if err != nil {
		return nil, nil, err
	}
	handler, err := deployer.GetEnrollmentCertificateHandler()
	if err != nil {
		return nil, nil, err
	}
	txHandler, err := handler.GetTransactionHandler()
	if err != nil {
		return nil, nil, err
	}
	tx, err := txHandler.NewChaincodeDeployTransaction(cds, uuid, attrs...)

	// Check binding consistency
	binding, _ := txHandler.GetBinding()
	if !reflect.DeepEqual(binding, primitives.Hash(append(handler.GetCertificate(), tx.Nonce...))) {
		t.Fatal("Binding is malformed!")
	}

	// Check confidentiality level
	if tx.ConfidentialityLevel != cds.ChaincodeSpec.ConfidentialityLevel {
		t.Fatal("Failed setting confidentiality level")
	}

	// Check metadata
	if !reflect.DeepEqual(cds.ChaincodeSpec.Metadata, tx.Metadata) {
		t.Fatal("Failed copying metadata")
	}

	return otx, tx, err
}

func createConfidentialECertHExecuteTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_INVOKE)
	if err != nil {
		return nil, nil, err
	}
	handler, err := invoker.GetEnrollmentCertificateHandler()
	if err != nil {
		return nil, nil, err
	}
	txHandler, err := handler.GetTransactionHandler()
	if err != nil {
		return nil, nil, err
	}
	tx, err := txHandler.NewChaincodeExecute(cis, uuid, attrs...)
	// Check binding consistency
	binding, _ := txHandler.GetBinding()
	if !reflect.DeepEqual(binding, primitives.Hash(append(handler.GetCertificate(), tx.Nonce...))) {
		t.Fatal("Binding is malformed!")
	}

	// Check confidentiality level
	if tx.ConfidentialityLevel != cis.ChaincodeSpec.ConfidentialityLevel {
		t.Fatal("Failed setting confidentiality level")
	}

	// Check metadata
	if !reflect.DeepEqual(cis.ChaincodeSpec.Metadata, tx.Metadata) {
		t.Fatal("Failed copying metadata")
	}

	return otx, tx, err
}

func createConfidentialECertHQueryTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_CONFIDENTIAL,
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_QUERY)
	if err != nil {
		return nil, nil, err
	}
	handler, err := invoker.GetEnrollmentCertificateHandler()
	if err != nil {
		return nil, nil, err
	}
	txHandler, err := handler.GetTransactionHandler()
	if err != nil {
		return nil, nil, err
	}
	tx, err := txHandler.NewChaincodeQuery(cis, uuid, attrs...)
	// Check binding consistency
	binding, _ := txHandler.GetBinding()
	if !reflect.DeepEqual(binding, primitives.Hash(append(handler.GetCertificate(), tx.Nonce...))) {
		t.Fatal("Binding is malformed!")
	}

	// Check confidentiality level
	if tx.ConfidentialityLevel != cis.ChaincodeSpec.ConfidentialityLevel {
		t.Fatal("Failed setting confidentiality level")
	}

	// Check metadata
	if !reflect.DeepEqual(cis.ChaincodeSpec.Metadata, tx.Metadata) {
		t.Fatal("Failed copying metadata")
	}

	return otx, tx, err
}

func createPublicDeployTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cds := &obc.ChaincodeDeploymentSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_PUBLIC,
		},
		EffectiveDate: nil,
		CodePackage:   nil,
	}

	otx, err := obc.NewChaincodeDeployTransaction(cds, uuid)
	if err != nil {
		return nil, nil, err
	}
	tx, err := deployer.NewChaincodeDeployTransaction(cds, uuid, attrs...)
	return otx, tx, err
}

func createPublicExecuteTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_PUBLIC,
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_INVOKE)
	if err != nil {
		return nil, nil, err
	}
	tx, err := invoker.NewChaincodeExecute(cis, uuid, attrs...)
	return otx, tx, err
}

func createPublicQueryTransaction(t *testing.T) (*obc.Transaction, *obc.Transaction, error) {
	uuid := util.GenerateUUID()

	cis := &obc.ChaincodeInvocationSpec{
		ChaincodeSpec: &obc.ChaincodeSpec{
			Type:                 obc.ChaincodeSpec_GOLANG,
			ChaincodeID:          &obc.ChaincodeID{Path: "Contract001"},
			CtorMsg:              nil,
			ConfidentialityLevel: obc.ConfidentialityLevel_PUBLIC,
		},
	}

	otx, err := obc.NewChaincodeExecute(cis, uuid, obc.Transaction_CHAINCODE_QUERY)
	if err != nil {
		return nil, nil, err
	}
	tx, err := invoker.NewChaincodeQuery(cis, uuid, attrs...)
	return otx, tx, err
}

func isEqual(src, dst *obc.Transaction) error {
	if !reflect.DeepEqual(src.Payload, dst.Payload) {
		return fmt.Errorf("Different Payload [%s]!=[%s].", utils.EncodeBase64(src.Payload), utils.EncodeBase64(dst.Payload))
	}

	if !reflect.DeepEqual(src.ChaincodeID, dst.ChaincodeID) {
		return fmt.Errorf("Different ChaincodeID [%s]!=[%s].", utils.EncodeBase64(src.ChaincodeID), utils.EncodeBase64(dst.ChaincodeID))
	}

	if !reflect.DeepEqual(src.Metadata, dst.Metadata) {
		return fmt.Errorf("Different Metadata [%s]!=[%s].", utils.EncodeBase64(src.Metadata), utils.EncodeBase64(dst.Metadata))
	}

	return nil
}

func cleanup() {
	fmt.Println("Cleanup...")
	ok, errs := CloseAllClients()
	if !ok {
		for _, err := range errs {
			log.Errorf("Failed closing clients [%s]", err)
		}
	}
	ok, errs = CloseAllPeers()
	if !ok {
		for _, err := range errs {
			log.Errorf("Failed closing clients [%s]", err)
		}
	}
	ok, errs = CloseAllValidators()
	if !ok {
		for _, err := range errs {
			log.Errorf("Failed closing clients [%s]", err)
		}
	}
	stopPKI()
	removeFolders()
	fmt.Println("Cleanup...done!")
}

func stopPKI() {
	aca.Stop()
	eca.Stop()
	tca.Stop()
	tlsca.Stop()

	server.Stop()
}

func removeFolders() {
	if err := os.RemoveAll(filepath.Join(os.TempDir(), "obc-crypto-tests")); err != nil {
		fmt.Printf("Failed removing [%s] [%s]\n", "obc-crypto-tests", err)
	}

}
