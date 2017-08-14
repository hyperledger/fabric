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

package shim

import (
	"fmt"

	pb "github.com/hyperledger/fabric/protos/peer"
)

//SendPanicFailure
type SendPanicFailure string

func (e SendPanicFailure) Error() string {
	return fmt.Sprintf("send failure %s", string(e))
}

// PeerChaincodeStream interface for stream between Peer and chaincode instance.
type inProcStream struct {
	recv <-chan *pb.ChaincodeMessage
	send chan<- *pb.ChaincodeMessage
}

func newInProcStream(recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) *inProcStream {
	return &inProcStream{recv, send}
}

func (s *inProcStream) Send(msg *pb.ChaincodeMessage) (err error) {
	err = nil

	//send may happen on a closed channel when the system is
	//shutting down. Just catch the exception and return error
	defer func() {
		if r := recover(); r != nil {
			err = SendPanicFailure(fmt.Sprintf("%s", r))
			return
		}
	}()
	s.send <- msg
	return
}

func (s *inProcStream) Recv() (*pb.ChaincodeMessage, error) {
	msg := <-s.recv
	return msg, nil
}

func (s *inProcStream) CloseSend() error {
	return nil
}
