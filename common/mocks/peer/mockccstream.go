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

package peer

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/hyperledger/fabric/protos/peer"
)

//MockResponseSet is used for processing CC to Peer comm
//such as GET/PUT/DEL state. The MockResponse contains the
//response to be returned for each input received.from the
//CC. Every stub call will generate a response
type MockResponseSet struct {
	//DoneFunc is invoked when all I/O is done for this
	//response set
	DoneFunc func(int, error)

	//ErrorFunc is invoked at any step when the input does not
	//match the received message
	ErrorFunc func(int, error)

	//Responses contained the expected received message (optional)
	//and response to send (optional)
	Responses []*MockResponse
}

//MockResponse contains the expected received message (optional)
//and response to send (optional)
type MockResponse struct {
	RecvMsg *pb.ChaincodeMessage
	RespMsg interface{}
}

// MockCCComm implements the mock communication between chaincode and peer
// We'd need two MockCCComm for communication. The receiver and sender will
// be switched between the two.
type MockCCComm struct {
	name        string
	bailOnError bool
	keepAlive   *pb.ChaincodeMessage
	recvStream  chan *pb.ChaincodeMessage
	sendStream  chan *pb.ChaincodeMessage
	respIndex   int
	respLock    sync.Mutex
	respSet     *MockResponseSet
	pong        bool
	skipClose   bool
}

func (s *MockCCComm) SetName(newname string) {
	s.name = newname
}

//Send sends a message
func (s *MockCCComm) Send(msg *pb.ChaincodeMessage) error {
	s.sendStream <- msg
	return nil
}

//Recv receives a message
func (s *MockCCComm) Recv() (*pb.ChaincodeMessage, error) {
	msg := <-s.recvStream
	return msg, nil
}

//CloseSend closes send
func (s *MockCCComm) CloseSend() error {
	return nil
}

//GetRecvStream returns the recvStream
func (s *MockCCComm) GetRecvStream() chan *pb.ChaincodeMessage {
	return s.recvStream
}

//GetSendStream returns the sendStream
func (s *MockCCComm) GetSendStream() chan *pb.ChaincodeMessage {
	return s.sendStream
}

//Quit closes the channels...
func (s *MockCCComm) Quit() {
	if !s.skipClose {
		close(s.recvStream)
		close(s.sendStream)
	}
}

//SetBailOnError will cause Run to return on any error
func (s *MockCCComm) SetBailOnError(b bool) {
	s.bailOnError = b
}

//SetPong pongs received keepalive. This mut be done on the chaincode only
func (s *MockCCComm) SetPong(val bool) {
	s.pong = val
}

//SetKeepAlive sets keepalive. This mut be done on the server only
func (s *MockCCComm) SetKeepAlive(ka *pb.ChaincodeMessage) {
	s.keepAlive = ka
}

//SetResponses sets responses for an Init or Invoke
func (s *MockCCComm) SetResponses(respSet *MockResponseSet) {
	s.respLock.Lock()
	s.respSet = respSet
	s.respIndex = 0
	s.respLock.Unlock()
}

//keepAlive
func (s *MockCCComm) ka(done <-chan struct{}) {
	for {
		if s.keepAlive == nil {
			return
		}
		s.Send(s.keepAlive)
		select {
		case <-time.After(10 * time.Millisecond):
		case <-done:
			return
		}
	}
}

//Run receives and sends indefinitely
func (s *MockCCComm) Run(done <-chan struct{}) error {
	//start the keepalive
	go s.ka(done)
	defer s.Quit()

	for {
		msg, err := s.Recv()

		//stream could just be closed
		if msg == nil {
			return err
		}

		if err != nil {
			return err
		}

		if err = s.respond(msg); err != nil {
			if s.bailOnError {
				return err
			}
		}
	}
}

func (s *MockCCComm) respond(msg *pb.ChaincodeMessage) error {
	if msg != nil && msg.Type == pb.ChaincodeMessage_KEEPALIVE {
		//if ping should be ponged, pong
		if s.pong {
			return s.Send(msg)
		}
		return nil
	}

	s.respLock.Lock()
	defer s.respLock.Unlock()

	var err error
	if s.respIndex < len(s.respSet.Responses) {
		mockResp := s.respSet.Responses[s.respIndex]
		if mockResp.RecvMsg != nil {
			if msg.Type != mockResp.RecvMsg.Type {
				if s.respSet.ErrorFunc != nil {
					s.respSet.ErrorFunc(s.respIndex, fmt.Errorf("Invalid message expected %d received %d", int32(mockResp.RecvMsg.Type), int32(msg.Type)))
					s.respIndex = s.respIndex + 1
					return nil
				}
			}
		}

		if mockResp.RespMsg != nil {
			var ccMsg *pb.ChaincodeMessage
			if ccMsg, _ = mockResp.RespMsg.(*pb.ChaincodeMessage); ccMsg == nil {
				if ccMsgFunc, ok := mockResp.RespMsg.(func(*pb.ChaincodeMessage) *pb.ChaincodeMessage); ok && ccMsgFunc != nil {
					ccMsg = ccMsgFunc(msg)
				}
			}

			if ccMsg == nil {
				panic("----no pb.ChaincodeMessage---")
			}
			err = s.Send(ccMsg)
		}

		s.respIndex = s.respIndex + 1

		if s.respIndex == len(s.respSet.Responses) {
			if s.respSet.DoneFunc != nil {
				s.respSet.DoneFunc(s.respIndex, nil)
			}
		}
	}
	return err
}
