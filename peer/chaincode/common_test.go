/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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
	"encoding/json"
	"testing"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestCheckChaincodeCmdParamsWithNewCallingSchema(t *testing.T) {
	chaincodeCtorJSON = `{ "Args":["func", "param"] }`
	chaincodePath = "some/path"
	chaincodeName = "somename"
	require := require.New(t)
	result := checkChaincodeCmdParams(&cobra.Command{})

	require.Nil(result)
}

func TestCheckChaincodeCmdParamsWithOldCallingSchema(t *testing.T) {
	chaincodeCtorJSON = `{ "Function":"func", "Args":["param"] }`
	chaincodePath = "some/path"
	chaincodeName = "somename"
	require := require.New(t)
	result := checkChaincodeCmdParams(&cobra.Command{})

	require.Nil(result)
}

func TestCheckChaincodeCmdParamsWithoutName(t *testing.T) {
	chaincodeCtorJSON = `{ "Function":"func", "Args":["param"] }`
	chaincodePath = "some/path"
	chaincodeName = ""
	require := require.New(t)
	result := checkChaincodeCmdParams(&cobra.Command{})

	require.Error(result)
}

func TestCheckChaincodeCmdParamsWithFunctionOnly(t *testing.T) {
	chaincodeCtorJSON = `{ "Function":"func" }`
	chaincodePath = "some/path"
	chaincodeName = "somename"
	require := require.New(t)
	result := checkChaincodeCmdParams(&cobra.Command{})

	require.Error(result)
}

func TestCheckChaincodeCmdParamsEmptyCtor(t *testing.T) {
	chaincodeCtorJSON = `{}`
	chaincodePath = "some/path"
	chaincodeName = "somename"
	require := require.New(t)
	result := checkChaincodeCmdParams(&cobra.Command{})

	require.Error(result)
}

func TestCheckValidJSON(t *testing.T) {
	validJSON := `{"Args":["a","b","c"]}`
	input := &pb.ChaincodeInput{}
	if err := json.Unmarshal([]byte(validJSON), &input); err != nil {
		t.Fail()
		t.Logf("Chaincode argument error: %s", err)
		return
	}

	validJSON = `{"Function":"f", "Args":["a","b","c"]}`
	if err := json.Unmarshal([]byte(validJSON), &input); err != nil {
		t.Fail()
		t.Logf("Chaincode argument error: %s", err)
		return
	}

	validJSON = `{"Function":"f", "Args":[]}`
	if err := json.Unmarshal([]byte(validJSON), &input); err != nil {
		t.Fail()
		t.Logf("Chaincode argument error: %s", err)
		return
	}

	validJSON = `{"Function":"f"}`
	if err := json.Unmarshal([]byte(validJSON), &input); err != nil {
		t.Fail()
		t.Logf("Chaincode argument error: %s", err)
		return
	}
}

func TestCheckInvalidJSON(t *testing.T) {
	invalidJSON := `{["a","b","c"]}`
	input := &pb.ChaincodeInput{}
	if err := json.Unmarshal([]byte(invalidJSON), &input); err == nil {
		t.Fail()
		t.Logf("Bar argument error should have been caught: %s", invalidJSON)
		return
	}

	invalidJSON = `{"Function":}`
	if err := json.Unmarshal([]byte(invalidJSON), &input); err == nil {
		t.Fail()
		t.Logf("Chaincode argument error: %s", err)
		t.Logf("Bar argument error should have been caught: %s", invalidJSON)
		return
	}
}
