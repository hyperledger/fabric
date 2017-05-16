/*
 Copyright Digital Asset Holdings, LLC 2017 All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package channel

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/peer/common"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"google.golang.org/grpc"
)

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
	go srv.Serve(lsnr)
	o := &timeoutOrderer{Server: srv,
		Listener:         lsnr,
		t:                t,
		nextExpectedSeek: uint64(1),
		blockChannel:     make(chan uint64, 1),
		counter:          int(1),
	}
	orderer.RegisterAtomicBroadcastServer(srv, o)
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

/// mock deliver client for UT
type mockDeliverClient struct {
	err error
}

func (m *mockDeliverClient) readBlock() (*cb.Block, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &cb.Block{}, nil
}

func (m *mockDeliverClient) getBlock() (*cb.Block, error) {
	b, err := m.readBlock()
	if err != nil {
		return nil, err
	}

	return b, nil
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
		panic(fmt.Errorf("Fatal error when reading MSP config: err %s\n", err))
	}
}

func mockBroadcastClientFactory() (common.BroadcastClient, error) {
	return common.GetMockBroadcastClient(nil), nil
}

func TestCreateChain(t *testing.T) {
	InitMSP()

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

func TestCreateChainWithDefaultAnchorPeers(t *testing.T) {
	InitMSP()

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
	InitMSP()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := errors.New("timeout waiting for channel creation")
	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{sendErr},
	}
	fakeOrderer := newOrderer(7050, t)
	defer fakeOrderer.Shutdown()

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected create command to succeed")
	}
}

func TestCreateChainWithTimeoutErr(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := errors.New("timeout waiting for channel creation")
	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{sendErr},
	}
	fakeOrderer := newOrderer(7050, t)
	defer fakeOrderer.Shutdown()

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050", "-t", "1"}
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

func TestCreateChainBCFail(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := errors.New("send create tx failed")

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
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := fmt.Errorf("failed connecting")

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: func() (common.BroadcastClient, error) {
			return common.GetMockBroadcastClient(sendErr), nil
		},
		Signer:        signer,
		DeliverClient: &mockDeliverClient{sendErr},
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

	if err = ioutil.WriteFile(filename, data, 0644); err != nil {
		return nil, err
	}

	return env, nil
}

func TestCreateChainFromTx(t *testing.T) {
	InitMSP()

	mockchannel := "mockchannel"

	dir, err := ioutil.TempDir("/tmp", "createtestfromtx-")
	if err != nil {
		t.Fatalf("couldn't create temp dir")
	}

	defer os.RemoveAll(dir) // clean up

	//this could be created by the create command
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

	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, mockchannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err != nil {
		t.Errorf("create chain failed")
	}
}

func TestCreateChainInvalidTx(t *testing.T) {
	InitMSP()

	mockchannel := "mockchannel"

	dir, err := ioutil.TempDir("/tmp", "createinvaltest-")
	if err != nil {
		t.Fatalf("couldn't create temp dir")
	}

	defer os.RemoveAll(dir) // clean up

	//this is created by create command
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

	//bad type CONFIG
	if _, err = createTxFile(file, cb.HeaderType_CONFIG, mockchannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	defer os.Remove(file)

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}

	//bad channel name - does not match one specified in command
	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, "different_channel"); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}

	//empty channel
	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, ""); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}
}
