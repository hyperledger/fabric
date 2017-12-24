/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"bytes"
	"encoding/hex"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	common2 "github.com/hyperledger/fabric/gossip/common"
	discovery2 "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var (
	logger = flogging.MustGetLogger("discovery")
)

var accessDenied = wrapError(errors.New("access denied"))

// certHashExtractor extracts the TLS certificate from a given context
// and returns its hash
type certHashExtractor func(ctx context.Context) []byte

// dispatcher defines a function that dispatches a query
type dispatcher func(q *discovery.Query) *discovery.QueryResult

type service struct {
	dispatchers map[discovery.QueryType]dispatcher
	auth        *authCache
	tlsEnabled  bool
	Support
}

// NewService creates a new discovery service instance
func NewService(tlsEnabled bool, sup Support) *service {
	s := &service{
		tlsEnabled: tlsEnabled,
		auth: newAuthCache(sup, authCacheConfig{
			maxCacheSize:        defaultMaxCacheSize,
			purgeRetentionRatio: defaultRetentionRatio,
		}),
		Support: sup,
	}
	s.dispatchers = map[discovery.QueryType]dispatcher{
		discovery.ConfigQueryType:         s.configQuery,
		discovery.ChaincodeQueryType:      s.chaincodeQuery,
		discovery.PeerMembershipQueryType: s.membershipQuery,
	}
	return s
}

func (s *service) Discover(ctx context.Context, request *discovery.SignedRequest) (*discovery.Response, error) {
	addr := util.ExtractRemoteAddress(ctx)
	req, err := validateStructure(ctx, request, addr, s.tlsEnabled, comm.ExtractCertificateHashFromContext)
	if err != nil {
		return nil, err
	}

	var res []*discovery.QueryResult
	for _, q := range req.Queries {
		res = append(res, s.processQuery(q, request, req.Authentication.ClientIdentity, addr))
	}
	return &discovery.Response{
		Results: res,
	}, nil
}

func (s *service) processQuery(query *discovery.Query, request *discovery.SignedRequest, identity []byte, addr string) *discovery.QueryResult {
	if !s.ChannelExists(query.Channel) {
		logger.Warning("got query for channel", query.Channel, "from", addr, "but it doesn't exist")
		return accessDenied
	}
	if err := s.auth.EligibleForService(query.Channel, common.SignedData{
		Data:      request.Payload,
		Signature: request.Signature,
		Identity:  identity,
	}); err != nil {
		logger.Warning("got query for channel", query.Channel, "from", addr, "but it isn't eligible:", err)
		return accessDenied
	}
	return s.dispatch(query)

}

func (s *service) dispatch(q *discovery.Query) *discovery.QueryResult {
	disp, exists := s.dispatchers[q.GetType()]
	if !exists {
		return wrapError(errors.New("unknown or missing request type"))
	}
	return disp(q)
}

func (s *service) chaincodeQuery(q *discovery.Query) *discovery.QueryResult {
	var descriptors []*discovery.EndorsementDescriptor
	for _, cc := range q.GetCcQuery().Chaincodes {
		desc, err := s.PeersForEndorsement(cc, common2.ChainID(q.Channel))
		if err != nil {
			logger.Errorf("Failed constructing descriptor for chaincode %s,: %v", cc, err)
			return wrapError(errors.Errorf("failed constructing descriptor for chaincode %s, version %s", cc))
		}
		descriptors = append(descriptors, desc)
	}

	return &discovery.QueryResult{
		Result: &discovery.QueryResult_CcQueryRes{
			CcQueryRes: &discovery.ChaincodeQueryResult{
				Content: descriptors,
			},
		},
	}
}

func (s *service) configQuery(q *discovery.Query) *discovery.QueryResult {
	conf, err := s.Config(q.Channel)
	if err != nil {
		logger.Errorf("Failed fetching config for channel %s: %v", q.Channel, err)
		return wrapError(errors.Errorf("failed fetching config for channel %s", q.Channel))
	}
	return &discovery.QueryResult{
		Result: &discovery.QueryResult_ConfigResult{
			ConfigResult: conf,
		},
	}
}

func (s *service) membershipQuery(q *discovery.Query) *discovery.QueryResult {
	peersByOrg := make(map[string]*discovery.Peers)
	res := &discovery.QueryResult{
		Result: &discovery.QueryResult_Members{
			Members: &discovery.PeerMembershipResult{
				PeersByOrg: peersByOrg,
			},
		},
	}

	peerAliveInfo := discovery2.Members(s.Peers()).ByID()
	chanPeers := s.PeersOfChannel(common2.ChainID(q.Channel))
	peerChannelInfo := discovery2.Members(chanPeers).ByID()
	for org, peerIdentities := range s.IdentityInfo().ByOrg() {
		peersForCurrentOrg := &discovery.Peers{}
		peersByOrg[org] = peersForCurrentOrg
		for _, id := range peerIdentities {
			// Check peer exists in channel
			chanInfo, exists := peerChannelInfo[string(id.PKIId)]
			if !exists {
				continue
			}
			// Check peer exists in alive membership view
			aliveInfo, exists := peerAliveInfo[string(id.PKIId)]
			if !exists {
				continue
			}
			peersForCurrentOrg.Peers = append(peersForCurrentOrg.Peers, &discovery.Peer{
				Identity:       id.Identity,
				StateInfo:      chanInfo.Envelope,
				MembershipInfo: aliveInfo.Envelope,
			})
		}
	}
	return res
}

// validateStructure validates that the request contains all the needed fields and that they are computed correctly
func validateStructure(ctx context.Context, request *discovery.SignedRequest, addr string, tlsEnabled bool, certHashFromContext certHashExtractor) (*discovery.Request, error) {
	if request == nil {
		return nil, errors.New("nil request")
	}
	req, err := request.ToRequest()
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing request")
	}
	if req.Authentication == nil {
		return nil, errors.New("access denied, no authentication info in request")
	}
	if len(req.Authentication.ClientIdentity) == 0 {
		return nil, errors.New("access denied, client identity wasn't supplied")
	}
	logger.Debug("Received request from", addr)
	if !tlsEnabled {
		return req, nil
	}
	computedHash := certHashFromContext(ctx)
	if len(computedHash) == 0 {
		return nil, errors.New("client didn't send a TLS certificate")
	}
	if !bytes.Equal(computedHash, req.Authentication.ClientTlsCertHash) {
		claimed := hex.EncodeToString(req.Authentication.ClientTlsCertHash)
		logger.Warningf("client claimed TLS hash %s doesn't match computed TLS hash from gRPC stream %s", claimed, hex.EncodeToString(computedHash))
		return nil, errors.New("client claimed TLS hash doesn't match computed TLS hash from gRPC stream")
	}
	return req, nil
}

func wrapError(err error) *discovery.QueryResult {
	return &discovery.QueryResult{
		Result: &discovery.QueryResult_Error{
			Error: &discovery.Error{
				Content: err.Error(),
			},
		},
	}
}
