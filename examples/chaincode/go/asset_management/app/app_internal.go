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
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

var (
	confidentialityOn bool

	confidentialityLevel pb.ConfidentialityLevel
	chaincodeName        string
)

func initNVP() (err error) {
	if err = initPeerClient(); err != nil {
		appLogger.Debugf("Failed deploying [%s]", err)
		return

	}
	if err = initCryptoClients(); err != nil {
		appLogger.Debugf("Failed deploying [%s]", err)
		return
	}

	return
}

func initPeerClient() (err error) {
	config.SetupTestConfig(".")
	viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
	viper.Set("peer.validator.validity-period.verification", "false")

	peerClientConn, err = peer.NewPeerClientConnection()
	if err != nil {
		fmt.Printf("error connection to server at host:port = %s\n", viper.GetString("peer.address"))
		return
	}
	serverClient = pb.NewPeerClient(peerClientConn)

	// Logging
	var formatter = logging.MustStringFormatter(
		`%{color}[%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)

	return
}

func initCryptoClients() error {
	crypto.Init()

	// Initialize the clients mapping alice, bob, charlie and dave
	// to identities already defined in 'membersrvc.yaml'

	// Alice as jim
	if err := crypto.RegisterClient("jim", nil, "jim", "6avZQLwcUe9b"); err != nil {
		return err
	}
	var err error
	alice, err = crypto.InitClient("jim", nil)
	if err != nil {
		return err
	}

	// Bob as lukas
	if err := crypto.RegisterClient("lukas", nil, "lukas", "NPKYL39uKbkj"); err != nil {
		return err
	}
	bob, err = crypto.InitClient("lukas", nil)
	if err != nil {
		return err
	}

	// Charlie
	if err := crypto.RegisterClient("diego", nil, "diego", "DRJ23pEQl16a"); err != nil {
		return err
	}
	charlie, err = crypto.InitClient("diego", nil)
	if err != nil {
		return err
	}

	// Dave as binhn
	if err := crypto.RegisterClient("binhn", nil, "binhn", "7avZQLwcUe9q"); err != nil {
		return err
	}
	dave, err = crypto.InitClient("binhn", nil)
	if err != nil {
		return err
	}

	return nil
}

func processTransaction(tx *pb.Transaction) (*pb.Response, error) {
	return serverClient.ProcessTransaction(context.Background(), tx)
}

func confidentiality(enabled bool) {
	confidentialityOn = enabled

	if confidentialityOn {
		confidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
	} else {
		confidentialityLevel = pb.ConfidentialityLevel_PUBLIC
	}
}

func deployInternal(deployer crypto.Client, adminCert crypto.CertificateHandler) (resp *pb.Response, err error) {
	// Prepare the spec. The metadata includes the identity of the administrator
	spec := &pb.ChaincodeSpec{
		Type:        1,
		ChaincodeID: &pb.ChaincodeID{Path: "github.com/hyperledger/fabric/examples/chaincode/go/asset_management"},
		//ChaincodeID:          &pb.ChaincodeID{Name: chaincodeName},
		CtorMsg:              &pb.ChaincodeInput{Args: util.ToChaincodeArgs("init")},
		Metadata:             adminCert.GetCertificate(),
		ConfidentialityLevel: confidentialityLevel,
	}

	// First build the deployment spec
	cds, err := getChaincodeBytes(spec)
	if err != nil {
		return nil, fmt.Errorf("Error getting deployment spec: %s ", err)
	}

	// Now create the Transactions message and send to Peer.
	transaction, err := deployer.NewChaincodeDeployTransaction(cds, cds.ChaincodeSpec.ChaincodeID.Name)
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	resp, err = processTransaction(transaction)

	appLogger.Debugf("resp [%s]", resp.String())

	chaincodeName = cds.ChaincodeSpec.ChaincodeID.Name
	appLogger.Debugf("ChaincodeName [%s]", chaincodeName)

	return
}

func assignOwnershipInternal(invoker crypto.Client, invokerCert crypto.CertificateHandler, asset string, newOwnerCert crypto.CertificateHandler) (resp *pb.Response, err error) {
	// Get a transaction handler to be used to submit the execute transaction
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

	chaincodeInput := &pb.ChaincodeInput{
		Args: util.ToChaincodeArgs("assign", asset, base64.StdEncoding.EncodeToString(newOwnerCert.GetCertificate())),
	}
	chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
	if err != nil {
		return nil, err
	}

	// Access control. Administrator signs chaincodeInputRaw || binding to confirm his identity
	sigma, err := invokerCert.Sign(append(chaincodeInputRaw, binding...))
	if err != nil {
		return nil, err
	}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: chaincodeName},
		CtorMsg:              chaincodeInput,
		Metadata:             sigma, // Proof of identity
		ConfidentialityLevel: confidentialityLevel,
	}

	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, util.GenerateUUID())
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	return processTransaction(transaction)
}

func transferOwnershipInternal(owner crypto.Client, ownerCert crypto.CertificateHandler, asset string, newOwnerCert crypto.CertificateHandler) (resp *pb.Response, err error) {
	// Get a transaction handler to be used to submit the execute transaction
	// and bind the chaincode access control logic using the binding

	submittingCertHandler, err := owner.GetTCertificateHandlerNext()
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

	chaincodeInput := &pb.ChaincodeInput{
		Args: util.ToChaincodeArgs("transfer", asset, base64.StdEncoding.EncodeToString(newOwnerCert.GetCertificate())),
	}
	chaincodeInputRaw, err := proto.Marshal(chaincodeInput)
	if err != nil {
		return nil, err
	}

	// Access control. Owner signs chaincodeInputRaw || binding to confirm his identity
	sigma, err := ownerCert.Sign(append(chaincodeInputRaw, binding...))
	if err != nil {
		return nil, err
	}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: chaincodeName},
		CtorMsg:              chaincodeInput,
		Metadata:             sigma, // Proof of identity
		ConfidentialityLevel: confidentialityLevel,
	}

	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	transaction, err := txHandler.NewChaincodeExecute(chaincodeInvocationSpec, util.GenerateUUID())
	if err != nil {
		return nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	return processTransaction(transaction)

}

func whoIsTheOwner(invoker crypto.Client, asset string) (transaction *pb.Transaction, resp *pb.Response, err error) {
	chaincodeInput := &pb.ChaincodeInput{Args: util.ToChaincodeArgs("query", asset)}

	// Prepare spec and submit
	spec := &pb.ChaincodeSpec{
		Type:                 1,
		ChaincodeID:          &pb.ChaincodeID{Name: chaincodeName},
		CtorMsg:              chaincodeInput,
		ConfidentialityLevel: confidentialityLevel,
	}

	chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	transaction, err = invoker.NewChaincodeQuery(chaincodeInvocationSpec, util.GenerateUUID())
	if err != nil {
		return nil, nil, fmt.Errorf("Error deploying chaincode: %s ", err)
	}

	resp, err = processTransaction(transaction)
	return
}

func getChaincodeBytes(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	mode := viper.GetString("chaincode.mode")
	var codePackageBytes []byte
	if mode != chaincode.DevModeUserRunsChaincode {
		appLogger.Debugf("Received build request for chaincode spec: %v", spec)
		var err error
		if err = checkSpec(spec); err != nil {
			return nil, err
		}

		codePackageBytes, err = container.GetChaincodePackageBytes(spec)
		if err != nil {
			err = fmt.Errorf("Error getting chaincode package bytes: %s", err)
			appLogger.Errorf("%s", err)
			return nil, err
		}
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func checkSpec(spec *pb.ChaincodeSpec) error {
	// Don't allow nil value
	if spec == nil {
		return errors.New("Expected chaincode specification, nil received")
	}

	platform, err := platforms.Find(spec.Type)
	if err != nil {
		return fmt.Errorf("Failed to determine platform type: %s", err)
	}

	return platform.ValidateSpec(spec)
}
