/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package producer

import (
	"crypto/tls"
	"crypto/x509"
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

	"io/ioutil"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	coreutil "github.com/hyperledger/fabric/core/testutil"
	"github.com/hyperledger/fabric/events/consumer"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	msp2 "github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/peer"
)

type Adapter struct {
	sync.RWMutex
	notfy chan struct{}
	count int
}

var adapter *Adapter
var ehClient *consumer.EventsClient
var ehServer *EventsServer

var timeWindow = time.Duration(15 * time.Minute)
var testCert = &x509.Certificate{
	Raw: []byte("test"),
}

const mutualTLS = true

func (a *Adapter) GetInterestedEvents() ([]*pb.Interest, error) {
	return []*pb.Interest{
		{EventType: pb.EventType_BLOCK},
		{EventType: pb.EventType_FILTEREDBLOCK},
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

func (a *Adapter) Recv(msg *pb.Event) (bool, error) {
	switch x := msg.Event.(type) {
	case *pb.Event_Block, *pb.Event_ChaincodeEvent, *pb.Event_Register, *pb.Event_Unregister, *pb.Event_FilteredBlock:
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

func createRegisterEvent(timestamp *timestamp.Timestamp, tlsCert *x509.Certificate) (*pb.Event, error) {
	events := make([]*pb.Interest, 2)
	events[0] = &pb.Interest{
		EventType: pb.EventType_BLOCK,
	}
	events[1] = &pb.Interest{
		EventType: pb.EventType_BLOCK,
		ChainID:   util.GetTestChainID(),
	}

	evt := &pb.Event{
		Event: &pb.Event_Register{
			Register: &pb.Register{
				Events: events,
			},
		},
		Creator:   signerSerialized,
		Timestamp: timestamp,
	}
	if tlsCert != nil {
		evt.TlsCertHash = util.ComputeSHA256(tlsCert.Raw)
	}
	return evt, nil
}

func createSignedRegisterEvent(timestamp *timestamp.Timestamp, cert *x509.Certificate) (*pb.SignedEvent, error) {
	evt, err := createRegisterEvent(timestamp, cert)
	if err != nil {
		return nil, err
	}
	sEvt, err := utils.GetSignedEvent(evt, signer)
	if err != nil {
		return nil, err
	}
	return sEvt, nil
}

var r *rand.Rand

func corrupt(bytes []byte) {
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().Unix()))
	}

	bytes[r.Int31n(int32(len(bytes)))]--
}

func createExpiredIdentity(t *testing.T) []byte {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "expiredCert.pem"))
	assert.NoError(t, err)
	sId := &msp2.SerializedIdentity{
		IdBytes: certBytes,
	}
	serializedIdentity, err := proto.Marshal(sId)
	assert.NoError(t, err)
	return serializedIdentity
}

func TestSignedEvent(t *testing.T) {
	recvChan := make(chan *streamEvent)
	sendChan := make(chan *pb.Event)
	stream := &mockEventStream{recvChan: recvChan, sendChan: sendChan}
	mockHandler := &handler{ChatStream: stream, eventProcessor: gEventProcessor}
	backupSerializedIdentity := signerSerialized
	signerSerialized = createExpiredIdentity(t)
	// get a test event
	evt, err := createRegisterEvent(nil, nil)
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

	// validate it. Expected to fail because the identity expired
	_, err = mockHandler.validateEventMessage(sEvt)
	assert.Equal(t, err.Error(), "identity expired")
	if err == nil {
		t.Fatalf("validateEventMessage succeeded but should have failed")
		return
	}

	// Restore the original legit serialized identity
	signerSerialized = backupSerializedIdentity
	evt, err = createRegisterEvent(nil, nil)
	if err != nil {
		t.Fatalf("createEvent failed, err %s", err)
		return
	}

	// sign it
	sEvt, err = utils.GetSignedEvent(evt, signer)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	// validate it. Expected to succeed
	_, err = mockHandler.validateEventMessage(sEvt)
	if err != nil {
		t.Fatalf("validateEventMessage failed, err %s", err)
		return
	}

	// mess with the signature
	corrupt(sEvt.Signature)

	// validate it, it should fail
	_, err = mockHandler.validateEventMessage(sEvt)
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
	_, err = mockHandler.validateEventMessage(sEvt)
	if err == nil {
		t.Fatalf("validateEventMessage should have failed")
		return
	}
}

func TestReceiveAnyMessage(t *testing.T) {
	block := testutil.ConstructTestBlock(t, 1, 10, 100)

	bevent, fbevent, _, err := CreateBlockEvents(block)
	if err != nil {
		t.Fail()
		t.Logf("Error processing block for events %s", err)
	}

	if err = Send(bevent); err != nil {
		t.Fail()
		t.Logf("Error sending block event: %s", err)
	}
	if err = Send(fbevent); err != nil {
		t.Fail()
		t.Logf("Error sending filtered block event: %s", err)
	}
	// receive 2 messages - a block and a filtered block event
	for i := 0; i < 2; i++ {
		select {
		case <-adapter.notfy:
		case <-time.After(1 * time.Second):
			t.Fail()
			t.Logf("timed out on message")
		}
	}
	assert.Equal(t, 0, len(gEventProcessor.eventChannel))
}

func TestReceiveEventsBlockingSend(t *testing.T) {
	recvChan := make(chan *streamEvent)
	defer close(recvChan)
	delayChan := make(chan struct{})
	defer close(delayChan)
	streamEmbed := mockEventStream{
		recvChan: recvChan,
	}
	delayBlockEventsStream := &mockstreamDelayBlockEvents{
		mockEventStream: streamEmbed,
		DelayChan:       delayChan,
	}
	handler := newHandler(delayBlockEventsStream, gEventProcessor)

	e, err := createRegisterEvent(nil, nil)
	assert.NoError(t, err)
	sEvt, err := utils.GetSignedEvent(e, signer)
	assert.NoError(t, err)

	err = handler.HandleMessage(sEvt)
	assert.NoError(t, err)

	for i := 1; i < 3; i++ {
		t.Run(fmt.Sprint("send block", i), func(t *testing.T) {
			block := testutil.ConstructTestBlock(t, uint64(i), 10, 100)
			bevent, fbevent, _, err := CreateBlockEvents(block)
			if err != nil {
				t.Fail()
				t.Logf("Error processing block for events %s", err)
			}
			if err = Send(bevent); err != nil {
				t.Fail()
				t.Logf("Error sending block event: %s", err)
			}
			if err = Send(fbevent); err != nil {
				t.Fail()
				t.Logf("Error sending filtered block event: %s", err)
			}
			// receive 2 messages - a block and a filtered block event
			for i := 0; i < 2; i++ {
				select {
				case <-adapter.notfy:
				case <-time.After(1 * time.Second):
					t.Fail()
					t.Logf("timed out on message")
				}
			}
			assert.Equal(t, 0, len(gEventProcessor.eventChannel))
		})
	}
}

func TestUnregister(t *testing.T) {
	block := testutil.ConstructTestBlock(t, 1, 10, 100)
	bevent, _, _, err := CreateBlockEvents(block)
	if err != nil {
		t.Fail()
		t.Logf("Error processing block for events %s", err)
	}
	if err = Send(bevent); err != nil {
		t.Fail()
		t.Logf("Error sending block event: %s", err)
	}

	select {
	case <-adapter.notfy:
	case <-time.After(1 * time.Second):
		t.Fail()
		t.Logf("timed out on message")
	}

	ehClient.UnregisterAsync([]*pb.Interest{{EventType: pb.EventType_BLOCK}})
	select {
	case <-adapter.notfy:
	case <-time.After(1 * time.Second):
		t.Fail()
		t.Logf("should have received unreg")
	}

	if err = Send(bevent); err != nil {
		t.Fail()
		t.Logf("Error sending block event: %s", err)
	}
	select {
	case <-adapter.notfy:
		t.Fail()
		t.Logf("should NOT have received event")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestRegister_outOfTimeWindow(t *testing.T) {
	interestedEvents, err := adapter.GetInterestedEvents()
	assert.NoError(t, err)
	config := &consumer.RegistrationConfig{
		InterestedEvents: interestedEvents,
		Timestamp:        &timestamp.Timestamp{Seconds: 0},
	}

	ehClient.RegisterAsync(config)
	select {
	case <-adapter.notfy:
		t.Fail()
		t.Logf("register with out of range timestamp should fail")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestRegister_MutualTLS(t *testing.T) {
	m := newMockEventhub()
	defer close(m.recvChan)

	go ehServer.Chat(m)

	resetEventProcessor(mutualTLS)
	defer resetEventProcessor(!mutualTLS)

	sEvt, err := createSignedRegisterEvent(util.CreateUtcTimestamp(), testCert)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	m.recvChan <- &streamEvent{event: sEvt}
	select {
	case registrationReply := <-m.sendChan:
		if registrationReply.GetRegister() == nil {
			t.Fatalf("Received an error on the reply channel")
		}
	case <-time.After(time.Second):
		t.Fatalf("Timed out waiting to get registration response")
	}

	var wrongCert = &x509.Certificate{
		Raw: []byte("wrong"),
	}

	sEvt, err = createSignedRegisterEvent(util.CreateUtcTimestamp(), wrongCert)
	if err != nil {
		t.Fatalf("GetSignedEvent failed, err %s", err)
		return
	}

	m.recvChan <- &streamEvent{event: sEvt}
	select {
	case <-m.sendChan:
		t.Fatalf("Received a response when none was expected")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestRegister_ExpiredIdentity(t *testing.T) {
	m := newMockEventhub()
	defer close(m.recvChan)

	go ehServer.Chat(m)

	publishBlock := func() {
		gEventProcessor.eventChannel <- &pb.Event{
			Event: &pb.Event_Block{
				Block: &common.Block{
					Header: &common.BlockHeader{
						Number: 100,
					},
				},
			},
		}
	}

	expireSessions := func() {
		gEventProcessor.RLock()
		handlerList := gEventProcessor.eventConsumers[pb.EventType_BLOCK]
		handlerList.Lock()
		for k := range handlerList.handlers {
			// Artificially move the session end time a minute into the past
			k.sessionEndTime = time.Now().Add(-1 * time.Minute)
		}
		handlerList.Unlock()
		gEventProcessor.RUnlock()
	}

	sEvt, err := createSignedRegisterEvent(util.CreateUtcTimestamp(), nil)
	assert.NoError(t, err)
	m.recvChan <- &streamEvent{event: sEvt}

	// Wait for register Ack
	select {
	case <-m.sendChan:
	case <-time.After(10 * time.Millisecond):
		assert.Fail(t, "Didn't receive back a register ack on time")
	}
	assert.Equal(t, 0, len(gEventProcessor.eventChannel))

	// Publish a block and make sure we receive it
	publishBlock()
	select {
	case resp := <-m.sendChan:
		assert.Equal(t, uint64(100), resp.GetBlock().Header.Number)
	case <-time.After(500 * time.Millisecond):
		assert.Fail(t, "Didn't receive the block on time, but should have")
	}
	assert.Equal(t, 0, len(gEventProcessor.eventChannel))

	// Expire the sessions, and publish a block again
	expireSessions()
	publishBlock()
	// Make sure we don't receive it
	select {
	case resp := <-m.sendChan:
		assert.NotEqual(t, uint64(100), resp.GetBlock().Header.Number)
		t.Fatalf("Received a block (%v) but wasn't supposed to", resp.GetBlock())
	case <-time.After(10 * time.Millisecond):
	}
	assert.Equal(t, 0, len(gEventProcessor.eventChannel))
}

func TestFailReceive(t *testing.T) {
	unsupportedEvent := &pb.Event{Event: &pb.Event_ChaincodeEvent{}}
	emptyEvent := &pb.Event{Event: &pb.Event_Block{}}
	for _, e := range []*pb.Event{unsupportedEvent, emptyEvent} {
		if err := Send(e); err != nil {
			t.Fail()
			t.Logf("Error sending message %s", err)
		}

		select {
		case <-adapter.notfy:
			t.Fail()
			t.Logf("should NOT have received event1")
		case <-time.After(10 * time.Millisecond):
		}
		assert.Equal(t, 0, len(gEventProcessor.eventChannel))
	}
}

func resetEventProcessor(useMutualTLS bool) {
	extract := func(msg proto.Message) []byte {
		evt, isEvent := msg.(*pb.Event)
		if !isEvent || evt == nil {
			return nil
		}
		return evt.TlsCertHash
	}
	gEventProcessor.BindingInspector = comm.NewBindingInspector(useMutualTLS, extract)

	// reset the event consumers
	gEventProcessor.eventConsumers = make(map[pb.EventType]*handlerList)

	// re-register the event types
	gEventProcessor.addSupportedEventTypes()
}

func TestNewEventsServer(t *testing.T) {
	config := &EventsServerConfig{
		BufferSize:  100,
		Timeout:     0,
		SendTimeout: 0,
		TimeWindow:  0,
	}
	doubleCreation := func() {
		NewEventsServer(config)
	}
	assert.Panics(t, doubleCreation)

	assert.NotNil(t, ehServer, "nil EventServer found")
}

type streamEvent struct {
	event *pb.SignedEvent
	err   error
}

type mockEventStream struct {
	grpc.ServerStream
	recvChan chan *streamEvent
	sendChan chan *pb.Event
}

func (mockEventStream) Context() context.Context {
	p := &peer.Peer{}
	p.AuthInfo = credentials.TLSInfo{
		State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				testCert,
			},
		},
	}
	return peer.NewContext(context.Background(), p)
}

type mockEventhub struct {
	mockEventStream
}

func newMockEventhub() *mockEventhub {
	return &mockEventhub{
		mockEventStream{
			recvChan: make(chan *streamEvent),
			sendChan: make(chan *pb.Event),
		},
	}
}

func (m *mockEventStream) Send(evt *pb.Event) error {
	m.sendChan <- evt
	return nil
}

func (m *mockEventStream) Recv() (*pb.SignedEvent, error) {
	msg, ok := <-m.recvChan
	if !ok {
		return nil, io.EOF
	}
	if msg.err != nil {
		return nil, msg.err
	}
	return msg.event, nil
}

type mockstreamDelayBlockEvents struct {
	mockEventStream
	DelayChan chan struct{}
}

func (m *mockstreamDelayBlockEvents) Send(evt *pb.Event) error {
	// only delay block events, not registration
	switch evt.Event.(type) {
	case *pb.Event_Block:
		<-m.DelayChan
	default:
	}
	return nil
}

func TestChat(t *testing.T) {
	m := newMockEventhub()
	defer close(m.recvChan)
	go ehServer.Chat(m)

	e, err := createRegisterEvent(nil, nil)
	sEvt, err := utils.GetSignedEvent(e, signer)
	assert.NoError(t, err)
	m.recvChan <- &streamEvent{event: sEvt}
	go ehServer.Chat(m)
	m.recvChan <- &streamEvent{event: &pb.SignedEvent{}}
	go ehServer.Chat(m)
	m.mockEventStream.recvChan <- &streamEvent{err: io.EOF}
	go ehServer.Chat(m)
	m.recvChan <- &streamEvent{err: errors.New("err")}
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

	extract := func(msg proto.Message) []byte {
		evt, isEvent := msg.(*pb.Event)
		if !isEvent || evt == nil {
			return nil
		}
		return evt.TlsCertHash
	}

	// use a buffer of 100 and timeout of 10ms
	timeout := 10 * time.Millisecond
	ehConfig := &EventsServerConfig{
		BufferSize:       uint(100),
		Timeout:          timeout,
		SendTimeout:      timeout,
		TimeWindow:       time.Minute,
		BindingInspector: comm.NewBindingInspector(!mutualTLS, extract)}

	ehServer = NewEventsServer(ehConfig)
	pb.RegisterEventsServer(grpcServer, ehServer)

	go grpcServer.Serve(lis)

	receiveChan := make(chan struct{})
	adapter = &Adapter{notfy: receiveChan}
	ehClient, _ = consumer.NewEventsClient(peerAddress, timeout, adapter)
	// create a new client where the adapter's recv just waits on the message, then see if the regular
	// client above can still receive
	if err = ehClient.Start(); err != nil {
		fmt.Printf("could not start chat %s\n", err)
		ehClient.Stop()
		return
	}
	os.Exit(m.Run())
}
