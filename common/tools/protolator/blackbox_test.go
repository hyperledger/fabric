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

package protolator_test

import (
	"bytes"
	"testing"

	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	. "github.com/hyperledger/fabric/common/tools/protolator"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func bidirectionalMarshal(t *testing.T, doc proto.Message) {
	var buffer bytes.Buffer

	assert.NoError(t, DeepMarshalJSON(&buffer, doc))

	newRoot := proto.Clone(doc)
	newRoot.Reset()
	assert.NoError(t, DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newRoot))

	// Note, we cannot do an equality check between newRoot and sampleDoc
	// because of the nondeterministic nature of binary proto marshaling
	// So instead we re-marshal to JSON which is a deterministic marshaling
	// and compare equality there instead

	//t.Log(doc)
	//t.Log(newRoot)

	var remarshaled bytes.Buffer
	assert.NoError(t, DeepMarshalJSON(&remarshaled, newRoot))
	assert.Equal(t, string(buffer.Bytes()), string(remarshaled.Bytes()))
	//t.Log(string(buffer.Bytes()))
	//t.Log(string(remarshaled.Bytes()))
}

func TestConfigUpdate(t *testing.T) {
	p := provisional.New(genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile))
	ct := p.ChannelTemplate()
	cue, err := ct.Envelope("Foo")
	assert.NoError(t, err)

	bidirectionalMarshal(t, cue)
}

func TestGenesisBlock(t *testing.T) {
	p := provisional.New(genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile))
	gb := p.GenesisBlockForChannel("foo")

	bidirectionalMarshal(t, gb)

}
