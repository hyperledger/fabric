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
	"crypto/tls"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/protos/gossip"
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

func (*naiveSecProvider) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveSecProvider) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveSecProvider) VerifyBlock(chainID common.ChainID, signedBlock api.SignedBlock) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*naiveSecProvider) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*naiveSecProvider) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	equal := bytes.Equal(signature, message)
	if !equal {
		return fmt.Errorf("Wrong certificate:%v, %v", signature, message)
	}
	return nil
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (*naiveSecProvider) VerifyByChannel(_ common.ChainID, _ api.PeerIdentityType, _, _ []byte) error {
	return nil
}

func newCommInstance(port int, sec api.MessageCryptoService) (Comm, error) {
	endpoint := fmt.Sprintf("localhost:%d", port)
	inst, err := NewCommInstanceWithServer(port, identity.NewIdentityMapper(sec), []byte(endpoint))
	return inst, err
}

func handshaker(endpoint string, comm Comm, t *testing.T, sigMutator func([]byte) []byte, pkiIDmutator func([]byte) []byte) <-chan ReceivedMessage {
	c := &commImpl{}
	err := generateCertificates("key.pem", "cert.pem")
	defer os.Remove("cert.pem")
	defer os.Remove("key.pem")
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	ta := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
	})
	acceptChan := comm.Accept(acceptAll)
	conn, err := grpc.Dial("localhost:9611", grpc.WithTransportCredentials(&authCreds{tlsCreds: ta}), grpc.WithBlock(), grpc.WithTimeout(time.Second))
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return nil
	}
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return nil
	}
	clientCertHash := certHashFromRawCert(cert.Certificate[0])

	pkiID := common.PKIidType(endpoint)
	if pkiIDmutator != nil {
		pkiID = common.PKIidType(pkiIDmutator([]byte(endpoint)))
	}
	assert.NoError(t, err, "%v", err)
	msg := c.createConnectionMsg(pkiID, clientCertHash, []byte(endpoint), func(msg []byte) ([]byte, error) {
		return msg, nil
	})

	if sigMutator != nil {
		msg.Signature = sigMutator(msg.Signature)
	}

	stream.Send(msg)
	msg, err = stream.Recv()
	assert.NoError(t, err, "%v", err)
	if sigMutator == nil {
		hash := extractCertificateHashFromContext(stream.Context())
		expectedMsg := c.createConnectionMsg(common.PKIidType("localhost:9611"), hash, []byte("localhost:9611"), func(msg []byte) ([]byte, error) {
			return msg, nil
		})
		assert.Equal(t, expectedMsg.Signature, msg.Signature)
	}
	assert.Equal(t, []byte("localhost:9611"), msg.GetConn().PkiID)
	msg2Send := createGossipMsg()
	nonce := uint64(rand.Int())
	msg2Send.Nonce = nonce
	go stream.Send(msg2Send)
	return acceptChan
}

func TestHandshake(t *testing.T) {
	t.Parallel()
	comm, _ := newCommInstance(9611, naiveSec)
	defer comm.Stop()

	acceptChan := handshaker("localhost:9610", comm, t, nil, nil)
	time.Sleep(2 * time.Second)
	assert.Equal(t, 1, len(acceptChan))

	// negative path, nothing should be read from the channel because the signature is wrong
	mutateSig := func(b []byte) []byte {
		if b[0] == 0 {
			b[0] = 1
		} else {
			b[0] = 0
		}
		return b
	}
	acceptChan = handshaker("localhost:9612", comm, t, mutateSig, nil)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))

	// negative path, nothing should be read from the channel because the PKIid doesn't match the identity
	mutatePKIID := func(b []byte) []byte {
		return []byte("localhost:9650")
	}
	acceptChan = handshaker("localhost:9613", comm, t, nil, mutatePKIID)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))
}

func TestBasic(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(2000, naiveSec)
	comm2, _ := newCommInstance(3000, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	m1 := comm1.Accept(acceptAll)
	m2 := comm2.Accept(acceptAll)
	out := make(chan uint64, 2)
	reader := func(ch <-chan ReceivedMessage) {
		m := <-ch
		out <- m.GetGossipMessage().Nonce
	}
	go reader(m1)
	go reader(m2)
	comm1.Send(createGossipMsg(), remotePeer(3000))
	time.Sleep(time.Second)
	comm2.Send(createGossipMsg(), remotePeer(2000))
	waitForMessages(t, out, 2, "Didn't receive 2 messages")
}

func TestGetPKIID(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(6000, naiveSec)
	comm2, _ := newCommInstance(7000, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	m1 := comm1.Accept(acceptAll)
	comm2.Send(createGossipMsg(), remotePeer(6000))
	select {
	case <-time.After(time.Second * 10):
		t.Fatal("Didn't receive a message in time")
	case msg := <-m1:
		assert.Equal(t, comm2.GetPKIid(), msg.GetPKIID())
	}
}

func TestBlackListPKIid(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(1611, naiveSec)
	comm2, _ := newCommInstance(1612, naiveSec)
	comm3, _ := newCommInstance(1613, naiveSec)
	comm4, _ := newCommInstance(1614, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	defer comm3.Stop()
	defer comm4.Stop()

	reader := func(instance string, out chan uint64, in <-chan ReceivedMessage) {
		for {
			msg := <-in
			if msg == nil {
				return
			}
			out <- msg.GetGossipMessage().Nonce
		}
	}

	out1 := make(chan uint64, 4)
	out2 := make(chan uint64, 4)
	out3 := make(chan uint64, 4)
	out4 := make(chan uint64, 4)

	go reader("comm1", out1, comm1.Accept(acceptAll))
	go reader("comm2", out2, comm2.Accept(acceptAll))
	go reader("comm3", out3, comm3.Accept(acceptAll))
	go reader("comm4", out4, comm4.Accept(acceptAll))

	// have comm1 BL comm3
	comm1.BlackListPKIid([]byte("localhost:1613"))

	// make comm3 send to 1 and 2
	comm3.Send(createGossipMsg(), remotePeer(1612)) // out2++
	comm3.Send(createGossipMsg(), remotePeer(1611))

	waitForMessages(t, out2, 1, "comm2 should have received 1 message")

	// make comm1 and comm2 send to comm3
	comm1.Send(createGossipMsg(), remotePeer(1613))
	comm2.Send(createGossipMsg(), remotePeer(1613)) // out3++
	waitForMessages(t, out3, 1, "comm3 should have received 1 message")

	// make comm1 and comm2 send to comm4 which is not blacklisted		// out4 += 4
	comm1.Send(createGossipMsg(), remotePeer(1614))
	comm2.Send(createGossipMsg(), remotePeer(1614))
	comm1.Send(createGossipMsg(), remotePeer(1614))
	comm2.Send(createGossipMsg(), remotePeer(1614))

	// blacklist comm3 by comm2
	comm2.BlackListPKIid([]byte("localhost:1613"))

	// send from comm1 and comm2 to comm3 again
	comm1.Send(createGossipMsg(), remotePeer(1613)) // shouldn't have an effect
	comm2.Send(createGossipMsg(), remotePeer(1613)) // shouldn't have an effect

	waitForMessages(t, out4, 4, "comm1 should have received 4 messages")
}

func TestParallelSend(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(5411, naiveSec)
	comm2, _ := newCommInstance(5412, naiveSec)
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
				comm1.Send(emptyMsg, remotePeer(5412))
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
	t.Parallel()
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
	comm2.Send(msg, remotePeer(8611))
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
	t.Parallel()
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

	out := make(chan uint64, defRecvBuffSize)
	sem := make(chan struct{}, 0)

	readIntoSlice := func(a *[]uint64, ch <-chan ReceivedMessage) {
		for m := range ch {
			*a = append(*a, m.GetGossipMessage().Nonce)
			out <- m.GetGossipMessage().Nonce
		}
		sem <- struct{}{}
	}

	go readIntoSlice(&evenResults, evenNONCES)
	go readIntoSlice(&oddResults, oddNONCES)

	for i := 0; i < defRecvBuffSize; i++ {
		comm2.Send(createGossipMsg(), remotePeer(7611))
	}

	waitForMessages(t, out, defRecvBuffSize, "Didn't receive all messages sent")

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
	t.Parallel()
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
	comm1.Send(createGossipMsg(), remotePeer(3612))
	waitForMessages(t, out2, 1, "Comm2 didn't receive a message from comm1 in a timely manner")
	time.Sleep(time.Second)
	// comm2 sends to comm1
	comm2.Send(createGossipMsg(), remotePeer(3611))
	waitForMessages(t, out1, 1, "Comm1 didn't receive a message from comm2 in a timely manner")

	comm1.Stop()
	comm1, _ = newCommInstance(3611, naiveSec)
	time.Sleep(time.Second)
	out1 = make(chan uint64, 1)
	go reader(out1, comm1.Accept(acceptAll))
	comm2.Send(createGossipMsg(), remotePeer(3611))
	waitForMessages(t, out1, 1, "Comm1 didn't receive a message from comm2 in a timely manner")
}

func TestProbe(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(6611, naiveSec)
	defer comm1.Stop()
	comm2, _ := newCommInstance(6612, naiveSec)
	time.Sleep(time.Duration(1) * time.Second)
	assert.NoError(t, comm1.Probe(remotePeer(6612)))
	assert.Error(t, comm1.Probe(remotePeer(9012)))
	comm2.Stop()
	time.Sleep(time.Second)
	assert.Error(t, comm1.Probe(remotePeer(6612)))
	comm2, _ = newCommInstance(6612, naiveSec)
	defer comm2.Stop()
	time.Sleep(time.Duration(1) * time.Second)
	assert.NoError(t, comm2.Probe(remotePeer(6611)))
	assert.NoError(t, comm1.Probe(remotePeer(6612)))
}

func TestPresumedDead(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(4611, naiveSec)
	comm2, _ := newCommInstance(4612, naiveSec)
	go comm1.Send(createGossipMsg(), remotePeer(4612))
	<-comm2.Accept(acceptAll)
	comm2.Stop()
	go func() {
		for i := 0; i < 5; i++ {
			comm1.Send(createGossipMsg(), remotePeer(4612))
			time.Sleep(time.Millisecond * 200)
		}
	}()

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

func remotePeer(port int) *RemotePeer {
	endpoint := fmt.Sprintf("localhost:%d", port)
	return &RemotePeer{Endpoint: endpoint, PKIID: []byte(endpoint)}
}

func waitForMessages(t *testing.T, msgChan chan uint64, count int, errMsg string) {
	c := 0
	waiting := true
	ticker := time.NewTicker(time.Duration(10) * time.Second)
	for waiting {
		select {
		case <-msgChan:
			c++
			if c == count {
				waiting = false
			}
			break
		case <-ticker.C:
			waiting = false
			break
		}
	}
	assert.Equal(t, count, c, errMsg)
}
