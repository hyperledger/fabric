/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation_test

import (
	"errors"
	"math"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestValidateJoinBlock(t *testing.T) {
	tests := []struct {
		testName             string
		joinBlock            *cb.Block
		expectedChannelID    string
		expectedIsAppChannel bool
		expectedErr          error
	}{
		{
			testName: "Valid system channel join block",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Consortiums": {},
				},
				"my-channel",
			),
			expectedChannelID:    "my-channel",
			expectedIsAppChannel: false,
			expectedErr:          nil,
		},
		{
			testName: "Valid application channel join block",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Application": {},
				},
				"my-channel",
			),
			expectedChannelID:    "my-channel",
			expectedIsAppChannel: true,
			expectedErr:          nil,
		},
		{
			testName:             "Join block not a config block",
			joinBlock:            nonConfigBlock(),
			expectedChannelID:    "",
			expectedIsAppChannel: false,
			expectedErr:          errors.New("block is not a config block"),
		},
		{
			testName: "block ChannelID not valid",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"Consortiums": {},
				},
				"My-Channel",
			),
			expectedChannelID:    "",
			expectedIsAppChannel: false,
			expectedErr:          errors.New("initializing configtx manager failed: bad channel ID: 'My-Channel' contains illegal characters"),
		},
		{
			testName: "Invalid bundle",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{
					"InvalidGroup": {},
				},
				"my-channel",
			),
			expectedChannelID:    "",
			expectedIsAppChannel: false,
			expectedErr:          errors.New("initializing channelconfig failed: Disallowed channel group: "),
		},
		{
			testName: "Join block has no application or consortiums group",
			joinBlock: blockWithGroups(
				map[string]*cb.ConfigGroup{},
				"my-channel",
			),
			expectedChannelID:    "",
			expectedIsAppChannel: false,
			expectedErr:          errors.New("invalid config: must have at least one of application or consortiums"),
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			channelID, isAppChannel, err := channelparticipation.ValidateJoinBlock(test.joinBlock)
			require.Equal(t, test.expectedChannelID, channelID)
			require.Equal(t, test.expectedIsAppChannel, isAppChannel)
			if test.expectedErr != nil {
				require.EqualError(t, err, test.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func blockWithGroups(groups map[string]*cb.ConfigGroup, channelID string) *cb.Block {
	return &cb.Block{
		Data: &cb.BlockData{
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
		},
	}
}

func nonConfigBlock() *cb.Block {
	return &cb.Block{
		Data: &cb.BlockData{
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
		},
	}
}
