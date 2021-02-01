/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"fmt"

	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// MockPeerCCSupport provides CC support for peer interfaces.
type MockPeerCCSupport struct {
	ccStream map[string]*MockCCComm
}

// NewMockPeerSupport getsa mock peer support
func NewMockPeerSupport() *MockPeerCCSupport {
	return &MockPeerCCSupport{ccStream: make(map[string]*MockCCComm)}
}

// AddCC adds a cc to the MockPeerCCSupport
func (mp *MockPeerCCSupport) AddCC(name string, recv chan *pb.ChaincodeMessage, send chan *pb.ChaincodeMessage) (*MockCCComm, error) {
	if mp.ccStream[name] != nil {
		return nil, fmt.Errorf("CC %s already added", name)
	}
	mcc := &MockCCComm{name: name, recvStream: recv, sendStream: send}
	mp.ccStream[name] = mcc
	return mcc, nil
}

// GetCC gets a cc from the MockPeerCCSupport
func (mp *MockPeerCCSupport) GetCC(name string) (*MockCCComm, error) {
	s := mp.ccStream[name]
	if s == nil {
		return nil, fmt.Errorf("CC %s not added", name)
	}
	return s, nil
}

// GetCCMirror creates a MockCCStream with streans switched
func (mp *MockPeerCCSupport) GetCCMirror(name string) *MockCCComm {
	s := mp.ccStream[name]
	if s == nil {
		return nil
	}

	return &MockCCComm{name: name, recvStream: s.sendStream, sendStream: s.recvStream, skipClose: true}
}

// RemoveCC removes a cc
func (mp *MockPeerCCSupport) RemoveCC(name string) error {
	if mp.ccStream[name] == nil {
		return fmt.Errorf("CC %s not added", name)
	}
	delete(mp.ccStream, name)
	return nil
}

// RemoveAll removes all ccs
func (mp *MockPeerCCSupport) RemoveAll() error {
	mp.ccStream = make(map[string]*MockCCComm)
	return nil
}
