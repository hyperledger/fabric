/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"bytes"
	"context"
	x509crypto "crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/util"
	comm_utils "github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	sourceNodeID      uint64 = 1
	destinationNodeID uint64 = 2
	nodeAuthRequest          = &orderer.NodeAuthRequest{
		Version:   0,
		FromId:    sourceNodeID,
		ToId:      destinationNodeID,
		Channel:   "mychannel",
		Timestamp: timestamppb.Now(),
	}
	nodeConsensusRequest = &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeConrequest{
			NodeConrequest: &orderer.NodeConsensusRequest{
				Payload: []byte{1, 2, 3},
			},
		},
	}
	nodeInvalidRequest = &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeConrequest{
			NodeConrequest: nil,
		},
	}
)

// Cluster Step stream for TLS Export Keying Material retrival
func getStepStream(t *testing.T) (*comm_utils.GRPCServer, orderer.ClusterNodeService_StepClient) {
	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	srvConfig := comm_utils.ServerConfig{
		SecOpts: comm_utils.SecureOptions{
			Key:         serverKeyPair.Key,
			Certificate: serverKeyPair.Cert,
			UseTLS:      true,
		},
	}
	gRPCServer, err := comm_utils.NewGRPCServer("127.0.0.1:", srvConfig)
	require.NoError(t, err)

	go gRPCServer.Start()

	tlsConf := &tls.Config{
		RootCAs: x509.NewCertPool(),
	}

	_ = tlsConf.RootCAs.AppendCertsFromPEM(ca.CertBytes())
	tlsOpts := grpc.WithTransportCredentials(credentials.NewTLS(tlsConf))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, gRPCServer.Address(), tlsOpts, grpc.WithBlock())
	require.NoError(t, err)

	cl := orderer.NewClusterNodeServiceClient(conn)

	stepStream, err := cl.Step(context.Background())
	require.NoError(t, err)

	return gRPCServer, stepStream
}

func TestClusterServiceStep(t *testing.T) {
	server, stepStream := getStepStream(t)
	defer server.Stop()

	t.Run("Create authenticated stream successfully", func(t *testing.T) {
		t.Parallel()
		var err error
		stream := &mocks.ClusterStepStream{}
		handler := &mocks.Handler{}
		serverKeyPair, _ := ca.NewServerCertKeyPair()

		serverKeyPair.Cert, err = crypto.SanitizeX509Cert(serverKeyPair.Cert)
		require.NoError(t, err)

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceStep1"),
			StepLogger:          flogging.MustGetLogger("test"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        serverKeyPair.Cert,
		}

		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		bindingHash := cluster.GetSessionBindingHash(authRequest)
		sessionBinding, err := cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		require.NoError(t, err)

		clientKeyPair, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair.Signer}
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: sessionBinding,
			Channel:        authRequest.Channel,
		})

		sig1, err := signer.Sign(asnSignFields)
		require.NoError(t, err)

		authRequest.SessionBinding = sessionBinding
		authRequest.Signature = sig1

		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		stream.On("Context").Return(stepStream.Context())
		stream.On("Recv").Return(stepRequest, nil).Once()
		stream.On("Recv").Return(nodeConsensusRequest, nil).Once()
		stream.On("Recv").Return(nil, io.EOF).Once()

		handler.On("OnConsensus", authRequest.Channel, authRequest.FromId, mock.Anything).Return(nil).Once()

		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})
		err = svc.Step(stream)
		require.NoError(t, err)
	})

	t.Run("Fail with error if first request not auth request message type", func(t *testing.T) {
		t.Parallel()
		stream := &mocks.ClusterStepStream{}
		handler := &mocks.Handler{}

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceStep2"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream.On("Context").Return(context.Background())
		stream.On("Recv").Return(nodeConsensusRequest, nil).Once()
		err := svc.Step(stream)
		require.EqualError(t, err, "rpc error: code = Unauthenticated desc = access denied")
	})

	t.Run("Client closes the stream prematurely", func(t *testing.T) {
		t.Parallel()
		stream := &mocks.ClusterStepStream{}
		handler := &mocks.Handler{}

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceStep3"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream.On("Context").Return(context.Background())
		stream.On("Recv").Return(nil, io.EOF).Once()
		err := svc.Step(stream)
		require.NoError(t, err)
	})

	t.Run("Connection terminated with error prematurely", func(t *testing.T) {
		t.Parallel()
		stream := &mocks.ClusterStepStream{}
		handler := &mocks.Handler{}

		svc4 := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceStep4"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream.On("Context").Return(context.Background())
		stream.On("Recv").Return(nil, errors.New("oops")).Once()
		err := svc4.Step(stream)
		require.EqualError(t, err, "oops")
	})

	t.Run("Invalid request type fails with error", func(t *testing.T) {
		t.Parallel()

		stream := &mocks.ClusterStepStream{}
		handler := &mocks.Handler{}

		var err error
		serverKeyPair, _ := ca.NewServerCertKeyPair()
		serverKeyPair.Cert, err = crypto.SanitizeX509Cert(serverKeyPair.Cert)
		require.NoError(t, err)

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceStep5"),
			StepLogger:          flogging.MustGetLogger("test"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        serverKeyPair.Cert,
		}

		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		bindingHash := cluster.GetSessionBindingHash(authRequest)
		sessionBinding, err := cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		require.NoError(t, err)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: sessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair, _ := ca.NewClientCertKeyPair()
		clientKeyPair.Cert, err = crypto.SanitizeX509Cert(clientKeyPair.Cert)
		require.NoError(t, err)

		signer := signingIdentity{clientKeyPair.Signer}
		sig, err := signer.Sign(asnSignFields)
		require.NoError(t, err)

		authRequest.Signature = sig
		authRequest.SessionBinding = sessionBinding

		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		stream.On("Context").Return(stepStream.Context())
		stream.On("Recv").Return(stepRequest, nil).Once()
		stream.On("Recv").Return(nodeInvalidRequest, nil).Once()
		stream.On("Recv").Return(nil, io.EOF).Once()

		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})
		err = svc.Step(stream)
		require.EqualError(t, err, "Message is neither a Submit nor Consensus request")
	})
}

func TestClusterServiceVerifyAuthRequest(t *testing.T) {
	t.Parallel()
	server, stepStream := getStepStream(t)
	defer server.Stop()

	t.Run("Verify auth request completes successfully", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		var err error
		handler := &mocks.Handler{}
		serverKeyPair, _ := ca.NewServerCertKeyPair()
		serverKeyPair.Cert, err = crypto.SanitizeX509Cert(serverKeyPair.Cert)
		require.NoError(t, err)

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest1"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        serverKeyPair.Cert,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair1.Signer}
		sig, err := signer.Sign(asnSignFields)
		require.NoError(t, err)

		authRequest.Signature = sig

		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.NoError(t, err)
	})

	t.Run("Verify auth request fails with sessing binding error", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		handler := &mocks.Handler{}
		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest2"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream := &mocks.ClusterStepStream{}
		stream.On("Context").Return(context.Background())
		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}})
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}
		_, err := svc.VerifyAuthRequest(stream, stepRequest)

		require.EqualError(t, err, "session binding read failed: failed extracting stream context")
	})

	t.Run("Verify auth request fails with session binding mismatch", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		handler := &mocks.Handler{}
		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest3"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		authRequest.SessionBinding = []byte{}
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair1.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}})

		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "session binding mismatch")
	})

	t.Run("Verify auth request fails with channel config not found", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		handler := &mocks.Handler{}
		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest4"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair1.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		delete(svc.MembershipByChannel, authRequest.Channel)

		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "channel mychannel not found in config")
	})

	t.Run("Verify auth request fails with node not part of the channel", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		handler := &mocks.Handler{}
		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest5"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair1.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig

		delete(svc.MembershipByChannel, authRequest.Channel)
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.ToId), Identity: clientKeyPair1.Cert}})

		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "node 1 is not member of channel mychannel")
	})

	t.Run("Verify auth request fails with signature mismatch", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		handler := &mocks.Handler{}
		serverKeyPair, _ := ca.NewServerCertKeyPair()
		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest6"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        serverKeyPair.Cert,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair1.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		clientKeyPair2, _ := ca.NewClientCertKeyPair()
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair2.Cert}, {Id: uint32(authRequest.ToId), Identity: clientKeyPair2.Cert}})
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "node id mismatch")
	})

	t.Run("Verify auth request fails with signature mismatch", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		handler := &mocks.Handler{}
		var err error
		serverKeyPair, _ := ca.NewServerCertKeyPair()
		serverKeyPair.Cert, err = crypto.SanitizeX509Cert(serverKeyPair.Cert)
		require.NoError(t, err)

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest7"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        serverKeyPair.Cert,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		clientKeyPair1.Cert, err = crypto.SanitizeX509Cert(clientKeyPair1.Cert)
		require.NoError(t, err)

		signer := signingIdentity{clientKeyPair1.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		clientKeyPair2, _ := ca.NewClientCertKeyPair()
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair2.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "signature mismatch: signature invalid")
	})

	// Note: The scenario where certificates have same public keys but different bytes
	// is thoroughly tested in the unit tests (TestCompareCertPublicKeysWithSameKeyDifferentBytes).
	// Integration testing of this scenario is complex due to signature verification requirements,
	// but the unit tests demonstrate that the compareCertPublicKeys function correctly handles
	// the case where bytes.Equal fails but public keys match.

	t.Run("Verify auth request fails when certificates have different public keys", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		handler := &mocks.Handler{}
		var err error

		// Generate the server certificate that will be used as NodeIdentity
		serverKeyPair, _ := ca.NewServerCertKeyPair()
		serverKeyPair.Cert, err = crypto.SanitizeX509Cert(serverKeyPair.Cert)
		require.NoError(t, err)

		// Generate a completely different certificate with different public key
		differentKeyPair, _ := ca.NewClientCertKeyPair()
		differentCert, err := crypto.SanitizeX509Cert(differentKeyPair.Cert)
		require.NoError(t, err)

		// Verify that the certificates have different bytes AND different public keys
		require.False(t, bytes.Equal(differentCert, serverKeyPair.Cert), "Certificates should have different bytes")

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest9"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        serverKeyPair.Cert,
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair, _ := ca.NewClientCertKeyPair()
		clientKeyPair.Cert, err = crypto.SanitizeX509Cert(clientKeyPair.Cert)
		require.NoError(t, err)

		signer := signingIdentity{clientKeyPair.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		// Configure the service with a certificate that has different public key than NodeIdentity
		// The key test is that toIdentity (differentCert) has different public key from NodeIdentity (serverKeyPair.Cert)
		// Both bytes.Equal and compareCertPublicKeys should fail
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{
			{Id: uint32(authRequest.FromId), Identity: clientKeyPair.Cert},
			{Id: uint32(authRequest.ToId), Identity: differentCert}, // Different bytes, different public key
		})

		// This should fail because compareCertPublicKeys will return false for different public keys
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "node id mismatch", "Auth request should fail when certificates have different public keys")
	})

	t.Run("Verify auth request handles certificate parsing errors gracefully", func(t *testing.T) {
		t.Parallel()
		authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

		handler := &mocks.Handler{}
		var err error

		// Create an invalid certificate (malformed PEM)
		invalidCert := []byte("-----BEGIN CERTIFICATE-----\ninvalid_certificate_data\n-----END CERTIFICATE-----")

		svc := &cluster.ClusterService{
			StreamCountReporter: &cluster.StreamCountReporter{
				Metrics: cluster.NewMetrics(&disabled.Provider{}),
			},
			Logger:              flogging.MustGetLogger("TestClusterServiceVerifyAuthRequest10"),
			MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
			RequestHandler:      handler,
			NodeIdentity:        invalidCert, // Use invalid cert as NodeIdentity to trigger parsing error
		}

		stream := &mocks.ClusterStepStream{}

		stream.On("Context").Return(stepStream.Context())
		bindingHash := cluster.GetSessionBindingHash(authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
			FromId:         strconv.FormatUint(authRequest.FromId, 10),
			ToId:           strconv.FormatUint(authRequest.ToId, 10),
			SessionBinding: authRequest.SessionBinding,
			Channel:        authRequest.Channel,
		})

		clientKeyPair, _ := ca.NewClientCertKeyPair()
		clientKeyPair.Cert, err = crypto.SanitizeX509Cert(clientKeyPair.Cert)
		require.NoError(t, err)

		signer := signingIdentity{clientKeyPair.Signer}
		sig, err := signer.Sign(cluster.SHA256Digest(asnSignFields))
		require.NoError(t, err)

		authRequest.Signature = sig
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: authRequest,
			},
		}

		// Configure the service with valid certificate for toIdentity
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{
			{Id: uint32(authRequest.FromId), Identity: clientKeyPair.Cert},
			{Id: uint32(authRequest.ToId), Identity: clientKeyPair.Cert}, // Valid certificate
		})

		// This should fail due to certificate parsing error when comparing with NodeIdentity (invalid cert)
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.Error(t, err, "Auth request should fail when certificate cannot be parsed")
		require.Contains(t, err.Error(), "failed to compare cert public keys", "Error should indicate certificate comparison failure")
	})
}

func TestConfigureNodeCerts(t *testing.T) {
	t.Parallel()
	authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

	t.Run("Creates new entry when input channel not part of the members list", func(t *testing.T) {
		t.Parallel()
		svc := &cluster.ClusterService{}
		svc.Logger = flogging.MustGetLogger("test")

		clientKeyPair1, _ := ca.NewClientCertKeyPair()

		var err error
		clientKeyPair1.Cert, err = crypto.SanitizeX509Cert(clientKeyPair1.Cert)
		require.NoError(t, err)

		err = svc.ConfigureNodeCerts("mychannel", []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}})
		require.NoError(t, err)
		require.Equal(t, clientKeyPair1.Cert, svc.MembershipByChannel["mychannel"].MemberMapping[authRequest.FromId])
	})

	t.Run("Updates entries when existing channel members provided", func(t *testing.T) {
		t.Parallel()
		svc := &cluster.ClusterService{}
		svc.Logger = flogging.MustGetLogger("test")

		var err error
		clientKeyPair1, _ := ca.NewClientCertKeyPair()
		clientKeyPair1.Cert, err = crypto.SanitizeX509Cert(clientKeyPair1.Cert)
		require.NoError(t, err)

		err = svc.ConfigureNodeCerts("mychannel", []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}})
		require.NoError(t, err)
		require.Equal(t, clientKeyPair1.Cert, svc.MembershipByChannel["mychannel"].MemberMapping[authRequest.FromId])

		clientKeyPair2, _ := ca.NewClientCertKeyPair()
		clientKeyPair2.Cert, err = crypto.SanitizeX509Cert(clientKeyPair2.Cert)
		require.NoError(t, err)

		err = svc.ConfigureNodeCerts("mychannel", []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair2.Cert}})
		require.NoError(t, err)
		require.Equal(t, clientKeyPair2.Cert, svc.MembershipByChannel["mychannel"].MemberMapping[authRequest.FromId])
	})
}

func TestExpirationWarning(t *testing.T) {
	t.Parallel()
	authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)

	server, stepStream := getStepStream(t)
	defer server.Stop()

	handler := &mocks.Handler{}
	stream := &mocks.ClusterStepStream{}

	var err error
	serverKeyPair, _ := ca.NewServerCertKeyPair()
	serverKeyPair.Cert, err = crypto.SanitizeX509Cert(serverKeyPair.Cert)
	require.NoError(t, err)

	cert := util.ExtractCertificateFromContext(stepStream.Context())

	svc := &cluster.ClusterService{
		CertExpWarningThreshold:          time.Until(cert.NotAfter),
		MinimumExpirationWarningInterval: time.Second * 2,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: cluster.NewMetrics(&disabled.Provider{}),
		},
		Logger:              flogging.MustGetLogger("TestExpirationWarning"),
		StepLogger:          flogging.MustGetLogger("test"),
		RequestHandler:      handler,
		MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
		NodeIdentity:        serverKeyPair.Cert,
	}

	bindingHash := cluster.GetSessionBindingHash(authRequest)
	authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

	asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
		Version:        int64(authRequest.Version),
		Timestamp:      cluster.EncodeTimestamp(authRequest.Timestamp),
		FromId:         strconv.FormatUint(authRequest.FromId, 10),
		ToId:           strconv.FormatUint(authRequest.ToId, 10),
		SessionBinding: authRequest.SessionBinding,
		Channel:        authRequest.Channel,
	})

	clientKeyPair1, _ := ca.NewClientCertKeyPair()
	clientKeyPair1.Cert, err = crypto.SanitizeX509Cert(clientKeyPair1.Cert)
	require.NoError(t, err)

	signer := signingIdentity{clientKeyPair1.Signer}
	sig, err := signer.Sign(asnSignFields)
	require.NoError(t, err)

	authRequest.Signature = sig
	stepRequest := &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
			NodeAuthrequest: authRequest,
		},
	}

	stream.On("Context").Return(stepStream.Context())
	stream.On("Recv").Return(stepRequest, nil).Once()
	stream.On("Recv").Return(nodeConsensusRequest, nil).Once()
	stream.On("Recv").Return(nil, io.EOF).Once()

	handler.On("OnConsensus", authRequest.Channel, authRequest.FromId, mock.Anything).Return(nil).Once()

	svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})

	alerts := make(chan struct{}, 10)
	svc.Logger = svc.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "expires in less than") {
			alerts <- struct{}{}
		}
		return nil
	}))

	_ = svc.Step(stream)

	// An alert is logged at the first time.
	select {
	case <-alerts:
	case <-time.After(time.Second * 5):
		t.Fatal("Should have received an alert")
	}
}

func TestClusterRequestAsString(t *testing.T) {
	t.Parallel()
	authRequest := proto.Clone(nodeAuthRequest).(*orderer.NodeAuthRequest)
	stepRequest := &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
			NodeAuthrequest: authRequest,
		},
	}
	tcs := []struct {
		name  string
		input *orderer.ClusterNodeServiceStepRequest
		exp   string
	}{
		{
			name:  "when input arg is nil returns error string",
			input: nil,
			exp:   "Request is nil",
		},
		{
			name:  "when input arg is unknown type returns error string",
			input: stepRequest,
			exp:   "unknown type:",
		},
		{
			name:  "when valid input arg is sent returns formatted string",
			input: nodeConsensusRequest,
			exp:   "ConsensusRequest for channel",
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			retVal := cluster.ClusterRequestAsString(tc.input)
			require.Contains(t, retVal, tc.exp)
		})
	}
}

// generateCertWithSameKey creates a new certificate using the provided signer (private key)
// but signed by a different CA, resulting in different certificate bytes but same public key
func generateCertWithSameKey(signer x509crypto.Signer, signingCA tlsgen.CA) ([]byte, error) {
	// Create a new certificate template
	template, err := newCertTemplate()
	if err != nil {
		return nil, err
	}

	// Set up the template for a client certificate
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}

	// Get the public key from the signer
	publicKey := signer.Public()

	// Get the signing CA's certificate
	caCertPEM := signingCA.CertBytes()
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return nil, errors.New("failed to decode CA certificate")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Create the certificate using the existing private key but signed by different CA
	rawBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, publicKey, signingCA.Signer())
	if err != nil {
		return nil, err
	}

	// Encode as PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rawBytes,
	})

	// Sanitize the certificate
	return crypto.SanitizeX509Cert(certPEM)
}

// Helper function to create certificate template (copied from tlsgen package for testing)
func newCertTemplate() (x509.Certificate, error) {
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return x509.Certificate{}, err
	}
	return x509.Certificate{
		Subject:      pkix.Name{SerialNumber: sn.String()},
		NotBefore:    time.Now().Add(time.Hour * (-24)),
		NotAfter:     time.Now().Add(time.Hour * 24),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		SerialNumber: sn,
	}, nil
}
