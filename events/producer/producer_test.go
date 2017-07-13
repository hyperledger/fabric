/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package producer

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config"
	coreutil "github.com/hyperledger/fabric/core/testutil"
	"github.com/hyperledger/fabric/events/consumer"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/peer"
	ehpb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type Adapter struct {
	sync.RWMutex
	notfy chan struct{}
	count int
}

var adapter *Adapter
var obcEHClient *consumer.EventsClient
var ehServer *EventsServer

func (a *Adapter) GetInterestedEvents() ([]*ehpb.Interest, error) {
	return []*ehpb.Interest{
		&ehpb.Interest{EventType: ehpb.EventType_BLOCK},
		&ehpb.Interest{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event1"}}},
		&ehpb.Interest{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event2"}}},
		&ehpb.Interest{EventType: ehpb.EventType_REGISTER, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event3"}}},
		&ehpb.Interest{EventType: ehpb.EventType_REJECTION, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event4"}}},
	}, nil
}

func (a *Adapter) updateCountNotify() {
	a.Lock()
	a.count--
	if a.count <= 0 {
		a.notfy <- struct{}{}
	}
	a.Unlock()
}

func (a *Adapter) Recv(msg *ehpb.Event) (bool, error) {
	switch x := msg.Event.(type) {
	case *ehpb.Event_Block, *ehpb.Event_ChaincodeEvent, *ehpb.Event_Register, *ehpb.Event_Unregister:
		a.updateCountNotify()
	case nil:
		// The field is not set.
		return false, fmt.Errorf("event not set")
	default:
		return false, fmt.Errorf("unexpected type %T", x)
	}
	return true, nil
}

func (a *Adapter) Disconnected(err error) {
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}
}

func createEvent() (*peer.Event, error) {
	events := make([]*peer.Interest, 2)
	events[0] = &peer.Interest{
		EventType: peer.EventType_BLOCK,
	}
	events[1] = &peer.Interest{
		EventType: peer.EventType_BLOCK,
		ChainID:   util.GetTestChainID(),
	}

	evt := &peer.Event{
		Event: &peer.Event_Register{
			Register: &peer.Register{
				Events: events,
			},
		},
		Creator: signerSerialized,
	}

	return evt, nil
}

var r *rand.Rand

func corrupt(bytes []byte) {
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().Unix()))
	}

	bytes[r.Int31n(int32(len(bytes)))]--
}

func TestSignedEvent(t *testing.T) {
	// get a test event
	evt, err := createEvent()
	if err != nil {
		t.Fatalf("createEvent failed, err %s", err)
		return
	}

	// sign it
	sEvt, err := utils.GetSignedEvent(evt, signer)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	// validate it. Expected to succeed
	_, err = validateEventMessage(sEvt)
	if err != nil {
		t.Fatalf("validateEventMessage failed, err %s", err)
		return
	}

	// mess with the signature
	corrupt(sEvt.Signature)

	// validate it, it should fail
	_, err = validateEventMessage(sEvt)
	if err == nil {
		t.Fatalf("validateEventMessage should have failed")
		return
	}

	// get a bad signing identity
	badSigner, err := mmsp.NewNoopMsp().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatal("couldn't get noop signer")
		return
	}

	// sign it again with the bad signer
	sEvt, err = utils.GetSignedEvent(evt, badSigner)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	// validate it, it should fail
	_, err = validateEventMessage(sEvt)
	if err == nil {
		t.Fatalf("validateEventMessage should have failed")
		return
	}
}

func createTestChaincodeEvent(tid string, typ string) *ehpb.Event {
	emsg := CreateChaincodeEvent(&ehpb.ChaincodeEvent{ChaincodeId: tid, EventName: typ})
	return emsg
}

// Test the invocation of a transaction.
func TestReceiveMessage(t *testing.T) {
	var err error

	adapter.count = 1
	emsg := createTestChaincodeEvent("0xffffffff", "event1")
	if err = Send(emsg); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	select {
	case <-adapter.notfy:
	case <-time.After(5 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}
}

func TestReceiveAnyMessage(t *testing.T) {
	var err error

	adapter.count = 1
	block := testutil.ConstructTestBlock(t, 1, 10, 100)
	if err = SendProducerBlockEvent(block); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	emsg := createTestChaincodeEvent("0xffffffff", "event2")
	if err = Send(emsg); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	//receive 2 messages - a block and a chaincode event
	for i := 0; i < 2; i++ {
		select {
		case <-adapter.notfy:
		case <-time.After(5 * time.Second):
			t.Fail()
			t.Logf("timed out on message")
		}
	}
}

func TestReceiveCCWildcard(t *testing.T) {
	var err error

	adapter.count = 1
	obcEHClient.RegisterAsync([]*ehpb.Interest{&ehpb.Interest{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: ""}}}})

	select {
	case <-adapter.notfy:
	case <-time.After(2 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}

	adapter.count = 1
	emsg := createTestChaincodeEvent("0xffffffff", "wildcardevent")
	if err = Send(emsg); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	select {
	case <-adapter.notfy:
	case <-time.After(2 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}
	adapter.count = 1
	obcEHClient.UnregisterAsync([]*ehpb.Interest{&ehpb.Interest{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: ""}}}})

	select {
	case <-adapter.notfy:
	case <-time.After(2 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}
}

func TestFailReceive(t *testing.T) {
	var err error

	adapter.count = 1
	emsg := createTestChaincodeEvent("badcc", "event1")
	if err = Send(emsg); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	select {
	case <-adapter.notfy:
		t.Fail()
		t.Logf("should NOT have received event1")
	case <-time.After(2 * time.Second):
	}
}

func TestUnregister(t *testing.T) {
	var err error
	obcEHClient.RegisterAsync([]*ehpb.Interest{&ehpb.Interest{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event10"}}}})

	adapter.count = 1
	select {
	case <-adapter.notfy:
	case <-time.After(2 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}

	emsg := createTestChaincodeEvent("0xffffffff", "event10")
	if err = Send(emsg); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	adapter.count = 1
	select {
	case <-adapter.notfy:
	case <-time.After(2 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}
	obcEHClient.UnregisterAsync([]*ehpb.Interest{&ehpb.Interest{EventType: ehpb.EventType_CHAINCODE, RegInfo: &ehpb.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &ehpb.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event10"}}}})
	adapter.count = 1
	select {
	case <-adapter.notfy:
	case <-time.After(2 * time.Second):
		t.Fail()
		t.Logf("should have received unreg")
	}

	adapter.count = 1
	emsg = createTestChaincodeEvent("0xffffffff", "event10")
	if err = Send(emsg); err != nil {
		t.Fail()
		t.Logf("Error sending message %s", err)
	}

	select {
	case <-adapter.notfy:
		t.Fail()
		t.Logf("should NOT have received event1")
	case <-time.After(5 * time.Second):
	}

}

func TestNewEventsServer(t *testing.T) {
	doubleCreation := func() {
		NewEventsServer(
			uint(viper.GetInt("peer.events.buffersize")),
			viper.GetDuration("peer.events.timeout"))
	}
	assert.Panics(t, doubleCreation)

	assert.NotNil(t, ehServer, "nil EventServer found")
}

type mockstream struct {
	c chan *streamEvent
}

type streamEvent struct {
	event *peer.SignedEvent
	err   error
}

func (*mockstream) Send(*peer.Event) error {
	return nil
}

func (s *mockstream) Recv() (*peer.SignedEvent, error) {
	se := <-s.c
	if se.err == nil {
		return se.event, nil
	}
	return nil, se.err
}

func (*mockstream) SetHeader(metadata.MD) error {
	panic("not implemented")
}

func (*mockstream) SendHeader(metadata.MD) error {
	panic("not implemented")
}

func (*mockstream) SetTrailer(metadata.MD) {
	panic("not implemented")
}

func (*mockstream) Context() context.Context {
	panic("not implemented")
}

func (*mockstream) SendMsg(m interface{}) error {
	panic("not implemented")
}

func (*mockstream) RecvMsg(m interface{}) error {
	panic("not implemented")
}

func TestChat(t *testing.T) {
	recvChan := make(chan *streamEvent)
	stream := &mockstream{c: recvChan}
	go ehServer.Chat(stream)
	e, err := createEvent()
	sEvt, err := utils.GetSignedEvent(e, signer)
	assert.NoError(t, err)
	recvChan <- &streamEvent{event: sEvt}
	recvChan <- &streamEvent{event: &peer.SignedEvent{}}
	go ehServer.Chat(stream)
	recvChan <- &streamEvent{err: io.EOF}
	go ehServer.Chat(stream)
	recvChan <- &streamEvent{err: errors.New("err")}
	time.Sleep(time.Second)
}

var signer msp.SigningIdentity
var signerSerialized []byte

func TestMain(m *testing.M) {
	// setup crypto algorithms
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp, err %s", err)
		os.Exit(-1)
		return
	}

	signer, err = mgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Println("Could not get signer")
		os.Exit(-1)
		return
	}

	signerSerialized, err = signer.Serialize()
	if err != nil {
		fmt.Println("Could not serialize identity")
		os.Exit(-1)
		return
	}
	coreutil.SetupTestConfig()
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(config.GetPath("peer.tls.cert.file"), config.GetPath("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	peerAddress = "0.0.0.0:60303"

	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		fmt.Printf("Error starting events listener %s....not doing tests", err)
		return
	}

	// Register EventHub server
	// use a buffer of 100 and blocking timeout
	viper.Set("peer.events.buffersize", 100)
	viper.Set("peer.events.timeout", 0)

	ehServer = NewEventsServer(
		uint(viper.GetInt("peer.events.buffersize")),
		viper.GetDuration("peer.events.timeout"))
	ehpb.RegisterEventsServer(grpcServer, ehServer)

	go grpcServer.Serve(lis)

	var regTimeout = 5 * time.Second
	done := make(chan struct{})
	adapter = &Adapter{notfy: done}
	obcEHClient, _ = consumer.NewEventsClient(peerAddress, regTimeout, adapter)
	if err = obcEHClient.Start(); err != nil {
		fmt.Printf("could not start chat %s\n", err)
		obcEHClient.Stop()
		return
	}

	time.Sleep(2 * time.Second)
	os.Exit(m.Run())
}
