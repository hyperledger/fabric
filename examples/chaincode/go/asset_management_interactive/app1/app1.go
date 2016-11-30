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
	"os"
	"time"

	"github.com/hyperledger/fabric/core/crypto"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

var (
	// Logging
	appLogger = logging.MustGetLogger("app")

	// NVP related objects
	peerClientConn *grpc.ClientConn
	serverClient   pb.PeerClient

	// Alice is the deployer
	alice crypto.Client

	// Bob is the administrator
	bob     crypto.Client
	bobCert crypto.CertificateHandler
)

func deploy() (err error) {
	appLogger.Debug("------------- Alice wants to assign the administrator role to Bob;")
	// Deploy:
	// 1. Alice is the deployer of the chaincode;
	// 2. Alice wants to assign the administrator role to Bob;
	// 3. Alice obtains, via an out-of-band channel, the ECert of Bob, let us call this certificate *BobCert*;
	// 4. Alice constructs a deploy transaction, as described in *application-ACL.md*,  setting the transaction
	// metadata to *DER(CharlieCert)*.
	// 5. Alice submits th	e transaction to the fabric network.

	resp, err := deployInternal(alice, bobCert)
	if err != nil {
		appLogger.Errorf("Failed deploying [%s]", err)
		return
	}
	appLogger.Debugf("Resp [%s]", resp.String())
	appLogger.Debugf("Chaincode NAME: [%s]-[%s]", chaincodeName, string(resp.Msg))

	appLogger.Debug("Wait 60 seconds")
	time.Sleep(60 * time.Second)

	appLogger.Debug("------------- Done!")
	return
}

func testAssetManagementChaincode() (err error) {
	// Deploy
	err = deploy()
	if err != nil {
		appLogger.Errorf("Failed deploying [%s]", err)
		return
	}

	appLogger.Debug("Deployed!")

	closeCryptoClient(alice)
	closeCryptoClient(bob)

	return
}

func main() {
	// Initialize a non-validating peer whose role is to submit
	// transactions to the fabric network.
	// A 'core.yaml' file is assumed to be available in the working directory.
	if err := initNVP(); err != nil {
		appLogger.Debugf("Failed initiliazing NVP [%s]", err)
		os.Exit(-1)
	}

	// Enable fabric 'confidentiality'
	confidentiality(true)

	// Exercise the 'asset_management' chaincode
	if err := testAssetManagementChaincode(); err != nil {
		appLogger.Debugf("Failed testing asset management chaincode [%s]", err)
		os.Exit(-2)
	}
}
