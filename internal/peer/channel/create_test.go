/*
Copyright Digital Asset Holdings, LLC All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/common/mock"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

//go:generate counterfeiter -o mock/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

type timeoutOrderer struct {
	counter int
	net.Listener
	*grpc.Server
	nextExpectedSeek uint64
	t                *testing.T
	blockChannel     chan uint64
}

func newOrderer(port int, t *testing.T) *timeoutOrderer {
	srv := grpc.NewServer()
	lsnr, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(err)
	}
	o := &timeoutOrderer{
		Server:           srv,
		Listener:         lsnr,
		t:                t,
		nextExpectedSeek: uint64(1),
		blockChannel:     make(chan uint64, 1),
		counter:          int(1),
	}
	orderer.RegisterAtomicBroadcastServer(srv, o)
	go srv.Serve(lsnr)
	return o
}

func (o *timeoutOrderer) Shutdown() {
	o.Server.Stop()
	o.Listener.Close()
}

func (*timeoutOrderer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("Should not have been called")
}

func (o *timeoutOrderer) SendBlock(seq uint64) {
	o.blockChannel <- seq
}

func (o *timeoutOrderer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	o.timeoutIncrement()
	if o.counter > 5 {
		o.sendBlock(stream, 0)
	}
	return nil
}

func (o *timeoutOrderer) sendBlock(stream orderer.AtomicBroadcast_DeliverServer, seq uint64) {
	block := &cb.Block{
		Header: &cb.BlockHeader{
			Number: seq,
		},
	}
	stream.Send(&orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: block},
	})
}

func (o *timeoutOrderer) timeoutIncrement() {
	o.counter++
}

var once sync.Once

// / mock deliver client for UT
type mockDeliverClient struct {
	err error
}

func (m *mockDeliverClient) readBlock() (*cb.Block, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &cb.Block{}, nil
}

func (m *mockDeliverClient) GetSpecifiedBlock(num uint64) (*cb.Block, error) {
	return m.readBlock()
}

func (m *mockDeliverClient) GetOldestBlock() (*cb.Block, error) {
	return m.readBlock()
}

func (m *mockDeliverClient) GetNewestBlock() (*cb.Block, error) {
	return m.readBlock()
}

func (m *mockDeliverClient) Close() error {
	return nil
}

// InitMSP init MSP
func InitMSP() {
	once.Do(initMSP)
}

func initMSP() {
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		panic(fmt.Errorf("Fatal error when reading MSP config: err %s", err))
	}
}

func mockBroadcastClientFactory() (common.BroadcastClient, error) {
	return common.GetMockBroadcastClient(nil), nil
}

func TestCreateChain(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected create command to succeed")
	}

	filename := mockchain + ".block"
	if _, err := os.Stat(filename); err != nil {
		t.Fail()
		t.Errorf("expected %s to exist", filename)
	}
}

func TestCreateChainWithOutputBlock(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)
	AddFlags(cmd)

	tempDir := t.TempDir()

	outputBlockPath := filepath.Join(tempDir, "output.block")
	args := []string{"-c", mockchain, "-o", "localhost:7050", "--outputBlock", outputBlockPath}
	cmd.SetArgs(args)
	defer func() { outputBlock = "" }()

	err = cmd.Execute()
	require.NoError(t, err, "execute should succeed")

	_, err = os.Stat(outputBlockPath)
	require.NoErrorf(t, err, "expected %s to exist", outputBlockPath)
}

func TestCreateChainWithDefaultAnchorPeers(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)
	AddFlags(cmd)
	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected create command to succeed")
	}
}

func TestCreateChainWithWaitSuccess(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{err: nil},
	}
	fakeOrderer := newOrderer(8101, t)
	defer fakeOrderer.Shutdown()

	cmd := createCmd(mockCF)
	AddFlags(cmd)
	args := []string{"-c", mockchain, "-o", "localhost:8101", "-t", "10s"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected create command to succeed")
	}
}

func TestCreateChainWithTimeoutErr(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{err: errors.New("bobsled")},
	}
	fakeOrderer := newOrderer(8102, t)
	defer fakeOrderer.Shutdown()

	// failure - connects to orderer but times out waiting for channel to
	// be created
	cmd := createCmd(mockCF)
	AddFlags(cmd)
	channelCmd.AddCommand(cmd)
	args := []string{"create", "-c", mockchain, "-o", "localhost:8102", "-t", "10ms", "--connTimeout", "1s"}
	channelCmd.SetArgs(args)

	if err := channelCmd.Execute(); err == nil {
		t.Error("expected create chain to fail with deliver error")
	} else {
		require.Contains(t, err.Error(), "timeout waiting for channel creation")
	}

	// failure - point to bad port and time out connecting to orderer
	args = []string{"create", "-c", mockchain, "-o", "localhost:0", "-t", "1s", "--connTimeout", "10ms"}
	channelCmd.SetArgs(args)

	if err := channelCmd.Execute(); err == nil {
		t.Error("expected create chain to fail with deliver error")
	} else {
		require.Contains(t, err.Error(), "failed connecting")
	}
}

func TestCreateChainBCFail(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := errors.New("luge")
	mockCF := &ChannelCmdFactory{
		BroadcastFactory: func() (common.BroadcastClient, error) {
			return common.GetMockBroadcastClient(sendErr), nil
		},
		Signer:        signer,
		DeliverClient: &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)
	AddFlags(cmd)
	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	expectedErrMsg := sendErr.Error()
	if err := cmd.Execute(); err == nil {
		t.Error("expected create chain to fail with broadcast error")
	} else {
		if err.Error() != expectedErrMsg {
			t.Errorf("Run create chain get unexpected error: %s(expected %s)", err.Error(), expectedErrMsg)
		}
	}
}

func TestCreateChainDeliverFail(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := fmt.Errorf("skeleton")
	mockCF := &ChannelCmdFactory{
		BroadcastFactory: func() (common.BroadcastClient, error) {
			return common.GetMockBroadcastClient(sendErr), nil
		},
		Signer:        signer,
		DeliverClient: &mockDeliverClient{err: sendErr},
	}
	cmd := createCmd(mockCF)
	AddFlags(cmd)
	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	expectedErrMsg := sendErr.Error()
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected create chain to fail with deliver error")
	} else {
		if err.Error() != expectedErrMsg {
			t.Errorf("Run create chain get unexpected error: %s(expected %s)", err.Error(), expectedErrMsg)
		}
	}
}

func createTxFile(filename string, typ cb.HeaderType, channelID string) (*cb.Envelope, error) {
	ch := &cb.ChannelHeader{Type: int32(typ), ChannelId: channelID}
	data, err := proto.Marshal(ch)
	if err != nil {
		return nil, err
	}

	p := &cb.Payload{Header: &cb.Header{ChannelHeader: data}}
	data, err = proto.Marshal(p)
	if err != nil {
		return nil, err
	}

	env := &cb.Envelope{Payload: data}
	data, err = proto.Marshal(env)
	if err != nil {
		return nil, err
	}

	if err = ioutil.WriteFile(filename, data, 0o644); err != nil {
		return nil, err
	}

	return env, nil
}

func TestCreateChainFromTx(t *testing.T) {
	defer resetFlags()
	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchannel := "mockchannel"
	dir := t.TempDir()

	// this could be created by the create command
	defer os.Remove(mockchannel + ".block")

	file := filepath.Join(dir, mockchannel)

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}
	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)
	AddFlags(cmd)

	// Error case 0
	args := []string{"-c", "", "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "Create command should have failed because channel ID is not specified")
	require.Contains(t, err.Error(), "must supply channel ID")

	// Error case 1
	args = []string{"-c", mockchannel, "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "Create command should have failed because tx file does not exist")
	msgExpr := regexp.MustCompile(`channel create configuration tx file not found.*no such file or directory`)
	require.True(t, msgExpr.MatchString(err.Error()))

	// Success case: -f option is empty
	args = []string{"-c", mockchannel, "-f", "", "-o", "localhost:7050"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.NoError(t, err)

	// Success case
	args = []string{"-c", mockchannel, "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)
	_, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, mockchannel)
	require.NoError(t, err, "Couldn't create tx file")
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestCreateChainInvalidTx(t *testing.T) {
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchannel := "mockchannel"

	dir := t.TempDir()

	// this is created by create command
	defer os.Remove(mockchannel + ".block")

	file := filepath.Join(dir, mockchannel)

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchannel, "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	// bad type CONFIG
	if _, err = createTxFile(file, cb.HeaderType_CONFIG, mockchannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	defer os.Remove(file)

	if err = cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}

	// bad channel name - does not match one specified in command
	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, "different_channel"); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err = cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}

	// empty channel
	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, ""); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}
}

func TestCreateChainNilCF(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	InitMSP()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	mockchannel := "mockchannel"
	dir := t.TempDir()

	// this is created by create command
	defer os.Remove(mockchannel + ".block")
	file := filepath.Join(dir, mockchannel)

	// Error case: grpc error
	viper.Set("orderer.client.connTimeout", 10*time.Millisecond)
	cmd := createCmd(nil)
	AddFlags(cmd)
	args := []string{"-c", mockchannel, "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create deliver client")

	// Error case: invalid ordering service endpoint
	args = []string{"-c", mockchannel, "-f", file, "-o", "localhost"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ordering service endpoint localhost is not valid or missing")

	// Error case: invalid ca file
	defer os.RemoveAll(dir) // clean up
	channelCmd.AddCommand(cmd)
	args = []string{"create", "-c", mockchannel, "-f", file, "-o", "localhost:7050", "--tls", "true", "--cafile", dir + "/ca.pem"}
	channelCmd.SetArgs(args)
	err = channelCmd.Execute()
	require.Error(t, err)
	t.Log(err)
	require.Contains(t, err.Error(), "no such file or directory")
}

func TestSanityCheckAndSignChannelCreateTx(t *testing.T) {
	defer resetFlags()

	signer := &mock.SignerSerializer{}
	env := &cb.Envelope{}
	env.Payload = make([]byte, 10)
	var err error

	// Error case 1
	_, err = sanityCheckAndSignConfigTx(env, signer)
	require.Error(t, err, "Error expected for nil payload")
	require.Contains(t, err.Error(), "bad payload")

	// Error case 2
	p := &cb.Payload{Header: nil}
	data, err1 := proto.Marshal(p)
	require.NoError(t, err1)
	env = &cb.Envelope{Payload: data}
	_, err = sanityCheckAndSignConfigTx(env, signer)
	require.Error(t, err, "Error expected for bad payload header")
	require.Contains(t, err.Error(), "bad header")

	// Error case 3
	bites := bytes.NewBufferString("foo").Bytes()
	p = &cb.Payload{Header: &cb.Header{ChannelHeader: bites}}
	data, err = proto.Marshal(p)
	require.NoError(t, err)
	env = &cb.Envelope{Payload: data}
	_, err = sanityCheckAndSignConfigTx(env, signer)
	require.Error(t, err, "Error expected for bad channel header")
	require.Contains(t, err.Error(), "could not unmarshall channel header")

	// Error case 4
	mockchannel := "mockchannel"
	cid := channelID
	channelID = mockchannel
	defer func() {
		channelID = cid
	}()
	ch := &cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG_UPDATE), ChannelId: mockchannel}
	data, err = proto.Marshal(ch)
	require.NoError(t, err)
	p = &cb.Payload{Header: &cb.Header{ChannelHeader: data}, Data: bytes.NewBufferString("foo").Bytes()}
	data, err = proto.Marshal(p)
	require.NoError(t, err)
	env = &cb.Envelope{Payload: data}
	_, err = sanityCheckAndSignConfigTx(env, signer)
	require.Error(t, err, "Error expected for bad payload data")
	require.Contains(t, err.Error(), "Bad config update env")

	// Error case 5
	signer = &mock.SignerSerializer{}
	signer.SerializeReturns(nil, errors.New("bad signer header"))
	env, err = protoutil.CreateSignedEnvelope(
		cb.HeaderType_CONFIG_UPDATE,
		"mockchannel",
		nil,
		&cb.ConfigEnvelope{},
		0,
		0)
	require.NoError(t, err)
	_, err = sanityCheckAndSignConfigTx(env, signer)
	require.EqualError(t, err, "bad signer header")

	// Error case 6
	signer = &mock.SignerSerializer{}
	signer.SignReturns(nil, errors.New("signer failed to sign"))
	env, err = protoutil.CreateSignedEnvelope(
		cb.HeaderType_CONFIG_UPDATE,
		"mockchannel",
		nil,
		&cb.ConfigEnvelope{},
		0,
		0)
	require.NoError(t, err)
	_, err = sanityCheckAndSignConfigTx(env, signer)
	require.EqualError(t, err, "signer failed to sign")
}
