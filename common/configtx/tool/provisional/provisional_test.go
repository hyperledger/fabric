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

package provisional

import (
	"bytes"
	"testing"

	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

var confSolo *genesisconfig.Profile
var confKafka *genesisconfig.Profile
var testCases []*genesisconfig.Profile

func init() {
	confSolo = genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile)
	confKafka = genesisconfig.Load("SampleInsecureKafka")
	testCases = []*genesisconfig.Profile{confSolo, confKafka}
}

func TestGenesisBlockHeader(t *testing.T) {
	expectedHeaderNumber := uint64(0)

	for _, tc := range testCases {
		genesisBlock := New(tc).GenesisBlock()
		if genesisBlock.Header.Number != expectedHeaderNumber {
			t.Fatalf("Case %s: Expected header number %d, got %d", tc.Orderer.OrdererType, expectedHeaderNumber, genesisBlock.Header.Number)
		}
		if !bytes.Equal(genesisBlock.Header.PreviousHash, nil) {
			t.Fatalf("Case %s: Expected header previousHash to be nil, got %x", tc.Orderer.OrdererType, genesisBlock.Header.PreviousHash)
		}
	}
}

func TestGenesisMetadata(t *testing.T) {
	for _, tc := range testCases {
		genesisBlock := New(tc).GenesisBlock()
		if genesisBlock.Metadata == nil {
			t.Fatalf("Expected non-nil metadata")
		}

		if genesisBlock.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] == nil {
			t.Fatalf("Should have last config set")
		}
	}
}

func TestGenesisBlockForChannelHeader(t *testing.T) {
	expectedHeaderNumber := uint64(0)

	for _, tc := range testCases {
		genesisBlock := New(tc).GenesisBlockForChannel("mychannel")
		assert.Equal(t, expectedHeaderNumber, genesisBlock.Header.Number, "Case %s: Header number should be equal", tc.Orderer.OrdererType)
		assert.Nil(t, genesisBlock.Header.PreviousHash, "Case %s: Header previousHash to be nil", tc.Orderer.OrdererType)
	}
}
