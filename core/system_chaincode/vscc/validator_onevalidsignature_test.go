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
package vscc

import (
	"testing"

	"fmt"
	"os"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

func createTx() (*common.Envelope, error) {
	cis := &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{ChaincodeID: &peer.ChaincodeID{Name: "foo"}}}

	uuid := util.GenerateUUID()

	prop, err := utils.CreateProposalFromCIS(uuid, util.GetTestChainID(), cis, sid)
	if err != nil {
		return nil, err
	}

	presp, err := utils.CreateProposalResponse(prop.Header, prop.Payload, []byte("res"), nil, nil, id)
	if err != nil {
		return nil, err
	}

	return utils.CreateSignedTx(prop, id, presp)
}

func TestInit(t *testing.T) {
	v := new(ValidatorOneValidSignature)
	stub := shim.NewMockStub("validatoronevalidsignature", v)

	if _, err := stub.MockInit("1", nil); err != nil {
		t.Fatalf("vscc init failed with %v", err)
	}
}

func TestInvoke(t *testing.T) {
	v := new(ValidatorOneValidSignature)
	stub := shim.NewMockStub("validatoronevalidsignature", v)

	// Failed path: Invalid arguments
	args := [][]byte{[]byte("dv")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("vscc invoke should have failed")
		return
	}

	args = [][]byte{[]byte("dv"), []byte("tx")}
	args[1] = nil
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("vscc invoke should have failed")
		return
	}

	tx, err := createTx()
	if err != nil {
		t.Fatalf("createTx returned err %s", err)
		return
	}

	envBytes, err := utils.GetBytesEnvelope(tx)
	if err != nil {
		t.Fatalf("GetBytesEnvelope returned err %s", err)
		return
	}

	args = [][]byte{[]byte("dv"), envBytes}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("vscc invoke returned err %s", err)
		return
	}
}

var id msp.SigningIdentity
var sid []byte

func TestMain(m *testing.M) {
	var err error

	primitives.InitSecurityLevel("SHA2", 256)
	// setup the MSP manager so that we can sign/verify
	mspMgrConfigFile := "../../../msp/peer-config.json"
	msp.GetManager().Setup(mspMgrConfigFile)

	id, err = msp.GetManager().GetSigningIdentity(&msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: "DEFAULT"}, Value: "PEER"})
	if err != nil {
		fmt.Printf("GetSigningIdentity failed with err %s", err)
		os.Exit(-1)
	}

	sid, err = id.Serialize()
	if err != nil {
		fmt.Printf("Serialize failed with err %s", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}
