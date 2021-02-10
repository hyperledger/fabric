/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/comm/testpb"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// test server to be registered with the GRPCServer
type testServiceServer struct{}

func (tss *testServiceServer) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	return new(testpb.Empty), nil
}

// createCertPool creates an x509.CertPool from an array of PEM-encoded certificates
func createCertPool(rootCAs [][]byte) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	for _, rootCA := range rootCAs {
		if !certPool.AppendCertsFromPEM(rootCA) {
			return nil, errors.New("Failed to load root certificates")
		}
	}
	return certPool, nil
}

// helper function to invoke the EmptyCall againt the test service
func invokeEmptyCall(address string, dialOptions []grpc.DialOption) (*testpb.Empty, error) {
	// add DialOptions
	dialOptions = append(dialOptions, grpc.WithBlock())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create GRPC client conn
	clientConn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return nil, err
	}
	defer clientConn.Close()

	// create GRPC client
	client := testpb.NewTestServiceClient(clientConn)

	// invoke service
	empty, err := client.EmptyCall(context.Background(), new(testpb.Empty))
	if err != nil {
		return nil, err
	}

	return empty, nil
}

// helper function to build an MSPConfig given root certs
func createMSPConfig(mspID string, rootCerts, tlsRootCerts, tlsIntermediateCerts [][]byte) (*mspproto.MSPConfig, error) {
	fmspconf := &mspproto.FabricMSPConfig{
		RootCerts:            rootCerts,
		TlsRootCerts:         tlsRootCerts,
		TlsIntermediateCerts: tlsIntermediateCerts,
		Name:                 mspID,
		FabricNodeOus: &mspproto.FabricNodeOUs{
			Enable: true,
			ClientOuIdentifier: &mspproto.FabricOUIdentifier{
				OrganizationalUnitIdentifier: "client",
			},
			PeerOuIdentifier: &mspproto.FabricOUIdentifier{
				OrganizationalUnitIdentifier: "peer",
			},
			AdminOuIdentifier: &mspproto.FabricOUIdentifier{
				OrganizationalUnitIdentifier: "admin",
			},
		},
	}

	return &mspproto.MSPConfig{
		Config: protoutil.MarshalOrPanic(fmspconf),
		Type:   int32(msp.FABRIC),
	}, nil
}

func createConfigBlock(channelID string, appMSPConf, ordererMSPConf *mspproto.MSPConfig,
	appOrgID, ordererOrgID string) (*cb.Block, error) {
	block, err := configtxtest.MakeGenesisBlockFromMSPs(channelID, appMSPConf, ordererMSPConf, appOrgID, ordererOrgID)
	if block == nil || err != nil {
		return block, err
	}

	txsFilter := txflags.NewWithValues(len(block.Data.Data), pb.TxValidationCode_VALID)
	block.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	return block, nil
}

func TestUpdateRootsFromConfigBlock(t *testing.T) {
	org1CA, err := tlsgen.NewCA()
	require.NoError(t, err)
	org1Server1KeyPair, err := org1CA.NewServerCertKeyPair("localhost", "127.0.0.1", "::1")
	require.NoError(t, err)

	org2CA, err := tlsgen.NewCA()
	require.NoError(t, err)
	org2Server1KeyPair, err := org2CA.NewServerCertKeyPair("localhost", "127.0.0.1", "::1")
	require.NoError(t, err)

	org2IntermediateCA, err := org2CA.NewIntermediateCA()
	require.NoError(t, err)
	org2IntermediateServer1KeyPair, err := org2IntermediateCA.NewServerCertKeyPair("localhost", "127.0.0.1", "::1")
	require.NoError(t, err)

	ordererOrgCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	ordererOrgServer1KeyPair, err := ordererOrgCA.NewServerCertKeyPair("localhost", "127.0.0.1", "::1")
	require.NoError(t, err)

	// create test MSPConfigs
	org1MSPConf, err := createMSPConfig("Org1MSP", [][]byte{org2CA.CertBytes()}, [][]byte{org1CA.CertBytes()}, [][]byte{})
	require.NoError(t, err)
	org2MSPConf, err := createMSPConfig("Org2MSP", [][]byte{org1CA.CertBytes()}, [][]byte{org2CA.CertBytes()}, [][]byte{})
	require.NoError(t, err)
	org2IntermediateMSPConf, err := createMSPConfig("Org2IntermediateMSP", [][]byte{org1CA.CertBytes()}, [][]byte{org2CA.CertBytes()}, [][]byte{org2IntermediateCA.CertBytes()})
	require.NoError(t, err)
	ordererOrgMSPConf, err := createMSPConfig("OrdererOrgMSP", [][]byte{org1CA.CertBytes()}, [][]byte{ordererOrgCA.CertBytes()}, [][]byte{})
	require.NoError(t, err)

	// create test channel create blocks
	channel1Block, err := createConfigBlock("channel1", org1MSPConf, ordererOrgMSPConf, "Org1MSP", "OrdererOrgMSP")
	require.NoError(t, err)
	channel2Block, err := createConfigBlock("channel2", org2MSPConf, ordererOrgMSPConf, "Org2MSP", "OrdererOrgMSP")
	require.NoError(t, err)
	channel3Block, err := createConfigBlock("channel3", org2IntermediateMSPConf, ordererOrgMSPConf, "Org2IntermediateMSP", "OrdererOrgMSP")
	require.NoError(t, err)

	serverConfig := comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:            true,
			Certificate:       org1Server1KeyPair.Cert,
			Key:               org1Server1KeyPair.Key,
			ServerRootCAs:     [][]byte{org1CA.CertBytes()},
			RequireClientCert: true,
		},
	}

	peerInstance, cleanup := peer.NewTestPeer(t)
	defer cleanup()
	peerInstance.CredentialSupport = comm.NewCredentialSupport()

	createChannel := func(t *testing.T, cid string, block *cb.Block) {
		err = peerInstance.CreateChannel(cid, block, &mock.DeployedChaincodeInfoProvider{}, nil, nil)
		require.NoError(t, err, "failed to create channel from block")
		t.Logf("Channel %s MSPIDs: (%s)", cid, peerInstance.GetMSPIDs(cid))
	}

	org1CertPool, err := createCertPool([][]byte{org1CA.CertBytes()})
	require.NoError(t, err)

	// use server cert as client cert
	org1ClientCert, err := tls.X509KeyPair(org1Server1KeyPair.Cert, org1Server1KeyPair.Key)
	require.NoError(t, err)

	org1Creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{org1ClientCert},
		RootCAs:      org1CertPool,
	})

	org2ClientCert, err := tls.X509KeyPair(org2Server1KeyPair.Cert, org2Server1KeyPair.Key)
	require.NoError(t, err)
	org2Creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{org2ClientCert},
		RootCAs:      org1CertPool,
	})

	org2IntermediateClientCert, err := tls.X509KeyPair(org2IntermediateServer1KeyPair.Cert, org2IntermediateServer1KeyPair.Key)
	require.NoError(t, err)
	org2IntermediateCreds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{org2IntermediateClientCert},
		RootCAs:      org1CertPool,
	})

	ordererOrgClientCert, err := tls.X509KeyPair(ordererOrgServer1KeyPair.Cert, ordererOrgServer1KeyPair.Key)
	require.NoError(t, err)

	ordererOrgCreds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{ordererOrgClientCert},
		RootCAs:      org1CertPool,
	})

	// basic function tests
	tests := []struct {
		name          string
		serverConfig  comm.ServerConfig
		createChannel func(*testing.T)
		goodOptions   []grpc.DialOption
		badOptions    []grpc.DialOption
		numAppCAs     int
		numOrdererCAs int
	}{
		{
			name:          "MutualTLSOrg1Org1",
			serverConfig:  serverConfig,
			createChannel: func(t *testing.T) { createChannel(t, "channel1", channel1Block) },
			goodOptions:   []grpc.DialOption{grpc.WithTransportCredentials(org1Creds)},
			badOptions:    []grpc.DialOption{grpc.WithTransportCredentials(ordererOrgCreds)},
			numAppCAs:     3, // each channel also has a DEFAULT MSP
			numOrdererCAs: 1,
		},
		{
			name:          "MutualTLSOrg1Org2",
			serverConfig:  serverConfig,
			createChannel: func(t *testing.T) { createChannel(t, "channel2", channel2Block) },
			goodOptions: []grpc.DialOption{
				grpc.WithTransportCredentials(org2Creds),
			},
			badOptions: []grpc.DialOption{
				grpc.WithTransportCredentials(ordererOrgCreds),
			},
			numAppCAs:     6,
			numOrdererCAs: 2,
		},
		{
			name:          "MutualTLSOrg1Org2Intermediate",
			serverConfig:  serverConfig,
			createChannel: func(t *testing.T) { createChannel(t, "channel3", channel3Block) },
			goodOptions: []grpc.DialOption{
				grpc.WithTransportCredentials(org2IntermediateCreds),
			},
			badOptions: []grpc.DialOption{
				grpc.WithTransportCredentials(ordererOrgCreds),
			},
			numAppCAs:     10,
			numOrdererCAs: 3,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			server, err := comm.NewGRPCServer("localhost:0", test.serverConfig)
			require.NoError(t, err, "failed to create gRPC server")
			require.NotNil(t, server)

			peerInstance.SetServer(server)
			peerInstance.ServerConfig = test.serverConfig

			// register a GRPC test service
			testpb.RegisterTestServiceServer(server.Server(), &testServiceServer{})
			go server.Start()
			defer server.Stop()

			// extract dynamic listen port
			_, port, err := net.SplitHostPort(server.Listener().Addr().String())
			require.NoError(t, err, "unable to extract listener port")
			testAddress := "localhost:" + port

			// invoke the EmptyCall service with good options but should fail
			// until channel is created and root CAs are updated
			_, err = invokeEmptyCall(testAddress, test.goodOptions)
			require.Error(t, err, "Expected error invoking the EmptyCall service ")

			// creating channel should update the trusted client roots
			test.createChannel(t)

			// invoke the EmptyCall service with good options
			_, err = invokeEmptyCall(testAddress, test.goodOptions)
			require.NoError(t, err, "Failed to invoke the EmptyCall service")

			// invoke the EmptyCall service with bad options
			_, err = invokeEmptyCall(testAddress, test.badOptions)
			require.Error(t, err, "Expected error using bad dial options")
		})
	}
}
