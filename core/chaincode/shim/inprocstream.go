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
	pb "github.com/hyperledger/fabric/protos/peer"
)

// PeerChaincodeStream interface for stream between Peer and chaincode instance.
type inProcStream struct {
	recv <-chan *pb.ChaincodeMessage
	send chan<- *pb.ChaincodeMessage
}

func newInProcStream(recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) *inProcStream {
	return &inProcStream{recv, send}
}

func (s *inProcStream) Send(msg *pb.ChaincodeMessage) error {
	s.send <- msg
	return nil
}

func (s *inProcStream) Recv() (*pb.ChaincodeMessage, error) {
	msg := <-s.recv
	return msg, nil
}

func (s *inProcStream) CloseSend() error {
	return nil
}
