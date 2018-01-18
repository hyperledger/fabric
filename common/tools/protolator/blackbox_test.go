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

	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	. "github.com/hyperledger/fabric/common/tools/protolator"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

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
	cg, err := encoder.NewChannelGroup(genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile))
	assert.NoError(t, err)

	bidirectionalMarshal(t, &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(&cb.ConfigUpdate{
			ReadSet:  cg,
			WriteSet: cg,
		}),
	})
}

func TestGenesisBlock(t *testing.T) {
	p := encoder.New(genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile))
	gb := p.GenesisBlockForChannel("foo")

	bidirectionalMarshal(t, gb)
}

func TestResourcesConfig(t *testing.T) {
	p := &cb.Config{
		Type: int32(cb.ConfigType_RESOURCE),
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Chaincodes": {
					Groups: map[string]*cb.ConfigGroup{
						"cc1": {
							Values: map[string]*cb.ConfigValue{
								"ChaincodeIdentifier": {
									Value: utils.MarshalOrPanic(&pb.ChaincodeIdentifier{
										Hash:    []byte("somehashvalue"),
										Version: "aversionstring",
									}),
								},
								"ChaincodeValidation": {
									Value: utils.MarshalOrPanic(
										&pb.ChaincodeValidation{
											Name: "vscc",
											Argument: utils.MarshalOrPanic(&pb.VSCCArgs{
												EndorsementPolicyRef: "foo",
											}),
										}),
								},
								"ChaincodeEndorsement": {
									Value: utils.MarshalOrPanic(&pb.ChaincodeEndorsement{
										Name: "escc",
									}),
								},
							},
						},
					},
				},
				"APIs": {
					Values: map[string]*cb.ConfigValue{
						"Test": {
							Value: utils.MarshalOrPanic(&pb.APIResource{PolicyRef: "Foo"}),
						},
						"Another": {
							Value: utils.MarshalOrPanic(&pb.APIResource{PolicyRef: "Bar"}),
						},
					},
				},
				"PeerPolicies": {},
			},
		},
	}

	bidirectionalMarshal(t, p)
}
