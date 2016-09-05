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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckChaincodeCmdParamsWithNewCallingSchema(t *testing.T) {
	chaincodeAttributesJSON = "[]"
	chaincodeCtorJSON = `{ "Args":["func", "param"] }`
	chaincodePath = "some/path"
	require := require.New(t)
	result := checkChaincodeCmdParams(nil)

	require.Nil(result)
}

func TestCheckChaincodeCmdParamsWithOldCallingSchema(t *testing.T) {
	chaincodeAttributesJSON = "[]"
	chaincodeCtorJSON = `{ "Function":"func", "Args":["param"] }`
	chaincodePath = "some/path"
	require := require.New(t)
	result := checkChaincodeCmdParams(nil)

	require.Nil(result)
}

func TestCheckChaincodeCmdParamsWithFunctionOnly(t *testing.T) {
	chaincodeAttributesJSON = "[]"
	chaincodeCtorJSON = `{ "Function":"func" }`
	chaincodePath = "some/path"
	require := require.New(t)
	result := checkChaincodeCmdParams(nil)

	require.Error(result)
}

func TestCheckChaincodeCmdParamsEmptyCtor(t *testing.T) {
	chaincodeAttributesJSON = "[]"
	chaincodeCtorJSON = `{}`
	chaincodePath = "some/path"
	require := require.New(t)
	result := checkChaincodeCmdParams(nil)

	require.Error(result)
}
