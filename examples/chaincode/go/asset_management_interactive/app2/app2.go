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

	// Bob is the administrator
	bob     crypto.Client
	bobCert crypto.CertificateHandler

	// Charlie, Dave, and Edwina are owners
	charlie     crypto.Client
	charlieCert crypto.CertificateHandler

	dave     crypto.Client
	daveCert crypto.CertificateHandler

	edwina     crypto.Client
	edwinaCert crypto.CertificateHandler

	assets  map[string]string
	lotNums []string
)

func assignOwnership() (err error) {
	appLogger.Debug("------------- Bob wants to assign the asset owners to all of the assets...")

	i := 0
	var ownerCert crypto.CertificateHandler

	for _, lotNum := range lotNums {
		assetName := assets[lotNum]

		if i%3 == 0 {
			appLogger.Debugf("Assigning ownership of asset '[%s]: [%s]' to '[%s]'", lotNum, assetName, "Charlie")
			ownerCert = charlieCert
		} else if i%3 == 1 {
			appLogger.Debugf("Assigning ownership of asset '[%s]: [%s]' to '[%s]'", lotNum, assetName, "Dave")
			ownerCert = daveCert
		} else {
			appLogger.Debugf("Assigning ownership of asset '[%s]: [%s]' to '[%s]'", lotNum, assetName, "Edwina")
			ownerCert = edwinaCert
		}

		resp, err := assignOwnershipInternal(bob, bobCert, assetName, ownerCert)
		if err != nil {
			appLogger.Errorf("Failed assigning ownership [%s]", err)
			return err
		}
		appLogger.Debugf("Resp [%s]", resp.String())

		i++
	}

	appLogger.Debug("Wait 60 seconds...")
	time.Sleep(60 * time.Second)

	appLogger.Debug("------------- Done!")
	return
}

func testAssetManagementChaincode() (err error) {
	// Assign
	err = assignOwnership()
	if err != nil {
		appLogger.Errorf("Failed assigning ownership [%s]", err)
		return
	}

	appLogger.Debug("Assigned ownership!")

	closeCryptoClient(bob)
	closeCryptoClient(charlie)
	closeCryptoClient(dave)
	closeCryptoClient(edwina)

	return
}

func main() {
	if len(os.Args) != 2 {
		appLogger.Errorf("Error -- A ChaincodeName must be specified.")
		os.Exit(-1)
	}

	chaincodeName = os.Args[1]

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
