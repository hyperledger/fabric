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

	pb "github.com/hyperledger/fabric/protos/peer"
)

//MockPeerCCSupport provides CC support for peer interfaces.
type MockPeerCCSupport struct {
	ccStream map[string]*MockCCComm
}

//NewMockPeerSupport getsa mock peer support
func NewMockPeerSupport() *MockPeerCCSupport {
	return &MockPeerCCSupport{ccStream: make(map[string]*MockCCComm)}
}

//AddCC adds a cc to the MockPeerCCSupport
func (mp *MockPeerCCSupport) AddCC(name string, recv chan *pb.ChaincodeMessage, send chan *pb.ChaincodeMessage) (*MockCCComm, error) {
	if mp.ccStream[name] != nil {
		return nil, fmt.Errorf("CC %s already added", name)
	}
	mcc := &MockCCComm{name: name, recvStream: recv, sendStream: send}
	mp.ccStream[name] = mcc
	return mcc, nil
}

//GetCC gets a cc from the MockPeerCCSupport
func (mp *MockPeerCCSupport) GetCC(name string) (*MockCCComm, error) {
	s := mp.ccStream[name]
	if s == nil {
		return nil, fmt.Errorf("CC %s not added", name)
	}
	return s, nil
}

//GetCCMirror creates a MockCCStream with streans switched
func (mp *MockPeerCCSupport) GetCCMirror(name string) *MockCCComm {
	s := mp.ccStream[name]
	if s == nil {
		return nil
	}

	return &MockCCComm{name: name, recvStream: s.sendStream, sendStream: s.recvStream}
}

//RemoveCC removes a cc
func (mp *MockPeerCCSupport) RemoveCC(name string) error {
	if mp.ccStream[name] == nil {
		return fmt.Errorf("CC %s not added", name)
	}
	delete(mp.ccStream, name)
	return nil
}

//RemoveAll removes all ccs
func (mp *MockPeerCCSupport) RemoveAll() error {
	mp.ccStream = make(map[string]*MockCCComm)
	return nil
}
