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
	"bufio"
	"fmt"
	"os"
	"reflect"

	"strings"

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

	// Charlie, Dave, and Edwina are owners
	charlie     crypto.Client
	charlieCert crypto.CertificateHandler

	dave     crypto.Client
	daveCert crypto.CertificateHandler

	edwina     crypto.Client
	edwinaCert crypto.CertificateHandler

	assets  map[string]string
	lotNums []string

	clients map[string]crypto.Client
	certs   map[string]crypto.CertificateHandler

	myClient crypto.Client
	myCert   crypto.CertificateHandler
)

func transferOwnership(lotNum string, newOwner string) (err error) {
	if !isAssetKnown(lotNum) {
		appLogger.Errorf("Error -- asset '%s' does not exist.", lotNum)
		return
	}

	if !isUserKnown(newOwner) {
		appLogger.Errorf("Error -- user '%s' is not known.", newOwner)
		return
	}

	assetName := assets[lotNum]

	appLogger.Debugf("------------- '%s' wants to transfer the ownership of '%s: %s' to '%s'...", user, lotNum, assetName, newOwner)

	if !isOwner(assetName, user) {
		appLogger.Debugf("'%s' is not the owner of '%s: %s' -- transfer not allowed.", user, lotNum, assetName)
		return
	}

	resp, err := transferOwnershipInternal(myClient, myCert, assetName, certs[newOwner])
	if err != nil {
		appLogger.Debugf("Failed to transfer '%s: %s' to '%s'", lotNum, assetName, newOwner)
		return err
	}
	appLogger.Debugf("Resp [%s]", resp.String())

	appLogger.Debugf("'%s' is the new owner of '%s: %s'!", newOwner, lotNum, assetName)
	appLogger.Debug("------------- Done!")
	return
}

func listOwnedAssets() {
	ownedAssets := make([]string, 0, len(assets))

	for _, lotNum := range lotNums {
		assetName := assets[lotNum]

		if isOwner(assetName, user) {
			ownedAsset := "'" + lotNum + ": " + assetName + "'"
			ownedAssets = append(ownedAssets, ownedAsset)
		}
	}

	appLogger.Debugf("'%s' owns the following %d assets:", user, len(ownedAssets))

	for _, asset := range ownedAssets {
		appLogger.Debug(asset)
	}
}

func isOwner(assetName string, user string) (isOwner bool) {
	appLogger.Debug("Query....")
	queryTx, theOwnerIs, err := whoIsTheOwner(myClient, assetName)
	if err != nil {
		return false
	}
	appLogger.Debugf("Resp [%s]", theOwnerIs.String())
	appLogger.Debug("Query....done")

	var res []byte
	if confidentialityOn {
		// Decrypt result
		res, err = myClient.DecryptQueryResult(queryTx, theOwnerIs.Msg)
		if err != nil {
			appLogger.Errorf("Failed decrypting result [%s]", err)
			return false
		}
	} else {
		res = theOwnerIs.Msg
	}

	if !reflect.DeepEqual(res, certs[user].GetCertificate()) {
		appLogger.Errorf("'%s' is not the owner.", user)

		appLogger.Debugf("Query result  : [% x]", res)
		appLogger.Debugf("%s's cert: [% x]", certs[user].GetCertificate(), user)

		return false
	} else {
		return true
	}
}

func isUserKnown(userName string) (ok bool) {
	_, ok = clients[userName]
	return ok
}

func isAssetKnown(assetName string) (ok bool) {
	_, ok = assets[assetName]
	return ok
}

func main() {
	if len(os.Args) != 3 {
		appLogger.Error("Error -- A ChaincodeName and username must be specified.")
		os.Exit(-1)
	}

	chaincodeName = os.Args[1]
	user = os.Args[2]

	// Initialize a non-validating peer whose role is to submit
	// transactions to the fabric network.
	// A 'core.yaml' file is assumed to be available in the working directory.
	if err := initNVP(); err != nil {
		appLogger.Debugf("Failed initiliazing NVP [%s]", err)
		os.Exit(-1)
	}

	if !isUserKnown(user) {
		appLogger.Errorf("Error -- user '%s' is not known.", user)
		os.Exit(-1)
	}

	// Enable fabric 'confidentiality'
	confidentiality(true)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s$ ", user)
		line, _ := reader.ReadString('\n')
		command := strings.Split(strings.TrimRight(line, "\n"), " ")

		if command[0] == "transfer" {
			transferOwnership(command[1], command[2])
		} else if command[0] == "list" {
			listOwnedAssets()
		} else if command[0] == "exit" {
			os.Exit(0)
		}
	}
}
