/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

import (
	"testing"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestGetName(t *testing.T) {
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: "ccname"}}
	ccid := &CCID{ChaincodeSpec: spec, Version: "ver"}
	name := ccid.GetName()
	assert.Equal(t, "ccname-ver", name, "unexpected name")
}

func TestBadCCID(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should never reach here... GetName should have paniced")
		} else {
			assert.Equal(t, "nil chaincode spec", err, "expected 'nil chaincode spec'")
		}
	}()

	ccid := &CCID{Version: "ver"}
	ccid.GetName()
}
