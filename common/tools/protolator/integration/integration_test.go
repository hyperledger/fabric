/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"bytes"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/protolator"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func bidirectionalMarshal(t *testing.T, doc proto.Message) {
	var buffer bytes.Buffer

	assert.NoError(t, protolator.DeepMarshalJSON(&buffer, doc))

	newRoot := proto.Clone(doc)
	newRoot.Reset()
	assert.NoError(t, protolator.DeepUnmarshalJSON(bytes.NewReader(buffer.Bytes()), newRoot))

	// Note, we cannot do an equality check between newRoot and sampleDoc
	// because of the nondeterministic nature of binary proto marshaling
	// So instead we re-marshal to JSON which is a deterministic marshaling
	// and compare equality there instead

	//t.Log(doc)
	//t.Log(newRoot)

	var remarshaled bytes.Buffer
	assert.NoError(t, protolator.DeepMarshalJSON(&remarshaled, newRoot))
	assert.Equal(t, string(buffer.Bytes()), string(remarshaled.Bytes()))
	//t.Log(string(buffer.Bytes()))
	//t.Log(string(remarshaled.Bytes()))
}

func TestConfigUpdate(t *testing.T) {
	cg, err := encoder.NewChannelGroup(configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile))
	assert.NoError(t, err)

	bidirectionalMarshal(t, &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(&cb.ConfigUpdate{
			ReadSet:  cg,
			WriteSet: cg,
		}),
	})
}

func TestIdemix(t *testing.T) {
	bidirectionalMarshal(t, &msp.MSPConfig{
		Type: 1,
		Config: utils.MarshalOrPanic(&msp.IdemixMSPConfig{
			Name: "fooo",
		}),
	})
}

func TestGenesisBlock(t *testing.T) {
	p := encoder.New(configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile))
	gb := p.GenesisBlockForChannel("foo")

	bidirectionalMarshal(t, gb)
}

func TestEmitDefaultsBug(t *testing.T) {
	block := &cb.Block{
		Header: &cb.BlockHeader{
			PreviousHash: []byte("foo"),
		},
		Data: &cb.BlockData{
			Data: [][]byte{
				utils.MarshalOrPanic(&cb.Envelope{
					Payload: utils.MarshalOrPanic(&cb.Payload{
						Header: &cb.Header{
							ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
								Type: int32(cb.HeaderType_CONFIG),
							}),
						},
					}),
					Signature: []byte("bar"),
				}),
			},
		},
	}

	err := protolator.DeepMarshalJSON(os.Stdout, block)
	assert.NoError(t, err)
}

func TestProposalResponsePayload(t *testing.T) {
	prp := &pb.ProposalResponsePayload{}
	assert.NoError(t, protolator.DeepUnmarshalJSON(bytes.NewReader([]byte(`{
            "extension": {
              "chaincode_id": {
                "name": "test",
                "path": "",
                "version": "1.0"
              },
              "events": {
                  "chaincode_id": "test"
              },
              "response": {
                "message": "",
                "payload": null,
                "status": 200
              },
              "results": {
                "data_model": "KV",
                "ns_rwset": [
                  {
                    "collection_hashed_rwset": [],
                    "namespace": "lscc",
                    "rwset": {
                      "metadata_writes": [],
                      "range_queries_info": [],
                      "reads": [
                        {
                          "key": "cc1",
                          "version": {
                            "block_num": "3",
                            "tx_num": "0"
                          }
                        },
                        {
                          "key": "cc2",
                          "version": {
                            "block_num": "4",
                            "tx_num": "0"
                          }
                        }
                      ],
                      "writes": []
                    }
                  },
                  {
                    "collection_hashed_rwset": [],
                    "namespace": "cc1",
                    "rwset": {
                      "metadata_writes": [],
                      "range_queries_info": [],
                      "reads": [
                        {
                          "key": "key1",
                          "version": {
                            "block_num": "8",
                            "tx_num": "0"
                          }
                        }
                      ],
                      "writes": [
                        {
                          "is_delete": false,
                          "key": "key2"
                        }
                      ]
                    }
                  },
                  {
                    "collection_hashed_rwset": [],
                    "namespace": "cc2",
                    "rwset": {
                      "metadata_writes": [],
                      "range_queries_info": [],
                      "reads": [
                        {
                          "key": "key1",
                          "version": {
                            "block_num": "9",
                            "tx_num": "0"
                          }
                        },
                        {
                          "key": "key2",
                          "version": {
                            "block_num": "10",
                            "tx_num": "0"
                          }
                        }
                      ],
                      "writes": [
                        {
                          "is_delete": false,
                          "key": "key1"
                        },
                        {
                          "is_delete": true,
                          "key": "key2"
                        }
                      ]
                    }
                  }
                ]
              }
            }
        }`)), prp))
	bidirectionalMarshal(t, prp)
}

func TestChannelCreationPolicy(t *testing.T) {
	cu := &cb.ConfigUpdate{
		WriteSet: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Consortiums": {
					Groups: map[string]*cb.ConfigGroup{
						"SampleConsortium": {
							Values: map[string]*cb.ConfigValue{
								"ChannelCreationPolicy": {
									Version: 0,
								},
							},
						},
					},
				},
			},
		},
	}

	bidirectionalMarshal(t, cu)
}
