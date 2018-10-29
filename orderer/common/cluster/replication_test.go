/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestBlockPullerFromConfigBlockFailures(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	assert.NoError(t, err)

	validBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, validBlock))

	for _, testCase := range []struct {
		name         string
		expectedErr  string
		pullerConfig cluster.PullerConfig
		block        *common.Block
	}{
		{
			name:        "nil block",
			expectedErr: "nil block",
		},
		{
			name:        "invalid block",
			expectedErr: "block data is nil",
			block:       &common.Block{},
		},
		{
			name: "bad envelope inside block",
			expectedErr: "failed extracting bundle from envelope: " +
				"failed to unmarshal payload from envelope: " +
				"error unmarshaling Payload: " +
				"proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:        "invalid TLS certificate",
			expectedErr: "unable to decode TLS certificate PEM: ////",
			block:       validBlock,
			pullerConfig: cluster.PullerConfig{
				TLSCert: []byte{255, 255, 255},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			bp, err := cluster.BlockPullerFromConfigBlock(testCase.pullerConfig, testCase.block)
			assert.EqualError(t, err, testCase.expectedErr)
			assert.Nil(t, bp)
		})
	}
}

func TestBlockPullerFromConfigBlockGreenPath(t *testing.T) {
	caCert, err := ioutil.ReadFile(filepath.Join("testdata", "ca.crt"))
	assert.NoError(t, err)

	tlsCert, err := ioutil.ReadFile(filepath.Join("testdata", "server.crt"))
	assert.NoError(t, err)

	tlsKey, err := ioutil.ReadFile(filepath.Join("testdata", "server.key"))
	assert.NoError(t, err)

	osn := newClusterNode(t)
	osn.srv.Stop()
	// Replace the gRPC server with a TLS one
	osn.srv, err = comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			Key:               tlsKey,
			RequireClientCert: true,
			Certificate:       tlsCert,
			ClientRootCAs:     [][]byte{caCert},
			UseTLS:            true,
		},
	})
	assert.NoError(t, err)
	orderer.RegisterAtomicBroadcastServer(osn.srv.Server(), osn)
	// And start it
	go osn.srv.Start()
	defer osn.stop()

	// Start from a valid configuration block
	blockBytes, err := ioutil.ReadFile(filepath.Join("testdata", "mychannel.block"))
	assert.NoError(t, err)

	validBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, validBlock))

	// And inject into it a 127.0.0.1 orderer endpoint endpoint and a new TLS CA certificate.
	injectTLSCACert(t, validBlock, caCert)
	injectOrdererEndpoint(t, validBlock, osn.srv.Address())
	validBlock.Header.DataHash = validBlock.Data.Hash()

	blockMsg := &orderer.DeliverResponse_Block{
		Block: validBlock,
	}

	osn.blockResponses <- &orderer.DeliverResponse{
		Type: blockMsg,
	}

	osn.blockResponses <- &orderer.DeliverResponse{
		Type: blockMsg,
	}

	bp, err := cluster.BlockPullerFromConfigBlock(cluster.PullerConfig{
		TLSCert:             tlsCert,
		TLSKey:              tlsKey,
		MaxTotalBufferBytes: 1,
		Channel:             "mychannel",
		Signer:              &crypto.LocalSigner{},
		Timeout:             time.Second,
	}, validBlock)
	assert.NoError(t, err)
	defer bp.Close()

	osn.addExpectProbeAssert()
	osn.addExpectPullAssert(0)

	block := bp.PullBlock(0)
	assert.Equal(t, uint64(0), block.Header.Number)
}

func TestNoopBlockVerifier(t *testing.T) {
	v := &cluster.NoopBlockVerifier{}
	assert.Nil(t, v.VerifyBlockSignature(nil, nil))
}

func injectOrdererEndpoint(t *testing.T, block *common.Block, endpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{endpoint})
	// Unwrap the layers until we reach the orderer addresses
	env, err := utils.ExtractEnvelope(block, 0)
	assert.NoError(t, err)
	payload, err := utils.ExtractPayload(env)
	assert.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)
	// Replace the orderer addresses
	confEnv.Config.ChannelGroup.Values[ordererAddresses.Key()].Value = utils.MarshalOrPanic(ordererAddresses.Value())
	// And put it back into the block
	payload.Data = utils.MarshalOrPanic(confEnv)
	env.Payload = utils.MarshalOrPanic(payload)
	block.Data.Data[0] = utils.MarshalOrPanic(env)
}

func injectTLSCACert(t *testing.T, block *common.Block, tlsCA []byte) {
	// Unwrap the layers until we reach the TLS CA certificates
	env, err := utils.ExtractEnvelope(block, 0)
	assert.NoError(t, err)
	payload, err := utils.ExtractPayload(env)
	assert.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)
	mspKey := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups["OrdererOrg"].Values[channelconfig.MSPKey]
	rawMSPConfig := mspKey.Value
	mspConf := &msp.MSPConfig{}
	proto.Unmarshal(rawMSPConfig, mspConf)
	fabricMSPConf := &msp.FabricMSPConfig{}
	proto.Unmarshal(mspConf.Config, fabricMSPConf)
	// Replace the TLS root certs with the given ones
	fabricMSPConf.TlsRootCerts = [][]byte{tlsCA}
	// And put it back into the block
	mspConf.Config = utils.MarshalOrPanic(fabricMSPConf)
	mspKey.Value = utils.MarshalOrPanic(mspConf)
	payload.Data = utils.MarshalOrPanic(confEnv)
	env.Payload = utils.MarshalOrPanic(payload)
	block.Data.Data[0] = utils.MarshalOrPanic(env)
}

func TestIsNewChannelBlock(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		expectedErr  string
		returnedName string
		block        *common.Block
	}{
		{
			name:        "nil block",
			expectedErr: "nil block",
		},
		{
			name:        "no data section in block",
			expectedErr: "block data is nil",
			block:       &common.Block{},
		},
		{
			name: "corrupt envelope in block",
			expectedErr: "block data does not carry an" +
				" envelope at index 0: error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{{1, 2, 3}},
				},
			},
		},
		{
			name:        "corrupt payload in envelope",
			expectedErr: "no payload in envelope: proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:        "no header in block",
			expectedErr: "nil header in payload",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{}),
					})},
				},
			},
		},
		{
			name: "corrupt channel header",
			expectedErr: "error unmarshaling ChannelHeader:" +
				" proto: common.ChannelHeader: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: []byte{1, 2, 3},
							},
						}),
					})},
				},
			},
		},
		{
			name:        "not an orderer transaction",
			expectedErr: "",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_CONFIG_UPDATE),
								}),
							},
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with corrupt inner envelope",
			expectedErr: "error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: []byte{1, 2, 3},
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with corrupt inner payload",
			expectedErr: "error unmarshaling Payload: proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: []byte{1, 2, 3},
							}),
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with nil inner header",
			expectedErr: "inner payload's header is nil",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{}),
							}),
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with corrupt inner channel header",
			expectedErr: "error unmarshaling ChannelHeader: proto: common.ChannelHeader: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: []byte{1, 2, 3},
									},
								}),
							}),
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction that is not a config, but a config update",
			expectedErr: "",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
											Type: int32(common.HeaderType_CONFIG_UPDATE),
										}),
									},
								}),
							}),
						}),
					})},
				},
			},
		},
		{
			expectedErr: "",
			name:        "orderer transaction that is a system channel config block",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									ChannelId: "systemChannel",
									Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
											Type:      int32(common.HeaderType_CONFIG),
											ChannelId: "systemChannel",
										}),
									},
								}),
							}),
						}),
					})},
				},
			},
		},
		{
			name:         "orderer transaction that creates a new application channel",
			expectedErr:  "",
			returnedName: "notSystemChannel",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									ChannelId: "systemChannel",
									Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
											Type:      int32(common.HeaderType_CONFIG),
											ChannelId: "notSystemChannel",
										}),
									},
								}),
							}),
						}),
					})},
				},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			channelName, err := cluster.IsNewChannelBlock(testCase.block)
			if testCase.expectedErr != "" {
				assert.EqualError(t, err, testCase.expectedErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, testCase.returnedName, channelName)
		})
	}
}

func TestChannels(t *testing.T) {
	makeBlock := func(outerChannelName, innerChannelName string) *common.Block {
		return &common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
					Payload: utils.MarshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
								ChannelId: outerChannelName,
								Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
							}),
						},
						Data: utils.MarshalOrPanic(&common.Envelope{
							Payload: utils.MarshalOrPanic(&common.Payload{
								Header: &common.Header{
									ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
										Type:      int32(common.HeaderType_CONFIG),
										ChannelId: innerChannelName,
									}),
								},
							}),
						}),
					}),
				})},
			},
		}
	}

	for _, testCase := range []struct {
		name               string
		prepareSystemChain func(systemChain []*common.Block)
		assertion          func(t *testing.T, ci *cluster.ChainInspector)
	}{
		{
			name: "happy path - artificial blocks",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				actual := ci.Channels()
				// Assert that the returned channels are returned in any order
				assert.Contains(t, [][]string{{"mychannel", "mychannel2"}, {"mychannel2", "mychannel"}}, actual)
			},
		},
		{
			name: "happy path - one block is not artificial but real",
			prepareSystemChain: func(systemChain []*common.Block) {
				blockbytes, err := ioutil.ReadFile(filepath.Join("testdata", "block3.pb"))
				assert.NoError(t, err)
				block := &common.Block{}
				err = proto.Unmarshal(blockbytes, block)
				assert.NoError(t, err)

				systemChain[len(systemChain)/2] = block
				assignHashes(systemChain)
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				actual := ci.Channels()
				// Assert that the returned channels are returned in any order
				assert.Contains(t, [][]string{{"mychannel", "bar"}, {"bar", "mychannel"}}, actual)
			},
		},
		{
			name:               "bad path - pulled chain's hash is mismatched",
			prepareSystemChain: func(_ []*common.Block) {},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				panicValue := "System channel pulled doesn't match the boot last config block:" +
					" block 4's hash (d8553eb97aa57e3c795a185f30efdbe8d88ae4b1e44b984b311159beac9bd5f4)" +
					" mismatches 3's prev block hash ()"
				assert.PanicsWithValue(t, panicValue, func() {
					ci.Channels()
				})
			},
		},
		{
			name: "bad path - a block cannot be classified",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
				systemChain[len(systemChain)-2].Data.Data = [][]byte{{1, 2, 3}}
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				panicValue := "Failed classifying block 3 : block data does not carry" +
					" an envelope at index 0: error unmarshaling Envelope: " +
					"proto: common.Envelope: illegal tag 0 (wire type 1)"
				assert.PanicsWithValue(t, panicValue, func() {
					ci.Channels()
				})
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			systemChain := []*common.Block{
				makeBlock("systemChannel", "systemChannel"),
				makeBlock("systemChannel", "mychannel"),
				makeBlock("systemChannel", "mychannel2"),
				makeBlock("systemChannel", "systemChannel"),
			}

			for i := 0; i < len(systemChain); i++ {
				systemChain[i].Header.DataHash = systemChain[i].Data.Hash()
				systemChain[i].Header.Number = uint64(i + 1)
			}
			testCase.prepareSystemChain(systemChain)
			puller := &mocks.ChainPuller{}
			for seq := uint64(1); int(seq) <= len(systemChain); seq++ {
				puller.On("PullBlock", seq).Return(systemChain[int(seq)-1])
			}

			ci := &cluster.ChainInspector{
				Logger:          flogging.MustGetLogger("test"),
				Puller:          puller,
				LastConfigBlock: systemChain[len(systemChain)-1],
			}
			testCase.assertion(t, ci)
		})
	}
}
