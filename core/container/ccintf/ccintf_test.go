/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package ccintf

import (
	"encoding/hex"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestGetName(t *testing.T) {
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: "ccname"}}
	ccid := &CCID{ChaincodeSpec: spec, Version: "ver"}
	name := ccid.GetName()
	assert.Equal(t, "ccname-ver", name, "unexpected name")

	ccid.ChainID = GetCCHandlerKey()
	hash := hex.EncodeToString(util.ComputeSHA256([]byte(ccid.ChainID)))
	name = ccid.GetName()
	assert.Equal(t, "ccname-ver-"+hash, name, "unexpected name with hash")
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
