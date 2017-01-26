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

package service

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/protos/peer"
)

const testChainID = "foo"

type mockReceiver struct {
	anchorPeers []*peer.AnchorPeer
	sequence    uint64
}

func (mr *mockReceiver) configUpdated(config Config) {
	logger.Debugf("[TEST] Setting config to %d %v", config.Sequence(), config.AnchorPeers())
	mr.anchorPeers = config.AnchorPeers()
	mr.sequence = config.Sequence()
}

type mockConfig mockReceiver

func (mc *mockConfig) Sequence() uint64 {
	return mc.sequence
}

func (mc *mockConfig) AnchorPeers() []*peer.AnchorPeer {
	return mc.anchorPeers
}

func (mc *mockConfig) ChainID() string {
	return testChainID
}

func TestInitialUpdate(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		anchorPeers: []*peer.AnchorPeer{
			&peer.AnchorPeer{
				Port: 9,
			},
		},
	}

	mr := &mockReceiver{}

	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(mc)

	if !reflect.DeepEqual(mc, (*mockConfig)(mr)) {
		t.Fatalf("Should have updated config on initial update but did not")
	}
}

func TestSecondUpdate(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		anchorPeers: []*peer.AnchorPeer{
			&peer.AnchorPeer{
				Port: 9,
			},
		},
	}

	mr := &mockReceiver{}

	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(mc)

	mc.sequence = 8
	mc.anchorPeers = []*peer.AnchorPeer{
		&peer.AnchorPeer{
			Port: 10,
		},
	}

	ce.ProcessConfigUpdate(mc)

	if !reflect.DeepEqual(mc, (*mockConfig)(mr)) {
		t.Fatalf("Should have updated config on initial update but did not")
	}
}

func TestSecondSameUpdate(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		anchorPeers: []*peer.AnchorPeer{
			&peer.AnchorPeer{
				Port: 9,
			},
		},
	}

	mr := &mockReceiver{}

	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(mc)
	mr.sequence = 0
	mr.anchorPeers = nil
	ce.ProcessConfigUpdate(mc)

	if mr.sequence != 0 {
		t.Errorf("Should not have updated sequence when reprocessing same config")
	}

	if mr.anchorPeers != nil {
		t.Errorf("Should not have updated anchor peers when reprocessing same config")
	}
}

func TestUpdatedSeqOnly(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		anchorPeers: []*peer.AnchorPeer{
			&peer.AnchorPeer{
				Port: 9,
			},
		},
	}

	mr := &mockReceiver{}

	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(mc)
	mc.sequence = 9
	ce.ProcessConfigUpdate(mc)

	if mr.sequence != 7 {
		t.Errorf("Should not have updated sequence when reprocessing same config")
	}

	if !reflect.DeepEqual(mr.anchorPeers, mc.anchorPeers) {
		t.Errorf("Should not have cleared anchor peers when reprocessing newer config with higher sequence")
	}
}
