/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package service

import (
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	deliverclient "github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/peer"
)

const testChainID = "foo"

func init() {
	util.SetupTestLogging()
}

type mockReceiver struct {
	ordererEndpointsByOrg map[string][]string
	appOrgs               map[string]channelconfig.ApplicationOrg
	sequence              uint64
}

func (mr *mockReceiver) updateAnchors(config Config) {
	logger.Debugf("[TEST] Setting config to %d %v", config.Sequence(), config.ApplicationOrgs())
	mr.appOrgs = config.ApplicationOrgs()
	mr.sequence = config.Sequence()
}

func (mr *mockReceiver) updateEndpoints(chainID string, _ deliverclient.ConnectionCriteria) {
}

type mockConfig mockReceiver

func (mc *mockConfig) OrdererAddressesByOrgs() map[string][]string {
	return mc.ordererEndpointsByOrg
}

func (mc *mockConfig) OrdererOrgs() []string {
	return nil
}

func (mc *mockConfig) ApplicationOrgs() ApplicationOrgs {
	return mc.appOrgs
}

func (mc *mockConfig) OrdererAddresses() []string {
	return []string{"localhost:7050"}
}

func (mc *mockConfig) Sequence() uint64 {
	return mc.sequence
}

func (mc *mockConfig) ChainID() string {
	return testChainID
}

const testOrgID = "testID"

func TestInitialUpdate(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		appOrgs: map[string]channelconfig.ApplicationOrg{
			testOrgID: &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
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
	appGrps := map[string]channelconfig.ApplicationOrg{
		testOrgID: &appGrp{
			anchorPeers: []*peer.AnchorPeer{{Port: 9}},
		},
	}
	mc := &mockConfig{
		sequence: 7,
		appOrgs:  appGrps,
	}

	mr := &mockReceiver{}

	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(mc)

	mc.sequence = 8
	appGrps[testOrgID] = &appGrp{
		anchorPeers: []*peer.AnchorPeer{{Port: 10}},
	}

	ce.ProcessConfigUpdate(mc)

	if !reflect.DeepEqual(mc, (*mockConfig)(mr)) {
		t.Fatal("Should have updated config on initial update but did not")
	}
}

func TestSecondSameUpdate(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		appOrgs: map[string]channelconfig.ApplicationOrg{
			testOrgID: &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
			},
		},
	}

	mr := &mockReceiver{}

	ce := newConfigEventer(mr)
	ce.ProcessConfigUpdate(mc)
	mr.sequence = 0
	mr.appOrgs = nil
	ce.ProcessConfigUpdate(mc)

	if mr.sequence != 0 {
		t.Error("Should not have updated sequence when reprocessing same config")
	}

	if mr.appOrgs != nil {
		t.Error("Should not have updated anchor peers when reprocessing same config")
	}
}

func TestUpdatedSeqOnly(t *testing.T) {
	mc := &mockConfig{
		sequence: 7,
		appOrgs: map[string]channelconfig.ApplicationOrg{
			testOrgID: &appGrp{
				anchorPeers: []*peer.AnchorPeer{{Port: 9}},
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

	if !reflect.DeepEqual(mr.appOrgs, mc.appOrgs) {
		t.Errorf("Should not have cleared anchor peers when reprocessing newer config with higher sequence")
	}
}
