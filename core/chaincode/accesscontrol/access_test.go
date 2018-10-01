/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type ccSrv struct {
	l              net.Listener
	grpcSrv        *grpc.Server
	t              *testing.T
	cert           []byte
	expectedCCname string
}

func (cs *ccSrv) Register(stream pb.ChaincodeSupport_RegisterServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}

	// First message is a register message
	assert.Equal(cs.t, pb.ChaincodeMessage_REGISTER.String(), msg.Type.String())
	// And its chaincode name is the expected one
	chaincodeID := &pb.ChaincodeID{}
	err = proto.Unmarshal(msg.Payload, chaincodeID)
	if err != nil {
		return err
	}
	assert.Equal(cs.t, cs.expectedCCname, chaincodeID.Name)
	// Subsequent messages are just echoed back
	for {
		msg, _ = stream.Recv()
		if err != nil {
			return err
		}
		err = stream.Send(msg)
		if err != nil {
			return err
		}
	}
}

func (cs *ccSrv) stop() {
	cs.grpcSrv.Stop()
	cs.l.Close()
}

func createTLSService(t *testing.T, ca tlsgen.CA, host string) *grpc.Server {
	keyPair, err := ca.NewServerCertKeyPair(host)
	cert, err := tls.X509KeyPair(keyPair.Cert, keyPair.Key)
	assert.NoError(t, err)
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    x509.NewCertPool(),
	}
	tlsConf.ClientCAs.AppendCertsFromPEM(ca.CertBytes())
	return grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
}

func newCCServer(t *testing.T, port int, expectedCCname string, withTLS bool, ca tlsgen.CA) *ccSrv {
	var s *grpc.Server
	if withTLS {
		s = createTLSService(t, ca, "localhost")
	} else {
		s = grpc.NewServer()
	}

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", port))
	assert.NoError(t, err, "%v", err)
	return &ccSrv{
		t:              t,
		expectedCCname: expectedCCname,
		l:              l,
		grpcSrv:        s,
	}
}

type ccClient struct {
	conn   *grpc.ClientConn
	stream pb.ChaincodeSupport_RegisterClient
}

func newClient(t *testing.T, port int, cert *tls.Certificate, peerCACert []byte) (*ccClient, error) {
	tlsCfg := &tls.Config{
		RootCAs: x509.NewCertPool(),
	}

	tlsCfg.RootCAs.AppendCertsFromPEM(peerCACert)
	if cert != nil {
		tlsCfg.Certificates = []tls.Certificate{*cert}
	}
	tlsOpts := grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("localhost:%d", port), tlsOpts, grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	chaincodeSupportClient := pb.NewChaincodeSupportClient(conn)
	stream, err := chaincodeSupportClient.Register(context.Background())
	assert.NoError(t, err)
	return &ccClient{
		conn:   conn,
		stream: stream,
	}, nil
}

func (c *ccClient) close() {
	c.conn.Close()
}

func (c *ccClient) sendMsg(msg *pb.ChaincodeMessage) {
	c.stream.Send(msg)
}

func (c *ccClient) recv() *pb.ChaincodeMessage {
	msgs := make(chan *pb.ChaincodeMessage, 1)
	go func() {
		msg, _ := c.stream.Recv()
		if msg != nil {
			msgs <- msg
		}
	}()
	select {
	case <-time.After(time.Second):
		return nil
	case msg := <-msgs:
		return msg
	}
}

func TestAccessControl(t *testing.T) {
	backupTTL := ttl
	defer func() {
		ttl = backupTTL
	}()
	ttl = time.Second * 3

	oldLogger := logger
	l, recorder := floggingtest.NewTestLogger(t, floggingtest.AtLevel(zapcore.InfoLevel))
	logger = l
	defer func() { logger = oldLogger }()

	chaincodeID := &pb.ChaincodeID{Name: "example02"}
	payload, err := proto.Marshal(chaincodeID)
	registerMsg := &pb.ChaincodeMessage{
		Type:    pb.ChaincodeMessage_REGISTER,
		Payload: payload,
	}
	putStateMsg := &pb.ChaincodeMessage{
		Type: pb.ChaincodeMessage_PUT_STATE,
	}

	ca, _ := tlsgen.NewCA()
	srv := newCCServer(t, 7052, "example02", true, ca)
	auth := NewAuthenticator(ca)
	pb.RegisterChaincodeSupportServer(srv.grpcSrv, auth.Wrap(srv))
	go srv.grpcSrv.Serve(srv.l)
	defer srv.stop()

	// Create an attacker without a TLS certificate
	_, err = newClient(t, 7052, nil, ca.CertBytes())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")

	// Create an attacker with its own TLS certificate
	maliciousCA, _ := tlsgen.NewCA()
	keyPair, err := maliciousCA.NewClientCertKeyPair()
	cert, err := tls.X509KeyPair(keyPair.Cert, keyPair.Key)
	assert.NoError(t, err)
	_, err = newClient(t, 7052, &cert, ca.CertBytes())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")

	// Create a chaincode for example01 that tries to impersonate example02
	kp, err := auth.Generate("example01")
	assert.NoError(t, err)
	keyBytes, err := base64.StdEncoding.DecodeString(kp.Key)
	assert.NoError(t, err)
	certBytes, err := base64.StdEncoding.DecodeString(kp.Cert)
	assert.NoError(t, err)
	cert, err = tls.X509KeyPair(certBytes, keyBytes)
	assert.NoError(t, err)
	mismatchedShim, err := newClient(t, 7052, &cert, ca.CertBytes())
	assert.NoError(t, err)
	defer mismatchedShim.close()
	mismatchedShim.sendMsg(registerMsg)
	mismatchedShim.sendMsg(putStateMsg)
	// Mismatched chaincode didn't get back anything
	assert.Nil(t, mismatchedShim.recv())
	assertLogContains(t, recorder, "with given certificate hash", "belongs to a different chaincode")

	// Create the real chaincode that its cert is generated by us that should pass the security checks
	kp, err = auth.Generate("example02")
	assert.NoError(t, err)
	keyBytes, err = base64.StdEncoding.DecodeString(kp.Key)
	assert.NoError(t, err)
	certBytes, err = base64.StdEncoding.DecodeString(kp.Cert)
	assert.NoError(t, err)
	cert, err = tls.X509KeyPair(certBytes, keyBytes)
	assert.NoError(t, err)
	realCC, err := newClient(t, 7052, &cert, ca.CertBytes())
	assert.NoError(t, err)
	defer realCC.close()
	realCC.sendMsg(registerMsg)
	realCC.sendMsg(putStateMsg)
	echoMsg := realCC.recv()
	// The real chaincode should be echoed back its message
	assert.NotNil(t, echoMsg)
	assert.Equal(t, pb.ChaincodeMessage_PUT_STATE, echoMsg.Type)
	// Log should not complain about anything
	assert.Empty(t, recorder.Messages())

	// Create the real chaincode that its cert is generated by us
	// but one that the first message sent by it isn't a register message.
	// The second message that is sent is a register message but it's "too late"
	// and the stream is already denied.
	kp, err = auth.Generate("example02")
	assert.NoError(t, err)
	keyBytes, err = base64.StdEncoding.DecodeString(kp.Key)
	assert.NoError(t, err)
	certBytes, err = base64.StdEncoding.DecodeString(kp.Cert)
	assert.NoError(t, err)
	cert, err = tls.X509KeyPair(certBytes, keyBytes)
	assert.NoError(t, err)
	confusedCC, err := newClient(t, 7052, &cert, ca.CertBytes())
	assert.NoError(t, err)
	defer confusedCC.close()
	confusedCC.sendMsg(putStateMsg)
	confusedCC.sendMsg(registerMsg)
	confusedCC.sendMsg(putStateMsg)
	assert.Nil(t, confusedCC.recv())
	assertLogContains(t, recorder, "expected a ChaincodeMessage_REGISTER message")

	// Create a real chaincode, that its cert was generated by us
	// but it sends a malformed first message
	kp, err = auth.Generate("example02")
	assert.NoError(t, err)
	keyBytes, err = base64.StdEncoding.DecodeString(kp.Key)
	assert.NoError(t, err)
	certBytes, err = base64.StdEncoding.DecodeString(kp.Cert)
	assert.NoError(t, err)
	cert, err = tls.X509KeyPair(certBytes, keyBytes)
	assert.NoError(t, err)
	malformedMessageCC, err := newClient(t, 7052, &cert, ca.CertBytes())
	assert.NoError(t, err)
	defer malformedMessageCC.close()
	// Save old payload
	originalPayload := registerMsg.Payload
	registerMsg.Payload = append(registerMsg.Payload, 0)
	malformedMessageCC.sendMsg(registerMsg)
	malformedMessageCC.sendMsg(putStateMsg)
	assert.Nil(t, malformedMessageCC.recv())
	assertLogContains(t, recorder, "Failed unmarshaling message")
	// Recover old payload
	registerMsg.Payload = originalPayload

	// Create a real chaincode, that its cert was generated by us
	// but have it reconnect only after too much time.
	// This tests a use case where the CC's cert has been expired
	// and the CC has been compromised. We don't want it to be able
	// to reconnect to us.
	kp, err = auth.Generate("example02")
	assert.NoError(t, err)
	keyBytes, err = base64.StdEncoding.DecodeString(kp.Key)
	assert.NoError(t, err)
	certBytes, err = base64.StdEncoding.DecodeString(kp.Cert)
	assert.NoError(t, err)
	cert, err = tls.X509KeyPair(certBytes, keyBytes)
	assert.NoError(t, err)
	lateCC, err := newClient(t, 7052, &cert, ca.CertBytes())
	assert.NoError(t, err)
	defer lateCC.close()
	time.Sleep(ttl + time.Second*2)
	lateCC.sendMsg(registerMsg)
	lateCC.sendMsg(putStateMsg)
	echoMsg = lateCC.recv()
	assert.Nil(t, echoMsg)
	assertLogContains(t, recorder, "with given certificate hash", "not found in registry")
}

func assertLogContains(t *testing.T, r *floggingtest.Recorder, ss ...string) {
	defer r.Reset()
	for _, s := range ss {
		assert.NotEmpty(t, r.MessagesContaining(s))
	}
}
