/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation_test

import (
	"errors"
	"math"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestValidateJoinBlock(t *testing.T) {
	validJoinBlock := blockWithGroups(
		map[string]*cb.ConfigGroup{
			"Application": {},
		},
		"my-channel",
	)

	tests := []struct {
		testName          string
		joinBlock         *cb.Block
		expectedChannelID string
		expectedErr       error
	}{
		{
			testName:          "Valid application channel join block",
			joinBlock:         validJoinBlock,
			expectedChannelID: "my-channel",
			expectedErr:       nil,
		},
		{
			testName: "Invalid block data hash",
			joinBlock: func() *cb.Block {
				b := proto.Clone(validJoinBlock).(*cb.Block)
				b.Header.DataHash = []byte("bogus")
				return b
			}(),
			expectedChannelID: "",
			expectedErr:       errors.New("invalid block: Header.DataHash is different from Hash(block.Data)"),
		},
		{
			testName: "Invalid block metadata",
			joinBlock: func() *cb.Block {
				b := proto.Clone(validJoinBlock).(*cb.Block)
				b.Metadata = nil
				return b
			}(),
			expectedChannelID: "",
			expectedErr:       errors.New("invalid block: does not have metadata"),
		},
		{
			testName: "Not supported: system channel join block",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Consortiums": {},
				},
				"my-channel",
			),
			expectedChannelID: "",
			expectedErr:       errors.New("invalid config: contains consortiums: system channel not supported"),
		},
		{
			testName:          "Join block not a config block",
			joinBlock:         nonConfigBlock(),
			expectedChannelID: "",
			expectedErr:       errors.New("block is not a config block"),
		},
		{
			testName: "block ChannelID not valid",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Consortiums": {},
				},
				"My-Channel",
			),
			expectedChannelID: "",
			expectedErr:       errors.New("initializing configtx manager failed: bad channel ID: 'My-Channel' contains illegal characters"),
		},
		{
			testName: "Invalid bundle",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"InvalidGroup": {},
				},
				"my-channel",
			),
			expectedChannelID: "",
			expectedErr:       errors.New("initializing channelconfig failed: Disallowed channel group: "),
		},
		{
			testName: "Join block has no application or consortiums group",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{},
				"my-channel",
			),
			expectedChannelID: "",
			expectedErr:       errors.New("invalid config: must contain application config"),
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			channelID, err := channelparticipation.ValidateJoinBlock(test.joinBlock)
			require.Equal(t, test.expectedChannelID, channelID)
			if test.expectedErr != nil {
				require.EqualError(t, err, test.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateUpdateEnvelope(t *testing.T) {
	validUpdateEnvelope := configUpdateEnvelopeWithGroups("my-channel")

	tests := []struct {
		testName            string
		updateEnvelopeBlock *cb.Envelope
		expectedChannelID   string
		expectedErr         error
	}{
		{
			testName:            "Valid application channel update envelope",
			updateEnvelopeBlock: validUpdateEnvelope,
			expectedChannelID:   "my-channel",
			expectedErr:         nil,
		},
		{
			testName:            "Update envelope not a config",
			updateEnvelopeBlock: nonConfigEnvelope(),
			expectedChannelID:   "",
			expectedErr:         errors.New("bad type"),
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			channelID, err := channelparticipation.ValidateUpdateConfigEnvelope(test.updateEnvelopeBlock)
			require.Equal(t, test.expectedChannelID, channelID)
			if test.expectedErr != nil {
				require.EqualError(t, err, test.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func blockWithGroups(groups map[string]*cb.ConfigGroup, channelID string) *cb.Block {
	block := protoutil.NewBlock(0, []byte{})
	block.Data = &cb.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
						Config: &cb.Config{
							ChannelGroup: &cb.ConfigGroup{
								Groups: groups,
								Values: map[string]*cb.ConfigValue{
									"HashingAlgorithm": {
										Value: protoutil.MarshalOrPanic(&cb.HashingAlgorithm{
											Name: bccsp.SHA256,
										}),
									},
									"BlockDataHashingStructure": {
										Value: protoutil.MarshalOrPanic(&cb.BlockDataHashingStructure{
											Width: math.MaxUint32,
										}),
									},
									"OrdererAddresses": {
										Value: protoutil.MarshalOrPanic(&cb.OrdererAddresses{
											Addresses: []string{"localhost"},
										}),
									},
								},
							},
						},
					}),
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							Type:      int32(cb.HeaderType_CONFIG),
							ChannelId: channelID,
						}),
					},
				}),
			}),
		},
	}
	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)
	protoutil.InitBlockMetadata(block)

	return block
}

func nonConfigBlock() *cb.Block {
	block := protoutil.NewBlock(0, []byte{})
	block.Data = &cb.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ENDORSER_TRANSACTION),
						}),
					},
				}),
			}),
		},
	}
	block.Header.DataHash, _ = protoutil.BlockDataHash(block.Data)
	protoutil.InitBlockMetadata(block)

	return block
}

func nonConfigEnvelope() *cb.Envelope {
	data := &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Type: int32(cb.HeaderType_ENDORSER_TRANSACTION),
				}),
			},
		}),
	}

	return data
}
