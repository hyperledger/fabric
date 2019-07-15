/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	fabricdisc "github.com/hyperledger/fabric/discovery"
	"github.com/hyperledger/fabric/discovery/endorsement"
	"github.com/hyperledger/fabric/gossip/api"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	gdisc "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	signerCacheSize uint = 1
)

var (
	ctx = context.Background()

	orgCombinationsThatSatisfyPolicy = [][]string{
		{"A", "B"}, {"C"}, {"A", "D"},
	}

	orgCombinationsThatSatisfyPolicy2 = [][]string{
		{"B", "D"},
	}

	expectedOrgCombinations = []map[string]struct{}{
		{
			"A": {},
			"B": {},
		},
		{
			"C": {},
		},
		{
			"A": {},
			"D": {},
		},
	}

	expectedOrgCombinations2 = []map[string]struct{}{
		{
			"B": {},
			"C": {},
			"D": {},
		},
	}

	cc = &gossip.Chaincode{
		Name:    "mycc",
		Version: "1.0",
	}

	cc2 = &gossip.Chaincode{
		Name:    "mycc2",
		Version: "1.0",
	}

	cc3 = &gossip.Chaincode{
		Name:    "mycc3",
		Version: "1.0",
	}

	propertiesWithChaincodes = &gossip.Properties{
		Chaincodes: []*gossip.Chaincode{cc, cc2, cc3},
	}

	expectedConf = &discovery.ConfigResult{
		Msps: map[string]*msp.FabricMSPConfig{
			"A": {},
			"B": {},
			"C": {},
			"D": {},
		},
		Orderers: map[string]*discovery.Endpoints{
			"A": {},
			"B": {},
		},
	}

	channelPeersWithChaincodes = gdisc.Members{
		newPeer(0, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(1, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(2, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(3, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(4, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(5, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(6, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
		newPeer(7, stateInfoMessage(cc, cc2), propertiesWithChaincodes).NetworkMember,
	}

	channelPeersWithoutChaincodes = gdisc.Members{
		newPeer(0, stateInfoMessage(), nil).NetworkMember,
		newPeer(1, stateInfoMessage(), nil).NetworkMember,
		newPeer(2, stateInfoMessage(), nil).NetworkMember,
		newPeer(3, stateInfoMessage(), nil).NetworkMember,
		newPeer(4, stateInfoMessage(), nil).NetworkMember,
		newPeer(5, stateInfoMessage(), nil).NetworkMember,
		newPeer(6, stateInfoMessage(), nil).NetworkMember,
		newPeer(7, stateInfoMessage(), nil).NetworkMember,
	}

	channelPeersWithDifferentLedgerHeights = gdisc.Members{
		newPeer(0, stateInfoMessageWithHeight(100, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(1, stateInfoMessageWithHeight(106, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(2, stateInfoMessageWithHeight(107, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(3, stateInfoMessageWithHeight(108, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(4, stateInfoMessageWithHeight(101, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(5, stateInfoMessageWithHeight(108, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(6, stateInfoMessageWithHeight(110, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(7, stateInfoMessageWithHeight(110, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(8, stateInfoMessageWithHeight(100, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(9, stateInfoMessageWithHeight(107, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(10, stateInfoMessageWithHeight(110, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(11, stateInfoMessageWithHeight(111, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(12, stateInfoMessageWithHeight(105, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(13, stateInfoMessageWithHeight(103, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(14, stateInfoMessageWithHeight(109, cc3), propertiesWithChaincodes).NetworkMember,
		newPeer(15, stateInfoMessageWithHeight(111, cc3), propertiesWithChaincodes).NetworkMember,
	}

	membershipPeers = gdisc.Members{
		newPeer(0, aliveMessage(0), nil).NetworkMember,
		newPeer(1, aliveMessage(1), nil).NetworkMember,
		newPeer(2, aliveMessage(2), nil).NetworkMember,
		newPeer(3, aliveMessage(3), nil).NetworkMember,
		newPeer(4, aliveMessage(4), nil).NetworkMember,
		newPeer(5, aliveMessage(5), nil).NetworkMember,
		newPeer(6, aliveMessage(6), nil).NetworkMember,
		newPeer(7, aliveMessage(7), nil).NetworkMember,
		newPeer(8, aliveMessage(8), nil).NetworkMember,
		newPeer(9, aliveMessage(9), nil).NetworkMember,
		newPeer(10, aliveMessage(10), nil).NetworkMember,
		newPeer(11, aliveMessage(11), nil).NetworkMember,
		newPeer(12, aliveMessage(12), nil).NetworkMember,
		newPeer(13, aliveMessage(13), nil).NetworkMember,
		newPeer(14, aliveMessage(14), nil).NetworkMember,
		newPeer(15, aliveMessage(15), nil).NetworkMember,
	}

	peerIdentities = api.PeerIdentitySet{
		peerIdentity("A", 0),
		peerIdentity("A", 1),
		peerIdentity("B", 2),
		peerIdentity("B", 3),
		peerIdentity("C", 4),
		peerIdentity("C", 5),
		peerIdentity("D", 6),
		peerIdentity("D", 7),
		peerIdentity("A", 8),
		peerIdentity("A", 9),
		peerIdentity("B", 10),
		peerIdentity("B", 11),
		peerIdentity("C", 12),
		peerIdentity("C", 13),
		peerIdentity("D", 14),
		peerIdentity("D", 15),
	}

	resultsWithoutEnvelopes = &discovery.QueryResult_CcQueryRes{
		CcQueryRes: &discovery.ChaincodeQueryResult{
			Content: []*discovery.EndorsementDescriptor{
				{
					Chaincode: "mycc",
					EndorsersByGroups: map[string]*discovery.Peers{
						"A": {
							Peers: []*discovery.Peer{
								{},
							},
						},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{},
						},
					},
				},
			},
		},
	}

	resultsWithEnvelopesButWithInsufficientPeers = &discovery.QueryResult_CcQueryRes{
		CcQueryRes: &discovery.ChaincodeQueryResult{
			Content: []*discovery.EndorsementDescriptor{
				{
					Chaincode: "mycc",
					EndorsersByGroups: map[string]*discovery.Peers{
						"A": {
							Peers: []*discovery.Peer{
								{
									StateInfo:      stateInfoMessage(),
									MembershipInfo: aliveMessage(0),
									Identity:       peerIdentity("A", 0).Identity,
								},
							},
						},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{
								"A": 2,
							},
						},
					},
				},
			},
		},
	}

	resultsWithEnvelopesButWithMismatchedLayout = &discovery.QueryResult_CcQueryRes{
		CcQueryRes: &discovery.ChaincodeQueryResult{
			Content: []*discovery.EndorsementDescriptor{
				{
					Chaincode: "mycc",
					EndorsersByGroups: map[string]*discovery.Peers{
						"A": {
							Peers: []*discovery.Peer{
								{
									StateInfo:      stateInfoMessage(),
									MembershipInfo: aliveMessage(0),
									Identity:       peerIdentity("A", 0).Identity,
								},
							},
						},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{
								"B": 2,
							},
						},
					},
				},
			},
		},
	}
)

func loadFileOrPanic(file string) []byte {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}
	return b
}

func createGRPCServer(t *testing.T) *comm.GRPCServer {
	serverCert := loadFileOrPanic(filepath.Join("testdata", "server", "cert.pem"))
	serverKey := loadFileOrPanic(filepath.Join("testdata", "server", "key.pem"))
	s, err := comm.NewGRPCServer("localhost:0", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			UseTLS:      true,
			Certificate: serverCert,
			Key:         serverKey,
		},
	})
	assert.NoError(t, err)
	return s
}

func createConnector(t *testing.T, certificate tls.Certificate, targetPort int) func() (*grpc.ClientConn, error) {
	caCert := loadFileOrPanic(filepath.Join("testdata", "server", "ca.pem"))
	tlsConf := &tls.Config{
		RootCAs:      x509.NewCertPool(),
		Certificates: []tls.Certificate{certificate},
	}
	tlsConf.RootCAs.AppendCertsFromPEM(caCert)

	addr := fmt.Sprintf("localhost:%d", targetPort)
	return func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(addr, grpc.WithBlock(), grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
		assert.NoError(t, err)
		if err != nil {
			panic(err)
		}
		return conn, nil
	}
}

func createDiscoveryService(sup *mockSupport) discovery.DiscoveryServer {
	conf := fabricdisc.Config{TLS: true}
	mdf := &ccMetadataFetcher{}
	pe := &principalEvaluator{}
	pf := &policyFetcher{}

	sigPol, _ := cauthdsl.FromString("OR(AND('A.member', 'B.member'), 'C.member', AND('A.member', 'D.member'))")
	polBytes, _ := proto.Marshal(sigPol)
	mdf.On("Metadata", "mycc").Return(&chaincode.Metadata{
		Policy:  polBytes,
		Name:    "mycc",
		Version: "1.0",
		Id:      []byte{1, 2, 3},
	})

	pf.On("PolicyByChaincode", "mycc").Return(&inquireablePolicy{
		orgCombinations: orgCombinationsThatSatisfyPolicy,
	})

	sigPol, _ = cauthdsl.FromString("AND('B.member', 'C.member')")
	polBytes, _ = proto.Marshal(sigPol)
	mdf.On("Metadata", "mycc2").Return(&chaincode.Metadata{
		Policy:  polBytes,
		Name:    "mycc2",
		Version: "1.0",
		Id:      []byte{1, 2, 3},
		CollectionsConfig: buildCollectionConfig(map[string][]*msp.MSPPrincipal{
			"col": {memberPrincipal("B"), memberPrincipal("C"), memberPrincipal("D")},
		}),
	})

	pf.On("PolicyByChaincode", "mycc2").Return(&inquireablePolicy{
		orgCombinations: orgCombinationsThatSatisfyPolicy2,
	})

	sigPol, _ = cauthdsl.FromString("AND('A.member', 'B.member', 'C.member', 'D.member')")
	polBytes, _ = proto.Marshal(sigPol)
	mdf.On("Metadata", "mycc3").Return(&chaincode.Metadata{
		Policy:  polBytes,
		Name:    "mycc3",
		Version: "1.0",
		Id:      []byte{1, 2, 3},
	})

	pf.On("PolicyByChaincode", "mycc3").Return(&inquireablePolicy{
		orgCombinations: [][]string{{"A", "B", "C", "D"}},
	})

	sup.On("Config", "mychannel").Return(expectedConf)
	sup.On("Peers").Return(membershipPeers)
	sup.endorsementAnalyzer = endorsement.NewEndorsementAnalyzer(sup, pf, pe, mdf)
	sup.On("IdentityInfo").Return(peerIdentities)
	return fabricdisc.NewService(conf, sup)
}

func TestClient(t *testing.T) {
	clientCert := loadFileOrPanic(filepath.Join("testdata", "client", "cert.pem"))
	clientKey := loadFileOrPanic(filepath.Join("testdata", "client", "key.pem"))
	clientTLSCert, err := tls.X509KeyPair(clientCert, clientKey)
	assert.NoError(t, err)
	server := createGRPCServer(t)
	sup := &mockSupport{}
	service := createDiscoveryService(sup)
	discovery.RegisterDiscoveryServer(server.Server(), service)
	go server.Start()

	_, portStr, _ := net.SplitHostPort(server.Address())
	port, _ := strconv.ParseInt(portStr, 10, 64)
	connect := createConnector(t, clientTLSCert, int(port))

	signer := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	authInfo := &discovery.AuthInfo{
		ClientIdentity:    []byte{1, 2, 3},
		ClientTlsCertHash: util.ComputeSHA256(clientTLSCert.Certificate[0]),
	}
	cl := NewClient(connect, signer, signerCacheSize)

	sup.On("PeersOfChannel").Return(channelPeersWithoutChaincodes).Times(2)
	req := NewRequest()
	req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddLocalPeersQuery().AddEndorsersQuery(interest("mycc"))
	r, err := cl.Send(ctx, req, authInfo)
	assert.NoError(t, err)

	t.Run("Channel mismatch", func(t *testing.T) {
		// Check behavior for channels that we didn't query for.
		fakeChannel := r.ForChannel("fakeChannel")
		peers, err := fakeChannel.Peers()
		assert.Equal(t, ErrNotFound, err)
		assert.Nil(t, peers)

		endorsers, err := fakeChannel.Endorsers(ccCall("mycc"), NoFilter)
		assert.Equal(t, ErrNotFound, err)
		assert.Nil(t, endorsers)

		conf, err := fakeChannel.Config()
		assert.Equal(t, ErrNotFound, err)
		assert.Nil(t, conf)
	})

	t.Run("Peer membership query", func(t *testing.T) {
		// Check response for the correct channel
		mychannel := r.ForChannel("mychannel")
		conf, err := mychannel.Config()
		assert.NoError(t, err)
		assert.Equal(t, expectedConf.Msps, conf.Msps)
		assert.Equal(t, expectedConf.Orderers, conf.Orderers)
		peers, err := mychannel.Peers()
		assert.NoError(t, err)
		// We should see all peers as provided above
		assert.Len(t, peers, 8)
		// Check response for peers when doing a local query
		peers, err = r.ForLocal().Peers()
		assert.NoError(t, err)
		assert.Len(t, peers, len(peerIdentities))
	})

	t.Run("Endorser query without chaincode installed", func(t *testing.T) {
		mychannel := r.ForChannel("mychannel")
		endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
		// However, since we didn't provide any chaincodes to these peers - the server shouldn't
		// be able to construct the descriptor.
		// Just check that the appropriate error is returned, and nothing crashes.
		assert.Contains(t, err.Error(), "failed constructing descriptor for chaincode")
		assert.Nil(t, endorsers)
	})

	t.Run("Endorser query with chaincodes installed", func(t *testing.T) {
		// Next, we check the case when the peers publish chaincode for themselves.
		sup.On("PeersOfChannel").Return(channelPeersWithChaincodes).Times(2)
		req = NewRequest()
		req.OfChannel("mychannel").AddPeersQuery().AddEndorsersQuery(interest("mycc"))
		r, err = cl.Send(ctx, req, authInfo)
		assert.NoError(t, err)

		mychannel := r.ForChannel("mychannel")
		peers, err := mychannel.Peers()
		assert.NoError(t, err)
		assert.Len(t, peers, 8)

		// We should get a valid endorsement descriptor from the service
		endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
		assert.NoError(t, err)
		// The combinations of endorsers should be in the expected combinations
		assert.Contains(t, expectedOrgCombinations, getMSPs(endorsers))
	})

	t.Run("Endorser query with cc2cc and collections", func(t *testing.T) {
		sup.On("PeersOfChannel").Return(channelPeersWithChaincodes).Twice()
		req = NewRequest()
		myccOnly := ccCall("mycc")
		myccAndmycc2 := ccCall("mycc", "mycc2")
		myccAndmycc2[1].CollectionNames = append(myccAndmycc2[1].CollectionNames, "col")
		req.OfChannel("mychannel").AddEndorsersQuery(cc2ccInterests(myccAndmycc2, myccOnly)...)
		r, err = cl.Send(ctx, req, authInfo)
		assert.NoError(t, err)
		mychannel := r.ForChannel("mychannel")

		// Check the endorsers for the non cc2cc call
		endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
		assert.NoError(t, err)
		assert.Contains(t, expectedOrgCombinations, getMSPs(endorsers))
		// Check the endorsers for the cc2cc call with collections
		call := ccCall("mycc", "mycc2")
		call[1].CollectionNames = append(call[1].CollectionNames, "col")
		endorsers, err = mychannel.Endorsers(call, NoFilter)
		assert.NoError(t, err)
		assert.Contains(t, expectedOrgCombinations2, getMSPs(endorsers))
	})

	t.Run("Peer membership query with collections and chaincodes", func(t *testing.T) {
		sup.On("PeersOfChannel").Return(channelPeersWithChaincodes).Once()
		interest := ccCall("mycc2")
		interest[0].CollectionNames = append(interest[0].CollectionNames, "col")
		req = NewRequest().OfChannel("mychannel").AddPeersQuery(interest...)
		r, err = cl.Send(ctx, req, authInfo)
		assert.NoError(t, err)
		mychannel := r.ForChannel("mychannel")
		peers, err := mychannel.Peers(interest...)
		assert.NoError(t, err)
		// We should see all peers that aren't in ORG A since it's not part of the collection
		for _, p := range peers {
			assert.NotEqual(t, "A", p.MSPID)
		}
		assert.Len(t, peers, 6)
	})

	t.Run("Endorser query with PrioritiesByHeight selector", func(t *testing.T) {
		sup.On("PeersOfChannel").Return(channelPeersWithDifferentLedgerHeights).Twice()
		req = NewRequest()
		req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc3"))
		r, err = cl.Send(ctx, req, authInfo)
		assert.NoError(t, err)
		mychannel := r.ForChannel("mychannel")

		// acceptablePeers are the ones at the highest ledger height for each org
		acceptablePeers := []string{"p5", "p9", "p11", "p15"}
		used := make(map[string]struct{})
		endorsers, err := mychannel.Endorsers(ccCall("mycc3"), NewFilter(PrioritiesByHeight, NoExclusion))
		assert.NoError(t, err)
		names := getNames(endorsers)
		assert.Subset(t, acceptablePeers, names)
		for _, name := range names {
			used[name] = struct{}{}
		}
		assert.Equalf(t, len(acceptablePeers), len(used), "expecting each endorser to be returned at least once")
	})

	t.Run("Endorser query with custom filter", func(t *testing.T) {
		sup.On("PeersOfChannel").Return(channelPeersWithDifferentLedgerHeights).Twice()
		req = NewRequest()
		req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc3"))
		r, err = cl.Send(ctx, req, authInfo)
		assert.NoError(t, err)
		mychannel := r.ForChannel("mychannel")

		threshold := uint64(3) // Use peers within 3 of the max height of the org peers
		acceptablePeers := []string{"p1", "p9", "p3", "p5", "p6", "p7", "p10", "p11", "p12", "p14", "p15"}
		used := make(map[string]struct{})

		for i := 0; i < 90; i++ {
			endorsers, err := mychannel.Endorsers(ccCall("mycc3"), &ledgerHeightFilter{threshold: threshold})
			assert.NoError(t, err)
			names := getNames(endorsers)
			assert.Subset(t, acceptablePeers, names)
			for _, name := range names {
				used[name] = struct{}{}
			}
		}
		assert.Equalf(t, len(acceptablePeers), len(used), "expecting each endorser to be returned at least once")

		threshold = 0 // only use the peers at the highest ledger height (same as using the PrioritiesByHeight selector)
		acceptablePeers = []string{"p5", "p9", "p11", "p15"}
		used = make(map[string]struct{})
		endorsers, err := mychannel.Endorsers(ccCall("mycc3"), &ledgerHeightFilter{threshold: threshold})
		assert.NoError(t, err)
		names := getNames(endorsers)
		assert.Subset(t, acceptablePeers, names)
		for _, name := range names {
			used[name] = struct{}{}
		}
		t.Logf("Used peers: %#v\n", used)
		assert.Equalf(t, len(acceptablePeers), len(used), "expecting each endorser to be returned at least once")
	})
}

func TestUnableToSign(t *testing.T) {
	signer := func(msg []byte) ([]byte, error) {
		return nil, errors.New("not enough entropy")
	}
	failToConnect := func() (*grpc.ClientConn, error) {
		return nil, nil
	}
	authInfo := &discovery.AuthInfo{
		ClientIdentity: []byte{1, 2, 3},
	}
	cl := NewClient(failToConnect, signer, signerCacheSize)
	req := NewRequest()
	req = req.OfChannel("mychannel")
	resp, err := cl.Send(ctx, req, authInfo)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not enough entropy")
}

func TestUnableToConnect(t *testing.T) {
	signer := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	failToConnect := func() (*grpc.ClientConn, error) {
		return nil, errors.New("unable to connect")
	}
	auth := &discovery.AuthInfo{
		ClientIdentity: []byte{1, 2, 3},
	}
	cl := NewClient(failToConnect, signer, signerCacheSize)
	req := NewRequest()
	req = req.OfChannel("mychannel")
	resp, err := cl.Send(ctx, req, auth)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "unable to connect")
}

func TestBadResponses(t *testing.T) {
	signer := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	svc := newMockDiscoveryService()
	t.Logf("Started mock discovery service on port %d", svc.port)
	defer svc.shutdown()

	connect := func() (*grpc.ClientConn, error) {
		return grpc.Dial(fmt.Sprintf("localhost:%d", svc.port), grpc.WithInsecure())
	}

	auth := &discovery.AuthInfo{
		ClientIdentity: []byte{1, 2, 3},
	}
	cl := NewClient(connect, signer, signerCacheSize)

	// Scenario I: discovery service sends back an error
	svc.On("Discover").Return(nil, errors.New("foo")).Once()
	req := NewRequest()
	req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	r, err := cl.Send(ctx, req, auth)
	assert.Contains(t, err.Error(), "foo")
	assert.Nil(t, r)

	// Scenario II: discovery service sends back an empty response
	svc.On("Discover").Return(&discovery.Response{}, nil).Once()
	req = NewRequest()
	req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	r, err = cl.Send(ctx, req, auth)
	assert.Equal(t, "Sent 3 queries but received 0 responses back", err.Error())
	assert.Nil(t, r)

	// Scenario III: discovery service sends back a layout for the wrong chaincode
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: &discovery.QueryResult_CcQueryRes{
					CcQueryRes: &discovery.ChaincodeQueryResult{
						Content: []*discovery.EndorsementDescriptor{
							{
								Chaincode: "notmycc",
							},
						},
					},
				},
			},
		},
	}, nil).Once()
	req = NewRequest()
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	r, err = cl.Send(ctx, req, auth)
	assert.Nil(t, r)
	assert.Contains(t, err.Error(), "expected chaincode mycc but got endorsement descriptor for notmycc")

	// Scenario IV: discovery service sends back a layout that has empty envelopes
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: resultsWithoutEnvelopes,
			},
		},
	}, nil).Once()
	req = NewRequest()
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	r, err = cl.Send(ctx, req, auth)
	assert.Contains(t, err.Error(), "received empty envelope(s) for endorsers for chaincode mycc")
	assert.Nil(t, r)

	// Scenario V: discovery service sends back a layout that has a group that requires more
	// members than are present.
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: resultsWithEnvelopesButWithInsufficientPeers,
			},
		},
	}, nil).Once()
	req = NewRequest()
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	r, err = cl.Send(ctx, req, auth)
	assert.NoError(t, err)
	mychannel := r.ForChannel("mychannel")
	endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
	assert.Nil(t, endorsers)
	assert.Contains(t, err.Error(), "no endorsement combination can be satisfied")

	// Scenario VI: discovery service sends back a layout that has a group that doesn't have a matching peer set
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: resultsWithEnvelopesButWithMismatchedLayout,
			},
		},
	}, nil).Once()
	req = NewRequest()
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	r, err = cl.Send(ctx, req, auth)
	assert.Contains(t, err.Error(), "group B isn't mapped to endorsers, but exists in a layout")
	assert.Empty(t, r)
}

func TestAddEndorsersQueryInvalidInput(t *testing.T) {
	_, err := NewRequest().AddEndorsersQuery()
	assert.Contains(t, err.Error(), "no chaincode interests given")

	_, err = NewRequest().AddEndorsersQuery(nil)
	assert.Contains(t, err.Error(), "chaincode interest is nil")

	_, err = NewRequest().AddEndorsersQuery(&discovery.ChaincodeInterest{})
	assert.Contains(t, err.Error(), "invocation chain should not be empty")

	_, err = NewRequest().AddEndorsersQuery(&discovery.ChaincodeInterest{
		Chaincodes: []*discovery.ChaincodeCall{{}},
	})
	assert.Contains(t, err.Error(), "chaincode name should not be empty")
}

func TestValidateAliveMessage(t *testing.T) {
	am := aliveMessage(1)
	msg, _ := am.ToGossipMessage()

	// Scenario I: Valid alive message
	assert.NoError(t, validateAliveMessage(msg))

	// Scenario II: Nullify timestamp
	msg.GetAliveMsg().Timestamp = nil
	err := validateAliveMessage(msg)
	assert.Equal(t, "timestamp is nil", err.Error())

	// Scenario III: Nullify membership
	msg.GetAliveMsg().Membership = nil
	err = validateAliveMessage(msg)
	assert.Equal(t, "membership is empty", err.Error())

	// Scenario IV: Nullify the entire alive message part
	msg.Content = nil
	err = validateAliveMessage(msg)
	assert.Equal(t, "message isn't an alive message", err.Error())
}

func TestValidateStateInfoMessage(t *testing.T) {
	si := stateInfoWithHeight(100)

	// Scenario I: Valid state info message
	assert.NoError(t, validateStateInfoMessage(si))

	// Scenario II: Nullify properties
	si.GetStateInfo().Properties = nil
	err := validateStateInfoMessage(si)
	assert.Equal(t, "properties is nil", err.Error())

	// Scenario III: Nullify timestamp
	si.GetStateInfo().Timestamp = nil
	err = validateStateInfoMessage(si)
	assert.Equal(t, "timestamp is nil", err.Error())

	// Scenario IV: Nullify the state info message part
	si.Content = nil
	err = validateStateInfoMessage(si)
	assert.Equal(t, "message isn't a stateInfo message", err.Error())
}

func TestString(t *testing.T) {
	var ic InvocationChain
	ic = append(ic, &discovery.ChaincodeCall{
		Name:            "foo",
		CollectionNames: []string{"c1", "c2"},
	})
	ic = append(ic, &discovery.ChaincodeCall{
		Name:            "bar",
		CollectionNames: []string{"c3", "c4"},
	})
	expected := `[{"name":"foo","collection_names":["c1","c2"]},{"name":"bar","collection_names":["c3","c4"]}]`
	assert.Equal(t, expected, ic.String())
}

func getMSP(peer *Peer) string {
	endpoint := peer.AliveMessage.GetAliveMsg().Membership.Endpoint
	id, _ := strconv.ParseInt(endpoint[1:], 10, 64)
	switch id / 2 {
	case 0, 4:
		return "A"
	case 1, 5:
		return "B"
	case 2, 6:
		return "C"
	default:
		return "D"
	}
}

func getMSPs(endorsers []*Peer) map[string]struct{} {
	m := make(map[string]struct{})
	for _, endorser := range endorsers {
		m[getMSP(endorser)] = struct{}{}
	}
	return m
}

type ccMetadataFetcher struct {
	mock.Mock
}

func (mdf *ccMetadataFetcher) Metadata(channel string, cc string, _ bool) *chaincode.Metadata {
	return mdf.Called(cc).Get(0).(*chaincode.Metadata)
}

type principalEvaluator struct {
}

func (pe *principalEvaluator) SatisfiesPrincipal(channel string, identity []byte, principal *msp.MSPPrincipal) error {
	sID := &msp.SerializedIdentity{}
	proto.Unmarshal(identity, sID)
	p := &msp.MSPRole{}
	proto.Unmarshal(principal.Principal, p)
	if sID.Mspid == p.MspIdentifier {
		return nil
	}
	return errors.Errorf("peer %s has MSP %s but should have MSP %s", string(sID.IdBytes), sID.Mspid, p.MspIdentifier)
}

type policyFetcher struct {
	mock.Mock
}

func (pf *policyFetcher) PolicyByChaincode(channel string, cc string) policies.InquireablePolicy {
	return pf.Called(cc).Get(0).(policies.InquireablePolicy)
}

type endorsementAnalyzer interface {
	PeersForEndorsement(chainID gossipcommon.ChainID, interest *discovery.ChaincodeInterest) (*discovery.EndorsementDescriptor, error)

	PeersAuthorizedByCriteria(chainID gossipcommon.ChainID, interest *discovery.ChaincodeInterest) (gdisc.Members, error)
}

type inquireablePolicy struct {
	principals      []*msp.MSPPrincipal
	orgCombinations [][]string
}

func (ip *inquireablePolicy) appendPrincipal(orgName string) {
	ip.principals = append(ip.principals, &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: orgName})})
}

func (ip *inquireablePolicy) SatisfiedBy() []policies.PrincipalSet {
	var res []policies.PrincipalSet
	for _, orgs := range ip.orgCombinations {
		for _, org := range orgs {
			ip.appendPrincipal(org)
		}
		res = append(res, ip.principals)
		ip.principals = nil
	}
	return res
}

func peerIdentity(mspID string, i int) api.PeerIdentityInfo {
	p := []byte(fmt.Sprintf("p%d", i))
	sID := &msp.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: p,
	}
	b, _ := proto.Marshal(sID)
	return api.PeerIdentityInfo{
		Identity:     api.PeerIdentityType(b),
		PKIId:        gossipcommon.PKIidType(p),
		Organization: api.OrgIdentityType(mspID),
	}
}

type peerInfo struct {
	identity api.PeerIdentityType
	pkiID    gossipcommon.PKIidType
	gdisc.NetworkMember
}

func aliveMessage(id int) *gossip.Envelope {
	g := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{
				Timestamp: &gossip.PeerTime{
					SeqNum: uint64(id),
					IncNum: uint64(time.Now().UnixNano()),
				},
				Membership: &gossip.Member{
					Endpoint: fmt.Sprintf("p%d", id),
				},
			},
		},
	}
	sMsg, _ := g.NoopSign()
	return sMsg.Envelope
}

func stateInfoMessage(chaincodes ...*gossip.Chaincode) *gossip.Envelope {
	return stateInfoMessageWithHeight(0, chaincodes...)
}

func stateInfoMessageWithHeight(ledgerHeight uint64, chaincodes ...*gossip.Chaincode) *gossip.Envelope {
	g := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{
				Timestamp: &gossip.PeerTime{
					SeqNum: 5,
					IncNum: uint64(time.Now().UnixNano()),
				},
				Properties: &gossip.Properties{
					Chaincodes:   chaincodes,
					LedgerHeight: ledgerHeight,
				},
			},
		},
	}
	sMsg, _ := g.NoopSign()
	return sMsg.Envelope
}

func newPeer(i int, env *gossip.Envelope, properties *gossip.Properties) *peerInfo {
	p := fmt.Sprintf("p%d", i)
	return &peerInfo{
		pkiID:    gossipcommon.PKIidType(p),
		identity: api.PeerIdentityType(p),
		NetworkMember: gdisc.NetworkMember{
			PKIid:            gossipcommon.PKIidType(p),
			Endpoint:         p,
			InternalEndpoint: p,
			Envelope:         env,
			Properties:       properties,
		},
	}
}

type mockSupport struct {
	seq uint64
	mock.Mock
	endorsementAnalyzer
}

func (ms *mockSupport) ConfigSequence(channel string) uint64 {
	// Ensure cache is bypassed
	ms.seq++
	return ms.seq
}

func (ms *mockSupport) IdentityInfo() api.PeerIdentitySet {
	return ms.Called().Get(0).(api.PeerIdentitySet)
}

func (*mockSupport) ChannelExists(channel string) bool {
	return true
}

func (ms *mockSupport) PeersOfChannel(gossipcommon.ChainID) gdisc.Members {
	return ms.Called().Get(0).(gdisc.Members)
}

func (ms *mockSupport) Peers() gdisc.Members {
	return ms.Called().Get(0).(gdisc.Members)
}

func (ms *mockSupport) PeersForEndorsement(channel gossipcommon.ChainID, interest *discovery.ChaincodeInterest) (*discovery.EndorsementDescriptor, error) {
	return ms.endorsementAnalyzer.PeersForEndorsement(channel, interest)
}

func (ms *mockSupport) PeersAuthorizedByCriteria(channel gossipcommon.ChainID, interest *discovery.ChaincodeInterest) (gdisc.Members, error) {
	return ms.endorsementAnalyzer.PeersAuthorizedByCriteria(channel, interest)
}

func (*mockSupport) EligibleForService(channel string, data common.SignedData) error {
	return nil
}

func (ms *mockSupport) Config(channel string) (*discovery.ConfigResult, error) {
	return ms.Called(channel).Get(0).(*discovery.ConfigResult), nil
}

type mockDiscoveryServer struct {
	mock.Mock
	*grpc.Server
	port int64
}

func newMockDiscoveryService() *mockDiscoveryServer {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	s := grpc.NewServer()
	d := &mockDiscoveryServer{
		Server: s,
	}
	discovery.RegisterDiscoveryServer(s, d)
	go s.Serve(l)
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	d.port, _ = strconv.ParseInt(portStr, 10, 64)
	return d
}

func (ds *mockDiscoveryServer) shutdown() {
	ds.Server.Stop()
}

func (ds *mockDiscoveryServer) Discover(context.Context, *discovery.SignedRequest) (*discovery.Response, error) {
	args := ds.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*discovery.Response), nil
}

func ccCall(ccNames ...string) []*discovery.ChaincodeCall {
	var call []*discovery.ChaincodeCall
	for _, ccName := range ccNames {
		call = append(call, &discovery.ChaincodeCall{
			Name: ccName,
		})
	}
	return call
}

func cc2ccInterests(invocationsChains ...[]*discovery.ChaincodeCall) []*discovery.ChaincodeInterest {
	var interests []*discovery.ChaincodeInterest
	for _, invocationChain := range invocationsChains {
		interests = append(interests, &discovery.ChaincodeInterest{
			Chaincodes: invocationChain,
		})
	}
	return interests
}

func interest(ccNames ...string) *discovery.ChaincodeInterest {
	interest := &discovery.ChaincodeInterest{
		Chaincodes: []*discovery.ChaincodeCall{},
	}
	for _, cc := range ccNames {
		interest.Chaincodes = append(interest.Chaincodes, &discovery.ChaincodeCall{
			Name: cc,
		})
	}
	return interest
}

func buildCollectionConfig(col2principals map[string][]*msp.MSPPrincipal) []byte {
	collections := &common.CollectionConfigPackage{}
	for col, principals := range col2principals {
		collections.Config = append(collections.Config, &common.CollectionConfig{
			Payload: &common.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &common.StaticCollectionConfig{
					Name: col,
					MemberOrgsPolicy: &common.CollectionPolicyConfig{
						Payload: &common.CollectionPolicyConfig_SignaturePolicy{
							SignaturePolicy: &common.SignaturePolicyEnvelope{
								Identities: principals,
							},
						},
					},
				},
			},
		})
	}
	return utils.MarshalOrPanic(collections)
}

func memberPrincipal(mspID string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal: utils.MarshalOrPanic(&msp.MSPRole{
			MspIdentifier: mspID,
			Role:          msp.MSPRole_MEMBER,
		}),
	}
}

// ledgerHeightFilter is a filter that uses ledger height to prioritize endorsers, although it provides more
// even balancing than simply prioritizing by highest ledger height. Certain peers tend to always be at a slightly
// higher ledger height than others (such as leaders) but we shouldn't always be selecting leaders.
// This filter treats endorsers that are within a certain block height threshold equally and sorts them randomly.
type ledgerHeightFilter struct {
	threshold uint64
}

// Filter returns a random set of endorsers that are above the configured ledger height threshold.
func (f *ledgerHeightFilter) Filter(endorsers Endorsers) Endorsers {
	if len(endorsers) <= 1 {
		return endorsers
	}

	if f.threshold < 0 {
		return endorsers.Shuffle()
	}

	maxHeight := getMaxLedgerHeight(endorsers)

	if maxHeight <= f.threshold {
		return endorsers.Shuffle()
	}

	cutoffHeight := maxHeight - f.threshold

	var filteredEndorsers Endorsers
	for _, p := range endorsers {
		ledgerHeight := getLedgerHeight(p)
		if ledgerHeight >= cutoffHeight {
			filteredEndorsers = append(filteredEndorsers, p)
		}
	}
	return filteredEndorsers.Shuffle()
}

func getLedgerHeight(endorser *Peer) uint64 {
	return endorser.StateInfoMessage.GetStateInfo().GetProperties().LedgerHeight
}

func getMaxLedgerHeight(endorsers Endorsers) uint64 {
	var maxHeight uint64
	for _, peer := range endorsers {
		height := getLedgerHeight(peer)
		if height > maxHeight {
			maxHeight = height
		}
	}
	return maxHeight
}

func getNames(endorsers Endorsers) []string {
	var names []string
	for _, p := range endorsers {
		names = append(names, p.AliveMessage.GetAliveMsg().Membership.Endpoint)
	}
	return names
}
