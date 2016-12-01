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

package testutils

import (
	"fmt"
	"os"

	"path/filepath"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
)

var (
	signingIdentity *msp.IdentityIdentifier
	signer          msp.SigningIdentity
)

func init() {
	var err error
	primitives.SetSecurityLevel("SHA2", 256)
	// setup the MSP manager so that we can sign/verify
	mspMgrConfigFile, err := getMSPMgrConfigFile()
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not get location of msp manager config file")
		return
	}
	msp.GetManager().Setup(mspMgrConfigFile)
	mspID := "DEFAULT"
	id := "PEER"
	signingIdentity = &msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: mspID}, Value: id}
	signer, err = msp.GetManager().GetSigningIdentity(signingIdentity)
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize msp/signer")
		return
	}
}

func getMSPMgrConfigFile() (string, error) {
	var pwd string
	var err error
	if pwd, err = os.Getwd(); err != nil {
		return "", err
	}
	path := pwd
	dir := ""
	for {
		path, dir = filepath.Split(path)
		path = filepath.Clean(path)
		fmt.Printf("path=%s, dir=%s\n", path, dir)
		if dir == "fabric" {
			break
		}
	}
	filePath := filepath.Join(path, "fabric/msp/peer-config.json")
	fmt.Printf("filePath=%s\n", filePath)
	return filePath, nil
}

// ConstructSingedTxEnvWithDefaultSigner constructs a transaction envelop for tests with a default signer.
// This method helps other modules to construct a transaction with supplied parameters
func ConstructSingedTxEnvWithDefaultSigner(txid string, chainID, ccName string, simulationResults []byte, events []byte, visibility []byte) (*common.Envelope, error) {
	return ConstructSingedTxEnv(txid, chainID, ccName, simulationResults, events, visibility, signer)
}

// ConstructSingedTxEnv constructs a transaction envelop for tests
func ConstructSingedTxEnv(txid string, chainID string, ccName string, simulationResults []byte, events []byte, visibility []byte, signer msp.SigningIdentity) (*common.Envelope, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	prop, err := putils.CreateChaincodeProposal(txid, chainID, &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: &pb.ChaincodeID{Name: ccName}}}, ss)
	if err != nil {
		return nil, err
	}

	presp, err := putils.CreateProposalResponse(prop.Header, prop.Payload, simulationResults, nil, nil, signer)
	if err != nil {
		return nil, err
	}

	env, err := putils.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, err
	}
	return env, nil
}

var mspLcl msp.PeerMSP
var sigId msp.SigningIdentity

// ConstructUnsingedTxEnv creates a Transaction envelope from given inputs
func ConstructUnsingedTxEnv(txid string, chainID string, ccName string, simulationResults []byte, events []byte, visibility []byte) (*common.Envelope, error) {
	if mspLcl == nil {
		mspLcl = msp.NewNoopMsp()
		sigId, _ = mspLcl.GetSigningIdentity(nil)
	}

	return ConstructSingedTxEnv(txid, chainID, ccName, simulationResults, events, visibility, sigId)
}
