/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	gdisc "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	for _, trueOfFalse := range []bool{true, false} {
		conf := Config{
			AuthCacheEnabled:             trueOfFalse,
			AuthCachePurgeRetentionRatio: 0.5,
			AuthCacheMaxSize:             42,
		}
		service := NewService(conf, &mockSupport{})
		require.Equal(t, trueOfFalse, service.auth.conf.enabled)
		require.Equal(t, 42, service.auth.conf.maxCacheSize)
		require.Equal(t, 0.5, service.auth.conf.purgeRetentionRatio)
	}
}

func TestService(t *testing.T) {
	conf := Config{
		AuthCacheEnabled: true,
	}
	ctx := context.Background()
	req := &discovery.Request{
		Authentication: &discovery.AuthInfo{
			ClientIdentity: []byte{1, 2, 3},
		},
		Queries: []*discovery.Query{
			{
				Channel: "noneExistentChannel",
			},
		},
	}
	mockSup := &mockSupport{}
	mockSup.On("ChannelExists", "noneExistentChannel").Return(false)
	mockSup.On("ChannelExists", "channelWithAccessDenied").Return(true)
	mockSup.On("ChannelExists", "channelWithAccessGranted").Return(true)
	mockSup.On("ChannelExists", "channelWithSomeProblem").Return(true)
	mockSup.On("EligibleForService", "channelWithAccessDenied", mock.Anything).Return(errors.New("foo"))
	mockSup.On("EligibleForService", "channelWithAccessGranted", mock.Anything).Return(nil)
	mockSup.On("EligibleForService", "channelWithSomeProblem", mock.Anything).Return(nil)
	ed1 := &discovery.EndorsementDescriptor{
		Chaincode: "cc1",
	}
	ed2 := &discovery.EndorsementDescriptor{
		Chaincode: "cc2",
	}
	ed3 := &discovery.EndorsementDescriptor{
		Chaincode: "cc3",
	}
	mockSup.On("PeersForEndorsement", "unknownCC").Return(nil, errors.New("unknown chaincode"))
	mockSup.On("PeersForEndorsement", "cc1").Return(ed1, nil)
	mockSup.On("PeersForEndorsement", "cc2").Return(ed2, nil)
	mockSup.On("PeersForEndorsement", "cc3").Return(ed3, nil)

	service := NewService(conf, mockSup)

	// Scenario I: Channel does not exist
	resp, err := service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Equal(t, wrapResult(&discovery.Error{Content: "access denied"}), resp)

	// Scenario II: Channel does not exist
	req.Queries[0].Channel = "channelWithAccessDenied"
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Equal(t, wrapResult(&discovery.Error{Content: "access denied"}), resp)

	// Scenario III: Request with nil query
	req.Queries[0].Channel = "channelWithAccessGranted"
	req.Queries[0].Query = nil
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "unknown or missing request type")

	// Scenario IV: Request payload is invalid
	signedRequest := toSignedRequest(req)
	// Corrupt the payload by appending a zero byte at its end
	signedRequest.Payload = append(signedRequest.Payload, 0)
	resp, err = service.Discover(ctx, signedRequest)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "failed parsing request")

	// Scenario V: Request a CC query with no chaincodes at all
	req.Queries[0].Query = &discovery.Query_CcQuery{
		CcQuery: &discovery.ChaincodeQuery{
			Interests: []*peer.ChaincodeInterest{
				{},
			},
		},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "chaincode interest must contain at least one chaincode")

	// Scenario VI: Request a CC query with no interests at all
	req.Queries[0].Query = &discovery.Query_CcQuery{
		CcQuery: &discovery.ChaincodeQuery{
			Interests: []*peer.ChaincodeInterest{},
		},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "chaincode query must have at least one chaincode interest")

	// Scenario VII: Request a CC query with a chaincode name that is empty
	req.Queries[0].Query = &discovery.Query_CcQuery{
		CcQuery: &discovery.ChaincodeQuery{
			Interests: []*peer.ChaincodeInterest{{
				Chaincodes: []*peer.ChaincodeCall{{
					Name: "",
				}},
			}},
		},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "chaincode name in interest cannot be empty")

	// Scenario VIII: Request with a CC query where one chaincode is unavailable
	req.Queries[0].Query = &discovery.Query_CcQuery{
		CcQuery: &discovery.ChaincodeQuery{
			Interests: []*peer.ChaincodeInterest{
				{
					Chaincodes: []*peer.ChaincodeCall{{Name: "unknownCC"}},
				},
				{
					Chaincodes: []*peer.ChaincodeCall{{Name: "cc1"}},
				},
			},
		},
	}

	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "failed constructing descriptor")
	require.Contains(t, resp.Results[0].GetError().Content, "unknownCC")

	// Scenario IX: Request with a CC query where all are available
	req.Queries[0].Query = &discovery.Query_CcQuery{
		CcQuery: &discovery.ChaincodeQuery{
			Interests: []*peer.ChaincodeInterest{
				{
					Chaincodes: []*peer.ChaincodeCall{{Name: "cc1"}},
				},
				{
					Chaincodes: []*peer.ChaincodeCall{{Name: "cc2"}},
				},
				{
					Chaincodes: []*peer.ChaincodeCall{{Name: "cc3"}},
				},
			},
		},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	expected := wrapResult(&discovery.ChaincodeQueryResult{
		Content: []*discovery.EndorsementDescriptor{ed1, ed2, ed3},
	})
	require.Equal(t, expected, resp)

	// Scenario X: Request with a config query
	mockSup.On("Config", mock.Anything).Return(nil, errors.New("failed fetching config")).Once()
	req.Queries[0].Query = &discovery.Query_ConfigQuery{
		ConfigQuery: &discovery.ConfigQuery{},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "failed fetching config for channel channelWithAccessGranted")

	// Scenario XI: Request with a config query
	mockSup.On("Config", mock.Anything).Return(&discovery.ConfigResult{}, nil).Once()
	req.Queries[0].Query = &discovery.Query_ConfigQuery{
		ConfigQuery: &discovery.ConfigQuery{},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.NotNil(t, resp.Results[0].GetConfigResult())

	// Scenario XII: Request with a membership query
	// Peers in membership view: { p0, p1, p2, p3}
	// Peers in channel view: {p1, p2, p4}
	// So that means that the returned peers for the channel should be the intersection
	// which is: {p1, p2}, but the returned peers for the local query should be
	// simply the membership view.
	peersInMembershipView := gdisc.Members{
		aliveMsg(0), aliveMsg(1), aliveMsg(2), aliveMsg(3),
	}
	peersInChannelView := gdisc.Members{
		stateInfoMsg(1), stateInfoMsg(2), stateInfoMsg(4),
	}
	// EligibleForService for an "empty" channel
	mockSup.On("EligibleForService", "", mock.Anything).Return(nil).Once()
	mockSup.On("PeersAuthorizedByCriteria", gcommon.ChannelID("channelWithAccessGranted")).Return(peersInChannelView, nil).Once()
	mockSup.On("PeersAuthorizedByCriteria", gcommon.ChannelID("channelWithSomeProblem")).Return(nil, errors.New("an error occurred")).Once()
	mockSup.On("Peers").Return(peersInMembershipView).Twice()
	mockSup.On("IdentityInfo").Return(api.PeerIdentitySet{
		idInfo(0, "O2"), idInfo(1, "O2"), idInfo(2, "O3"),
		idInfo(3, "O3"), idInfo(4, "O3"),
	}).Twice()

	req.Queries = []*discovery.Query{
		{
			Channel: "channelWithAccessGranted",
			Query: &discovery.Query_PeerQuery{
				PeerQuery: &discovery.PeerMembershipQuery{},
			},
		},
		{
			Query: &discovery.Query_LocalPeers{
				LocalPeers: &discovery.LocalPeerQuery{},
			},
		},
		{
			Channel: "channelWithSomeProblem",
			Query: &discovery.Query_PeerQuery{
				PeerQuery: &discovery.PeerMembershipQuery{
					Filter: &peer.ChaincodeInterest{},
				},
			},
		},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	expectedChannelResponse := &discovery.PeerMembershipResult{
		PeersByOrg: map[string]*discovery.Peers{
			"O2": {
				Peers: []*discovery.Peer{
					{
						Identity:       idInfo(1, "O2").Identity,
						StateInfo:      stateInfoMsg(1).Envelope,
						MembershipInfo: aliveMsg(1).Envelope,
					},
				},
			},
			"O3": {
				Peers: []*discovery.Peer{
					{
						Identity:       idInfo(2, "O3").Identity,
						StateInfo:      stateInfoMsg(2).Envelope,
						MembershipInfo: aliveMsg(2).Envelope,
					},
				},
			},
		},
	}
	expectedLocalResponse := &discovery.PeerMembershipResult{
		PeersByOrg: map[string]*discovery.Peers{
			"O2": {
				Peers: []*discovery.Peer{
					{
						Identity:       idInfo(0, "O2").Identity,
						MembershipInfo: aliveMsg(0).Envelope,
					},
					{
						Identity:       idInfo(1, "O2").Identity,
						MembershipInfo: aliveMsg(1).Envelope,
					},
				},
			},
			"O3": {
				Peers: []*discovery.Peer{
					{
						Identity:       idInfo(2, "O3").Identity,
						MembershipInfo: aliveMsg(2).Envelope,
					},
					{
						Identity:       idInfo(3, "O3").Identity,
						MembershipInfo: aliveMsg(3).Envelope,
					},
				},
			},
		},
	}

	require.Len(t, resp.Results, 3)
	require.Len(t, resp.Results[0].GetMembers().PeersByOrg, 2)
	require.Len(t, resp.Results[1].GetMembers().PeersByOrg, 2)
	require.Equal(t, "an error occurred", resp.Results[2].GetError().Content)

	for org, responsePeers := range resp.Results[0].GetMembers().PeersByOrg {
		err := peers(expectedChannelResponse.PeersByOrg[org].Peers).compare(peers(responsePeers.Peers))
		require.NoError(t, err)
	}
	for org, responsePeers := range resp.Results[1].GetMembers().PeersByOrg {
		err := peers(expectedLocalResponse.PeersByOrg[org].Peers).compare(peers(responsePeers.Peers))
		require.NoError(t, err)
	}

	// Scenario XIII: The client is eligible for channel queries but not for channel-less
	// since it's not an admin. It sends a query for a channel-less query but puts a channel in the query.
	// It should fail because channel-less query types cannot have a channel configured in them.
	req.Queries = []*discovery.Query{
		{
			Channel: "channelWithAccessGranted",
			Query: &discovery.Query_LocalPeers{
				LocalPeers: &discovery.LocalPeerQuery{},
			},
		},
	}
	resp, err = service.Discover(ctx, toSignedRequest(req))
	require.NoError(t, err)
	require.Contains(t, resp.Results[0].GetError().Content, "unknown or missing request type")
}

func TestValidateStructure(t *testing.T) {
	extractHash := func(ctx context.Context) []byte {
		return nil
	}
	// Scenarios I-V without TLS, scenarios VI onwards TLS

	// Scenario I: Nil request
	res, err := validateStructure(context.Background(), nil, false, extractHash)
	require.Nil(t, res)
	require.Equal(t, "nil request", err.Error())

	// Scenario II: Malformed envelope
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{
		Payload: []byte{1, 2, 3},
	}, false, extractHash)
	require.Nil(t, res)
	require.Contains(t, err.Error(), "failed parsing request")

	// Scenario III: Empty request
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{}, false, extractHash)
	require.Nil(t, res)
	require.Equal(t, "access denied, no authentication info in request", err.Error())

	// Scenario IV: request without a client identity
	req := &discovery.Request{
		Authentication: &discovery.AuthInfo{},
	}
	b, _ := proto.Marshal(req)
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{
		Payload: b,
	}, false, extractHash)
	require.Nil(t, res)
	require.Equal(t, "access denied, client identity wasn't supplied", err.Error())

	// Scenario V: request with a client identity, should succeed because no TLS is used
	req = &discovery.Request{
		Authentication: &discovery.AuthInfo{
			ClientIdentity: []byte{1, 2, 3},
		},
	}
	b, _ = proto.Marshal(req)
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{
		Payload: b,
	}, false, extractHash)
	require.NoError(t, err)
	// Ensure returned request is as before serialization to bytes
	require.True(t, proto.Equal(req, res))

	// Scenario VI: request with a client identity but with TLS enabled but client doesn't send a TLS cert
	req = &discovery.Request{
		Authentication: &discovery.AuthInfo{
			ClientIdentity: []byte{1, 2, 3},
		},
	}
	b, _ = proto.Marshal(req)
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{
		Payload: b,
	}, true, extractHash)
	require.Nil(t, res)
	require.Equal(t, "client didn't send a TLS certificate", err.Error())

	// Scenario VII: request with a client identity and with TLS enabled but the TLS cert hash doesn't match
	// the computed one
	extractHash = func(ctx context.Context) []byte {
		return []byte{1, 2}
	}
	req = &discovery.Request{
		Authentication: &discovery.AuthInfo{
			ClientIdentity:    []byte{1, 2, 3},
			ClientTlsCertHash: []byte{1, 2, 3},
		},
	}
	b, _ = proto.Marshal(req)
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{
		Payload: b,
	}, true, extractHash)
	require.Nil(t, res)
	require.Equal(t, "client claimed TLS hash doesn't match computed TLS hash from gRPC stream", err.Error())

	// Scenario VIII: request with a client identity and with TLS enabled and the TLS cert hash doesn't match
	// the computed one
	extractHash = func(ctx context.Context) []byte {
		return []byte{1, 2, 3}
	}
	req = &discovery.Request{
		Authentication: &discovery.AuthInfo{
			ClientIdentity:    []byte{1, 2, 3},
			ClientTlsCertHash: []byte{1, 2, 3},
		},
	}
	b, _ = proto.Marshal(req)
	res, err = validateStructure(context.Background(), &discovery.SignedRequest{
		Payload: b,
	}, true, extractHash)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestValidateCCQuery(t *testing.T) {
	err := validateCCQuery(&discovery.ChaincodeQuery{
		Interests: []*peer.ChaincodeInterest{
			nil,
		},
	})
	require.Equal(t, "chaincode interest is nil", err.Error())
}

func wrapResult(responses ...interface{}) *discovery.Response {
	response := &discovery.Response{}
	for _, res := range responses {
		response.Results = append(response.Results, wrapQueryResult(res))
	}
	return response
}

func wrapQueryResult(res interface{}) *discovery.QueryResult {
	if err, isErr := res.(*discovery.Error); isErr {
		return &discovery.QueryResult{
			Result: &discovery.QueryResult_Error{
				Error: err,
			},
		}
	}
	if ccRes, isCCQuery := res.(*discovery.ChaincodeQueryResult); isCCQuery {
		return &discovery.QueryResult{
			Result: &discovery.QueryResult_CcQueryRes{
				CcQueryRes: ccRes,
			},
		}
	}
	if membRes, isMembershipQuery := res.(*discovery.PeerMembershipResult); isMembershipQuery {
		return &discovery.QueryResult{
			Result: &discovery.QueryResult_Members{
				Members: membRes,
			},
		}
	}
	if confRes, isConfQuery := res.(*discovery.ConfigResult); isConfQuery {
		return &discovery.QueryResult{
			Result: &discovery.QueryResult_ConfigResult{
				ConfigResult: confRes,
			},
		}
	}
	panic(fmt.Sprint("invalid type:", reflect.TypeOf(res)))
}

func toSignedRequest(req *discovery.Request) *discovery.SignedRequest {
	b, _ := proto.Marshal(req)
	return &discovery.SignedRequest{
		Payload: b,
	}
}

type mockSupport struct {
	mock.Mock
}

func (ms *mockSupport) ConfigSequence(channel string) uint64 {
	return 0
}

func (ms *mockSupport) IdentityInfo() api.PeerIdentitySet {
	return ms.Called().Get(0).(api.PeerIdentitySet)
}

func (ms *mockSupport) ChannelExists(channel string) bool {
	return ms.Called(channel).Get(0).(bool)
}

func (ms *mockSupport) PeersOfChannel(channel gcommon.ChannelID) gdisc.Members {
	panic("not implemented")
}

func (ms *mockSupport) Peers() gdisc.Members {
	return ms.Called().Get(0).(gdisc.Members)
}

func (ms *mockSupport) PeersForEndorsement(channel gcommon.ChannelID, interest *peer.ChaincodeInterest) (*discovery.EndorsementDescriptor, error) {
	cc := interest.Chaincodes[0].Name
	args := ms.Called(cc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*discovery.EndorsementDescriptor), args.Error(1)
}

func (ms *mockSupport) PeersAuthorizedByCriteria(chainID gcommon.ChannelID, interest *peer.ChaincodeInterest) (gdisc.Members, error) {
	args := ms.Called(chainID)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(gdisc.Members), args.Error(1)
}

func (*mockSupport) Chaincodes(id gcommon.ChannelID) []*gossip.Chaincode {
	panic("implement me")
}

func (ms *mockSupport) EligibleForService(channel string, data protoutil.SignedData) error {
	return ms.Called(channel, data).Error(0)
}

func (ms *mockSupport) Config(channel string) (*discovery.ConfigResult, error) {
	args := ms.Called(channel)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*discovery.ConfigResult), args.Error(1)
}

func idInfo(id int, org string) api.PeerIdentityInfo {
	endpoint := fmt.Sprintf("p%d", id)
	return api.PeerIdentityInfo{
		PKIId:        gcommon.PKIidType(endpoint),
		Organization: api.OrgIdentityType(org),
		Identity:     api.PeerIdentityType(endpoint),
	}
}

func stateInfoMsg(id int) gdisc.NetworkMember {
	endpoint := fmt.Sprintf("p%d", id)
	pkiID := gcommon.PKIidType(endpoint)
	si := &gossip.StateInfo{
		PkiId: pkiID,
	}
	gm := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: si,
		},
	}
	sm, _ := protoext.NoopSign(gm)
	return gdisc.NetworkMember{
		PKIid:    pkiID,
		Envelope: sm.Envelope,
	}
}

func aliveMsg(id int) gdisc.NetworkMember {
	endpoint := fmt.Sprintf("p%d", id)
	pkiID := gcommon.PKIidType(endpoint)
	am := &gossip.AliveMessage{
		Membership: &gossip.Member{
			PkiId:    pkiID,
			Endpoint: endpoint,
		},
	}
	gm := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: am,
		},
	}
	sm, _ := protoext.NoopSign(gm)
	return gdisc.NetworkMember{
		PKIid:    pkiID,
		Endpoint: endpoint,
		Envelope: sm.Envelope,
	}
}

type peers []*discovery.Peer

func (ps peers) exists(p *discovery.Peer) error {
	var found bool
	for _, q := range ps {
		if reflect.DeepEqual(*p, *q) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%v wasn't found in %v", ps, p)
	}
	return nil
}

func (ps peers) compare(otherPeers peers) error {
	if len(ps) != len(otherPeers) {
		return fmt.Errorf("size mismatch: %d, %d", len(ps), len(otherPeers))
	}

	for _, p := range otherPeers {
		if err := ps.exists(p); err != nil {
			return err
		}
	}

	for _, p := range ps {
		if err := otherPeers.exists(p); err != nil {
			return err
		}
	}
	return nil
}
