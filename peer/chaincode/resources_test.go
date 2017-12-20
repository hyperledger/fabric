/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/msp"
	common2 "github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/protos/common"
	msp2 "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func TestChaincodeUpdate(t *testing.T) {
	assertPanic := func(expectedError string, f func()) {
		defer func() {
			r := recover()
			if r == nil {
				assert.Fail(t, "Didn't panic")
			}
			if err, isError := r.(error); isError {
				assert.Equal(t, expectedError, err.Error())
			} else {
				assert.Equal(t, expectedError, r)
			}
		}()
		f()
	}

	assertPanic("failed creating signature header", func() {
		_ = (&ccUpdate{
			policy: &common.SignaturePolicyEnvelope{},
			computeDelta: func(original, updated *common.Config) (*common.ConfigUpdate, error) {
				return &common.ConfigUpdate{}, nil
			},
			SignatureSupport: (&mockSigningIdentity{}).thatCreatesSignatureHeader(errors.New("failed creating signature header")),
			oldConfig: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						resourcesconfig.ChaincodesGroupKey: {
							Groups: map[string]*common.ConfigGroup{
								"example02": {},
							},
						},
					},
				},
			},
		}).buildCCUpdateEnvelope()
	})

	assertPanic("failed signing config update: failed signing", func() {
		sId := (&mockSigningIdentity{}).thatCreatesSignatureHeader(&common.SignatureHeader{}).thatSigns(errors.New("failed signing"), 1)
		_ = (&ccUpdate{
			policy: &common.SignaturePolicyEnvelope{},
			computeDelta: func(original, updated *common.Config) (*common.ConfigUpdate, error) {
				return &common.ConfigUpdate{}, nil
			},
			SignatureSupport: sId,
			oldConfig: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						resourcesconfig.ChaincodesGroupKey: {
							Groups: map[string]*common.ConfigGroup{
								"example02": {},
							},
						},
					},
				},
			},
		}).buildCCUpdateEnvelope()
	})

	assertPanic("failed computing delta", func() {
		sId := (&mockSigningIdentity{}).thatCreatesSignatureHeader(&common.SignatureHeader{}).thatSigns([]byte("signature"), 1)
		_ = (&ccUpdate{
			policy: &common.SignaturePolicyEnvelope{},
			computeDelta: func(original, updated *common.Config) (*common.ConfigUpdate, error) {
				return nil, errors.New("failed computing delta")
			},
			SignatureSupport: sId,
			oldConfig: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						resourcesconfig.ChaincodesGroupKey: {
							Groups: map[string]*common.ConfigGroup{
								"example02": {},
							},
						},
					},
				},
			},
		}).buildCCUpdateEnvelope()
	})

	assertPanic("failed signing", func() {
		sId := (&mockSigningIdentity{}).thatCreatesSignatureHeader(&common.SignatureHeader{}).thatSigns([]byte("signature"), 1).thatSigns(errors.New("failed signing"), 1)
		_ = (&ccUpdate{
			policy: &common.SignaturePolicyEnvelope{},
			computeDelta: func(original, updated *common.Config) (*common.ConfigUpdate, error) {
				return &common.ConfigUpdate{}, nil
			},
			SignatureSupport: sId,
			oldConfig: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						resourcesconfig.ChaincodesGroupKey: {
							Groups: map[string]*common.ConfigGroup{
								"example02": {},
							},
						},
					},
				},
			},
		}).buildCCUpdateEnvelope()
	})

	assert.NotPanics(t, func() {
		sId := (&mockSigningIdentity{}).
			thatCreatesSignatureHeader(&common.SignatureHeader{}).
			thatSigns([]byte("signature"), 4)

		ccGrps := []map[string]*common.ConfigGroup{
			{"example02": {}}, {},
		}
		for _, ccGrp := range ccGrps {
			var grp map[string]*common.ConfigGroup
			if len(ccGrp) != 0 {
				grp = ccGrp
			}
			_ = (&ccUpdate{
				policy: &common.SignaturePolicyEnvelope{},
				computeDelta: func(original, updated *common.Config) (*common.ConfigUpdate, error) {
					return &common.ConfigUpdate{}, nil
				},
				SignatureSupport: sId,
				oldConfig: &common.Config{
					ChannelGroup: &common.ConfigGroup{
						Groups: map[string]*common.ConfigGroup{
							resourcesconfig.ChaincodesGroupKey: {
								Groups: grp,
							},
						},
					},
				},
			}).buildCCUpdateEnvelope()
		}
	})
}

func TestProbeChannelVersion(t *testing.T) {
	// Identity isn't serialized properly
	sId := (&mockSigningIdentity{}).thatSerializes(errors.New("failed serializing identity"), 1)
	ec := &mockEndorserClient{}
	_, _, err := fetchResourceConfig(ec, sId, "testchain")
	assert.Equal(t, "failed serializing identity", err.Error())

	// Signing fails
	sId = (&mockSigningIdentity{}).thatSerializes([]byte{1, 2, 3}, 1).thatSigns(errors.New("failed signing"), 1)
	_, _, err = fetchResourceConfig(ec, sId, "testchain")
	assert.Equal(t, "failed signing", err.Error())

	// Peer responds with an error
	sId = (&mockSigningIdentity{}).thatSerializes([]byte{1, 2, 3}, 1).thatSigns([]byte{1, 2, 3}, 1)
	ec.On("ProcessProposal").Return(nil, errors.New("proposal failed")).Once()
	_, _, err = fetchResourceConfig(ec, sId, "testchain")
	assert.Equal(t, "proposal failed", err.Error())

	// Peer responds with a bad status which means this is a v1.0 channel or a v1.0 peer
	sId = (&mockSigningIdentity{}).thatSerializes([]byte{1, 2, 3}, 1).thatSigns([]byte{1, 2, 3}, 1)
	ec.On("ProcessProposal").Return(&peer.ProposalResponse{
		Response: &peer.Response{
			Status: shim.ERROR,
		},
	}, nil).Once()
	ver, _, err := fetchResourceConfig(ec, sId, "testchain")
	assert.NoError(t, err)
	assert.Equal(t, v1, int(ver))

	// Peer responds with an OK but with an invalid payload
	ec.On("ProcessProposal").Return(&peer.ProposalResponse{
		Response: &peer.Response{
			Status:  shim.OK,
			Payload: []byte{1, 2, 3},
		},
	}, nil).Once()
	sId.thatSerializes([]byte{1, 2, 3}, 1).thatSigns([]byte{1, 2, 3}, 1)
	ver, _, err = fetchResourceConfig(ec, sId, "testchain")
	assert.NoError(t, err)
	assert.Equal(t, v1, int(ver))

	// Peer responds with a good config tree, but the chaincode group is empty.
	// should be classified as a v1.1 peer, but running the channel in a v1 mode
	b, _ := proto.Marshal(&peer.ConfigTree{
		ResourcesConfig: &common.Config{
			ChannelGroup: &common.ConfigGroup{},
		},
	})
	ec.On("ProcessProposal").Return(&peer.ProposalResponse{
		Response: &peer.Response{
			Status:  shim.OK,
			Payload: b,
		},
	}, nil).Once()
	sId.thatSerializes([]byte{1, 2, 3}, 1).thatSigns([]byte{1, 2, 3}, 1)
	ver, _, err = fetchResourceConfig(ec, sId, "testchain")
	assert.NoError(t, err)
	assert.Equal(t, v1, int(ver))

	// Peer responds with a good config tree with a chaincode group.
	// Classified as a v1.1 channel
	b, _ = proto.Marshal(&peer.ConfigTree{
		ResourcesConfig: &common.Config{
			ChannelGroup: &common.ConfigGroup{
				Groups: map[string]*common.ConfigGroup{
					resourcesconfig.ChaincodesGroupKey: {},
				},
			},
		},
	})
	ec.On("ProcessProposal").Return(&peer.ProposalResponse{
		Response: &peer.Response{
			Status:  shim.OK,
			Payload: b,
		},
	}, nil).Once()
	sId.thatSerializes([]byte{1, 2, 3}, 1).thatSigns([]byte{1, 2, 3}, 1)
	ver, _, err = fetchResourceConfig(ec, sId, "testchain")
	assert.NoError(t, err)
	assert.Equal(t, v11, int(ver))
}

type configUpdateBroadcastEvent struct {
	*common2.MockBroadcastClient
}

func (cube configUpdateBroadcastEvent) wasSent() bool {
	return cube.Envelope != nil
}

func (cube configUpdateBroadcastEvent) signatureCount() int {
	payload := &common.Payload{}
	proto.Unmarshal(cube.Envelope.Payload, payload)
	update := &common.ConfigUpdateEnvelope{}
	proto.Unmarshal(payload.Data, update)
	return len(update.Signatures)
}

type mockSigningIdentity struct {
	mock.Mock
}

func (m *mockSigningIdentity) ExpiresAt() time.Time {
	panic("implement me")
}

func (m *mockSigningIdentity) GetIdentifier() *msp.IdentityIdentifier {
	panic("implement me")
}

func (m *mockSigningIdentity) GetMSPIdentifier() string {
	panic("implement me")
}

func (m *mockSigningIdentity) Validate() error {
	panic("implement me")
}

func (m *mockSigningIdentity) GetOrganizationalUnits() []*msp.OUIdentifier {
	panic("implement me")
}

func (m *mockSigningIdentity) Verify(msg []byte, sig []byte) error {
	panic("implement me")
}

func (m *mockSigningIdentity) SatisfiesPrincipal(principal *msp2.MSPPrincipal) error {
	panic("implement me")
}

func (m *mockSigningIdentity) GetPublicVersion() msp.Identity {
	panic("implement me")
}

func (m *mockSigningIdentity) Sign(msg []byte) ([]byte, error) {
	args := m.Called()
	signature, err := args.Get(0), args.Error(1)
	if err == nil {
		return signature.([]byte), nil
	}
	return nil, err
}

func (m *mockSigningIdentity) Serialize() ([]byte, error) {
	args := m.Called()
	identity, err := args.Get(0), args.Error(1)
	if err == nil {
		return identity.([]byte), nil
	}
	return nil, err
}

func (m *mockSigningIdentity) NewSignatureHeader() (*common.SignatureHeader, error) {
	args := m.Called()
	hdr, err := args.Get(0), args.Error(1)
	if err == nil {
		return hdr.(*common.SignatureHeader), nil
	}
	return nil, err
}

func (m *mockSigningIdentity) thatSigns(o interface{}, times int) *mockSigningIdentity {
	err, isError := o.(error)
	if isError {
		m.On("Sign").Return(nil, err)
		return m
	}
	m.On("Sign").Return(o.([]byte), nil).Times(times)
	return m
}

func (m *mockSigningIdentity) thatSerializes(o interface{}, times int) *mockSigningIdentity {
	err, isError := o.(error)
	if isError {
		m.On("Serialize").Return(nil, err)
		return m
	}
	m.On("Serialize").Return(o.([]byte), nil).Times(times)
	return m
}

func (m *mockSigningIdentity) thatCreatesSignatureHeader(o interface{}) *mockSigningIdentity {
	err, isError := o.(error)
	if isError {
		m.On("NewSignatureHeader").Return(nil, err)
		return m
	}
	m.On("NewSignatureHeader").Return(o.(*common.SignatureHeader), nil)
	return m
}

type mockEndorserClient struct {
	mock.Mock
}

func (m *mockEndorserClient) ProcessProposal(ctx context.Context, in *peer.SignedProposal, opts ...grpc.CallOption) (*peer.ProposalResponse, error) {
	for _, call := range m.Calls {
		for _, arg := range call.ReturnArguments {
			fmt.Println(arg)
		}
	}
	args := m.Called()
	signature, err := args.Get(0), args.Error(1)
	if err == nil {
		return signature.(*peer.ProposalResponse), nil
	}
	return nil, err
}
