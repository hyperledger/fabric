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
	"testing"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"

	"golang.org/x/net/context"
)

func TestExecuteInvokeOnManyChains(t *testing.T) {
	testForSkip(t)
	//lets use 2 chains to test multi chains
	chains := []string{"chain1", "chain2"}
	lis, err := initPeer(chains...)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chains...)

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID := &pb.ChaincodeID{Name: "example02", Path: url, Version: "0"}

	args := []string{"a", "b", "10"}
	for _, c := range chains {
		cccid := ccprovider.NewCCContext(c, "example02", "0", "", false, nil, nil)
		err = invokeExample02Transaction(ctxt, cccid, chaincodeID, pb.ChaincodeSpec_GOLANG, args, false)
		if err != nil {
			t.Fail()
			t.Logf("Error invoking transaction: %s", err)
		} else {
			t.Logf("Invoke test passed for chain %s", c)
		}
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
	}

}
