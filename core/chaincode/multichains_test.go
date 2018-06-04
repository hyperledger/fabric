/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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
	_, chaincodeSupport, cleanup, err := initPeer(chains...)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer cleanup()

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"
	chaincodeID := &pb.ChaincodeID{Name: "example02", Path: url, Version: "0"}

	args := []string{"a", "b", "10"}
	for _, c := range chains {
		cccid := ccprovider.NewCCContext(c, "example02", "0", "", false, nil, nil)
		err = invokeExample02Transaction(ctxt, cccid, chaincodeID, pb.ChaincodeSpec_GOLANG, args, chaincodeSupport)
		if err != nil {
			t.Fail()
			t.Logf("Error invoking transaction: %s", err)
		} else {
			t.Logf("Invoke test passed for chain %s", c)
		}
		chaincodeSupport.Stop(cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
	}

}
