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

package ramledger

import (
	"testing"

	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
)

func TestGetOrCreate(t *testing.T) {
	rlf := New(3)
	channel, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	channel2, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	if channel != channel2 {
		t.Fatalf("Expecting already created channel.")
	}
}

func TestChainIDs(t *testing.T) {
	rlf := New(3)
	rlf.GetOrCreate("channel1")
	rlf.GetOrCreate("channel2")
	rlf.GetOrCreate("channel3")
	if len(rlf.ChainIDs()) != 3 {
		t.Fatalf("Expecting three channels,")
	}
	rlf.Close()
}
