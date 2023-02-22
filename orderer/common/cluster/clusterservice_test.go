/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto"

	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
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
)

var (
	sourceNodeID      uint64 = 1
	destinationNodeID uint64 = 2
	streamID          uint64 = 111
	tstamp, _                = ptypes.TimestampProto(time.Now().UTC())
	nodeAuthRequest          = orderer.NodeAuthRequest{
		Version:   0,
		FromId:    sourceNodeID,
		ToId:      destinationNodeID,
		Channel:   "mychannel",
		Timestamp: tstamp,
	}
	nodeConsensusRequest = &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeConrequest{
			NodeConrequest: &orderer.NodeConsensusRequest{
				Payload: []byte{1, 2, 3},
			},
		},
	}
	nodeTranRequest = &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeTranrequest{
			NodeTranrequest: &orderer.NodeTransactionOrderRequest{
				LastValidationSeq: 0,
				Payload:           &common.Envelope{},
			},
		},
	}
	nodeInvalidRequest = &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeConrequest{
			NodeConrequest: nil,
		},
	}
	submitRequest = &orderer.StepRequest{
		Payload: &orderer.StepRequest_SubmitRequest{
			SubmitRequest: &orderer.SubmitRequest{
				LastValidationSeq: 0,
				Payload:           &common.Envelope{},
				Channel:           "mychannel",
			},
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

		authRequest := nodeAuthRequest

		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		sessionBinding, err := cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		require.NoError(t, err)

		clientKeyPair, _ := ca.NewClientCertKeyPair()
		signer := signingIdentity{clientKeyPair.Signer}
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
				NodeAuthrequest: &authRequest,
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

		authRequest := nodeAuthRequest

		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		sessionBinding, err := cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		require.NoError(t, err)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
				NodeAuthrequest: &authRequest,
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
		authRequest := nodeAuthRequest

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
		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
				NodeAuthrequest: &authRequest,
			},
		}
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair1.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.NoError(t, err)
	})

	t.Run("Verify auth request fails with sessing binding error", func(t *testing.T) {
		t.Parallel()
		authRequest := nodeAuthRequest

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
				NodeAuthrequest: &authRequest,
			},
		}
		_, err := svc.VerifyAuthRequest(stream, stepRequest)

		require.EqualError(t, err, "session binding read failed: failed extracting stream context")
	})

	t.Run("Verify auth request fails with session binding mismatch", func(t *testing.T) {
		t.Parallel()
		authRequest := nodeAuthRequest
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: &authRequest,
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
			Timestamp:      authRequest.Timestamp.String(),
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
		authRequest := nodeAuthRequest

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
		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
				NodeAuthrequest: &authRequest,
			},
		}

		delete(svc.MembershipByChannel, authRequest.Channel)

		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "channel mychannel not found in config")
	})

	t.Run("Verify auth request fails with node not part of the channel", func(t *testing.T) {
		t.Parallel()
		authRequest := nodeAuthRequest
		stepRequest := &orderer.ClusterNodeServiceStepRequest{
			Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
				NodeAuthrequest: &authRequest,
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
		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)
		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
		authRequest := nodeAuthRequest

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
		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
				NodeAuthrequest: &authRequest,
			},
		}

		clientKeyPair2, _ := ca.NewClientCertKeyPair()
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair2.Cert}, {Id: uint32(authRequest.ToId), Identity: clientKeyPair2.Cert}})
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "node id mismatch")
	})

	t.Run("Verify auth request fails with signature mismatch", func(t *testing.T) {
		t.Parallel()
		authRequest := nodeAuthRequest

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
		bindingHash := cluster.GetSessionBindingHash(&authRequest)
		authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

		asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
			Version:        int64(authRequest.Version),
			Timestamp:      authRequest.Timestamp.String(),
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
				NodeAuthrequest: &authRequest,
			},
		}

		clientKeyPair2, _ := ca.NewClientCertKeyPair()
		svc.ConfigureNodeCerts(authRequest.Channel, []*common.Consenter{{Id: uint32(authRequest.FromId), Identity: clientKeyPair2.Cert}, {Id: uint32(authRequest.ToId), Identity: svc.NodeIdentity}})
		_, err = svc.VerifyAuthRequest(stream, stepRequest)
		require.EqualError(t, err, "signature mismatch: signature invalid")
	})
}

func TestConfigureNodeCerts(t *testing.T) {
	t.Parallel()
	authRequest := nodeAuthRequest

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
	authRequest := nodeAuthRequest

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

	bindingHash := cluster.GetSessionBindingHash(&authRequest)
	authRequest.SessionBinding, _ = cluster.GetTLSSessionBinding(stepStream.Context(), bindingHash)

	asnSignFields, _ := asn1.Marshal(cluster.AuthRequestSignature{
		Version:        int64(authRequest.Version),
		Timestamp:      authRequest.Timestamp.String(),
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
			NodeAuthrequest: &authRequest,
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
	authRequest := nodeAuthRequest
	stepRequest := &orderer.ClusterNodeServiceStepRequest{
		Payload: &orderer.ClusterNodeServiceStepRequest_NodeAuthrequest{
			NodeAuthrequest: &authRequest,
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
