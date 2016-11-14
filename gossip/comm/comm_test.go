/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package comm

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"crypto/tls"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func init() {
	rand.Seed(42)
	SetDialTimeout(time.Duration(300) * time.Millisecond)
}

func acceptAll(msg interface{}) bool {
	return true
}

var naiveSec = &naiveSecProvider{}

type naiveSecProvider struct {
}

func (*naiveSecProvider) IsEnabled() bool {
	return true
}

func (*naiveSecProvider) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*naiveSecProvider) Verify(vkID, signature, message []byte) error {
	if bytes.Equal(signature, message) {
		return nil
	}
	return fmt.Errorf("Failed verifying")
}

func newCommInstance(port int, sec SecurityProvider) (Comm, error) {
	endpoint := fmt.Sprintf("localhost:%d", port)
	inst, err := NewCommInstanceWithServer(port, sec, []byte(endpoint))
	return inst, err
}

func TestHandshake(t *testing.T) {
	comm1, _ := newCommInstance(9611, naiveSec)
	defer comm1.Stop()

	ta := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})
	conn, err := grpc.Dial("localhost:9611", grpc.WithTransportCredentials(&authCreds{tlsCreds: ta}), grpc.WithBlock(), grpc.WithTimeout(time.Second))
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}

	// happy path
	clientTLSUnique := ExtractTLSUnique(stream.Context())
	sig, err := naiveSec.Sign(clientTLSUnique)
	assert.NoError(t, err, "%v", err)
	msg := createConnectionMsg(common.PKIidType("localhost:9610"), sig)
	stream.Send(msg)
	msg, err = stream.Recv()
	assert.NoError(t, err, "%v", err)
	assert.Equal(t, clientTLSUnique, msg.GetConn().Sig)
	assert.Equal(t, []byte("localhost:9611"), msg.GetConn().PkiID)
	time.Sleep(time.Second)
	msg2Send := createGossipMsg()
	nonce := uint64(rand.Int())
	msg2Send.Nonce = nonce
	rcvChan := make(chan *proto.GossipMessage, 1)
	go func() {
		m := <-comm1.Accept(acceptAll)
		rcvChan <- m.GetGossipMessage()
	}()
	time.Sleep(time.Second)
	go stream.Send(msg2Send)
	time.Sleep(time.Second)
	assert.Equal(t, 1, len(rcvChan))
	var receivedMsg *proto.GossipMessage
	select {
	case receivedMsg = <-rcvChan:
		break
	case <-time.NewTicker(time.Duration(time.Second * 2)).C:
		assert.Fail(t, "Timed out waiting for received message")
		break
	}

	assert.Equal(t, nonce, receivedMsg.Nonce)

	// negative path, nothing should be read from the channel because the signature is wrong
	rcvChan = make(chan *proto.GossipMessage, 1)
	go func() {
		m := <-comm1.Accept(acceptAll)
		if m == nil {
			return
		}
		rcvChan <- m.GetGossipMessage()
	}()
	conn, err = grpc.Dial("localhost:9611", grpc.WithTransportCredentials(&authCreds{tlsCreds: ta}), grpc.WithBlock(), grpc.WithTimeout(time.Second))
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	cl = proto.NewGossipClient(conn)
	stream, err = cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return
	}
	clientTLSUnique = ExtractTLSUnique(stream.Context())
	sig, err = naiveSec.Sign(clientTLSUnique)
	assert.NoError(t, err, "%v", err)
	// ruin the signature
	if sig[0] == 0 {
		sig[0] = 1
	} else {
		sig[0] = 0
	}
	msg = createConnectionMsg(common.PKIidType("localhost:9612"), sig)
	stream.Send(msg)
	msg, err = stream.Recv()
	assert.Equal(t, []byte("localhost:9611"), msg.GetConn().PkiID)
	assert.NoError(t, err, "%v", err)
	msg2Send = createGossipMsg()
	nonce = uint64(rand.Int())
	msg2Send.Nonce = nonce
	stream.Send(msg2Send)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(rcvChan))
}

func TestBasic(t *testing.T) {
	comm1, _ := newCommInstance(2000, naiveSec)
	comm2, _ := newCommInstance(3000, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	time.Sleep(time.Duration(3) * time.Second)
	msgs := make(chan *proto.GossipMessage, 2)
	go func() {
		m := <-comm2.Accept(acceptAll)
		msgs <- m.GetGossipMessage()
	}()
	go func() {
		m := <-comm1.Accept(acceptAll)
		msgs <- m.GetGossipMessage()
	}()
	comm1.Send(createGossipMsg(), &RemotePeer{PKIID: []byte("localhost:3000"), Endpoint: "localhost:3000"})
	time.Sleep(time.Second)
	comm2.Send(createGossipMsg(), &RemotePeer{PKIID: []byte("localhost:2000"), Endpoint: "localhost:2000"})
	time.Sleep(time.Second)
	assert.Equal(t, 2, len(msgs))
}

func TestBlackListPKIid(t *testing.T) {
	comm1, _ := newCommInstance(1611, naiveSec)
	comm2, _ := newCommInstance(1612, naiveSec)
	comm3, _ := newCommInstance(1613, naiveSec)
	comm4, _ := newCommInstance(1614, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	defer comm3.Stop()
	defer comm4.Stop()

	reader := func(out chan uint64, in <-chan ReceivedMessage) {
		for {
			msg := <-in
			if msg == nil {
				return
			}
			out <- msg.GetGossipMessage().Nonce
		}
	}

	sender := func(comm Comm, port int, n int) {
		endpoint := fmt.Sprintf("localhost:%d", port)
		for i := 0; i < n; i++ {
			comm.Send(createGossipMsg(), &RemotePeer{Endpoint: endpoint, PKIID: []byte(endpoint)})
			time.Sleep(time.Duration(1) * time.Second)
		}
	}

	out1 := make(chan uint64, 5)
	out2 := make(chan uint64, 5)
	out3 := make(chan uint64, 10)
	out4 := make(chan uint64, 10)

	go reader(out1, comm1.Accept(acceptAll))
	go reader(out2, comm2.Accept(acceptAll))
	go reader(out3, comm3.Accept(acceptAll))
	go reader(out4, comm4.Accept(acceptAll))

	// have comm1 BL comm3
	comm1.BlackListPKIid([]byte("localhost:1613"))

	// make comm3 send to 1 and 2
	go sender(comm3, 1611, 5)
	go sender(comm3, 1612, 5)

	// make comm1 and comm2 send to comm3
	go sender(comm1, 1613, 5)
	go sender(comm2, 1613, 5)

	// make comm1 and comm2 send to comm4 which is not blacklisted
	go sender(comm1, 1614, 5)
	go sender(comm2, 1614, 5)

	time.Sleep(time.Duration(1) * time.Second)

	// blacklist comm3 mid-sending
	comm2.BlackListPKIid([]byte("localhost:1613"))
	time.Sleep(time.Duration(5) * time.Second)

	assert.Equal(t, 0, len(out1), "Comm instance 1 received messages(%d) from comm3 although comm3 is black listed", len(out1))
	assert.True(t, len(out2) < 2, "Comm instance 2 received too many messages(%d) from comm3 although comm3 is black listed", len(out2))
	assert.True(t, len(out3) < 3, "Comm instance 3 received too many messages(%d) although black listed", len(out3))
	assert.Equal(t, 10, len(out4), "Comm instance 4 didn't receive all messages sent to it")
}

func TestParallelSend(t *testing.T) {
	comm1, _ := newCommInstance(5611, naiveSec)
	comm2, _ := newCommInstance(5612, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()

	messages2Send := defRecvBuffSize

	wg := sync.WaitGroup{}
	go func() {
		for i := 0; i < messages2Send; i++ {
			wg.Add(1)
			emptyMsg := createGossipMsg()
			go func() {
				defer wg.Done()
				comm1.Send(emptyMsg, &RemotePeer{Endpoint: "localhost:5612", PKIID: []byte("localhost:5612")})
			}()
		}
		wg.Wait()
	}()

	c := 0
	waiting := true
	ticker := time.NewTicker(time.Duration(5) * time.Second)
	ch := comm2.Accept(acceptAll)
	for waiting {
		select {
		case <-ch:
			c++
			if c == messages2Send {
				waiting = false
			}
			break
		case <-ticker.C:
			waiting = false
			break
		}
	}
	assert.Equal(t, messages2Send, c)
}

func TestResponses(t *testing.T) {
	comm1, _ := newCommInstance(8611, naiveSec)
	comm2, _ := newCommInstance(8612, naiveSec)

	defer comm1.Stop()
	defer comm2.Stop()

	nonceIncrememter := func(msg ReceivedMessage) ReceivedMessage {
		msg.GetGossipMessage().Nonce++
		return msg
	}

	msg := createGossipMsg()
	go func() {
		inChan := comm1.Accept(acceptAll)
		for m := range inChan {
			m = nonceIncrememter(m)
			m.Respond(m.GetGossipMessage())
		}
	}()
	expectedNOnce := uint64(msg.Nonce + 1)
	responsesFromComm1 := comm2.Accept(acceptAll)

	ticker := time.NewTicker(time.Duration(6000) * time.Millisecond)
	comm2.Send(msg, &RemotePeer{PKIID: []byte("localhost:8611"), Endpoint: "localhost:8611"})
	time.Sleep(time.Duration(100) * time.Millisecond)

	select {
	case <-ticker.C:
		assert.Fail(t, "Haven't got response from comm1 within a timely manner")
		break
	case resp := <-responsesFromComm1:
		ticker.Stop()
		assert.Equal(t, expectedNOnce, resp.GetGossipMessage().Nonce)
		break
	}
}

func TestAccept(t *testing.T) {
	comm1, _ := newCommInstance(7611, naiveSec)
	comm2, _ := newCommInstance(7612, naiveSec)

	evenNONCESelector := func(m interface{}) bool {
		return m.(ReceivedMessage).GetGossipMessage().Nonce%2 == 0
	}

	oddNONCESelector := func(m interface{}) bool {
		return m.(ReceivedMessage).GetGossipMessage().Nonce%2 != 0
	}

	evenNONCES := comm1.Accept(evenNONCESelector)
	oddNONCES := comm1.Accept(oddNONCESelector)

	var evenResults []uint64
	var oddResults []uint64

	sem := make(chan struct{}, 0)

	readIntoSlice := func(a *[]uint64, ch <-chan ReceivedMessage) {
		for m := range ch {
			*a = append(*a, m.GetGossipMessage().Nonce)
		}
		sem <- struct{}{}
	}

	go readIntoSlice(&evenResults, evenNONCES)
	go readIntoSlice(&oddResults, oddNONCES)

	for i := 0; i < defRecvBuffSize; i++ {
		comm2.Send(createGossipMsg(), &RemotePeer{Endpoint: "localhost:7611", PKIID: []byte("localhost:7611")})
	}

	time.Sleep(time.Duration(5) * time.Second)

	comm1.Stop()
	comm2.Stop()

	<-sem
	<-sem

	assert.NotEmpty(t, evenResults)
	assert.NotEmpty(t, oddResults)

	remainderPredicate := func(a []uint64, rem uint64) {
		for _, n := range a {
			assert.Equal(t, n%2, rem)
		}
	}

	remainderPredicate(evenResults, 0)
	remainderPredicate(oddResults, 1)
}

func TestReConnections(t *testing.T) {
	comm1, _ := newCommInstance(3611, naiveSec)
	comm2, _ := newCommInstance(3612, naiveSec)

	reader := func(out chan uint64, in <-chan ReceivedMessage) {
		for {
			msg := <-in
			if msg == nil {
				return
			}
			out <- msg.GetGossipMessage().Nonce
		}
	}

	out1 := make(chan uint64, 10)
	out2 := make(chan uint64, 10)

	go reader(out1, comm1.Accept(acceptAll))
	go reader(out2, comm2.Accept(acceptAll))

	// comm1 connects to comm2
	comm1.Send(createGossipMsg(), &RemotePeer{Endpoint: "localhost:3612", PKIID: []byte("localhost:3612")})
	time.Sleep(100 * time.Millisecond)
	// comm2 sends to comm1
	comm2.Send(createGossipMsg(), &RemotePeer{Endpoint: "localhost:3611", PKIID: []byte("localhost:3611")})
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, len(out2))
	assert.Equal(t, 1, len(out1))

	comm1.Stop()
	comm1, _ = newCommInstance(3611, naiveSec)
	go reader(out1, comm1.Accept(acceptAll))
	time.Sleep(300 * time.Millisecond)
	comm2.Send(createGossipMsg(), &RemotePeer{Endpoint: "localhost:3611", PKIID: []byte("localhost:3611")})
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 2, len(out1))
}

func TestProbe(t *testing.T) {
	comm1, _ := newCommInstance(6611, naiveSec)
	defer comm1.Stop()
	comm2, _ := newCommInstance(6612, naiveSec)
	time.Sleep(time.Duration(1) * time.Second)
	assert.NoError(t, comm1.Probe("localhost:6612", []byte("localhost:6612")))
	assert.Error(t, comm1.Probe("localhost:9012", []byte("localhost:9012")))
	comm2.Stop()
	time.Sleep(time.Second)
	assert.Error(t, comm1.Probe("localhost:6612", []byte("localhost:6612")))
	comm2, _ = newCommInstance(6612, naiveSec)
	defer comm2.Stop()
	time.Sleep(time.Duration(1) * time.Second)
	assert.NoError(t, comm2.Probe("localhost:6611", []byte("localhost:6611")))
	assert.NoError(t, comm1.Probe("localhost:6612", []byte("localhost:6612")))
}

func TestPresumedDead(t *testing.T) {
	comm1, _ := newCommInstance(7611, naiveSec)
	defer comm1.Stop()
	comm2, _ := newCommInstance(7612, naiveSec)
	go comm1.Send(createGossipMsg(), &RemotePeer{PKIID: []byte("localhost:7612"), Endpoint: "localhost:7612"})
	<-comm2.Accept(acceptAll)
	comm2.Stop()
	for i := 0; i < 5; i++ {
		comm1.Send(createGossipMsg(), &RemotePeer{Endpoint: "localhost:7612", PKIID: []byte("localhost:7612")})
		time.Sleep(time.Second)
	}
	ticker := time.NewTicker(time.Second * time.Duration(3))
	select {
	case <-ticker.C:
		assert.Fail(t, "Didn't get a presumed dead message within a timely manner")
		break
	case <-comm1.PresumedDead():
		ticker.Stop()
		break
	}
}

func createGossipMsg() *proto.GossipMessage {
	return &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: uint64(rand.Int()),
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{},
		},
	}
}
