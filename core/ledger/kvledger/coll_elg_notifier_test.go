/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestCollElgNotifier(t *testing.T) {
	mockDeployedChaincodeInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockDeployedChaincodeInfoProvider.UpdatedChaincodesReturns([]*ledger.ChaincodeLifecycleInfo{
		{Name: "cc1"},
	}, nil)

	// Returns 3 collections - the bool value indicates the eligibility of peer for corresponding collection
	mockDeployedChaincodeInfoProvider.ChaincodeInfoReturnsOnCall(0,
		&ledger.DeployedChaincodeInfo{
			ExplicitCollectionConfigPkg: testutilPrepapreMockCollectionConfigPkg(
				map[string]bool{"coll1": true, "coll2": true, "coll3": false}),
		}, nil)

	// post commit - returns 4 collections
	mockDeployedChaincodeInfoProvider.ChaincodeInfoReturnsOnCall(1,
		&ledger.DeployedChaincodeInfo{
			ExplicitCollectionConfigPkg: testutilPrepapreMockCollectionConfigPkg(
				map[string]bool{"coll1": false, "coll2": true, "coll3": true, "coll4": true}),
		}, nil)

	mockMembershipInfoProvider := &mock.MembershipInfoProvider{}
	mockMembershipInfoProvider.AmMemberOfStub = func(channel string, p *peer.CollectionPolicyConfig) (bool, error) {
		return testutilIsEligibleForMockPolicy(p), nil
	}

	mockCollElgListener := &mockCollElgListener{}

	collElgNotifier := &collElgNotifier{
		mockDeployedChaincodeInfoProvider,
		mockMembershipInfoProvider,
		make(map[string]collElgListener),
	}
	collElgNotifier.registerListener("testLedger", mockCollElgListener)

	err := collElgNotifier.HandleStateUpdates(&ledger.StateUpdateTrigger{
		LedgerID:           "testLedger",
		CommittingBlockNum: uint64(500),
		StateUpdates: map[string]*ledger.KVStateUpdates{
			"doesNotMatterNS": {
				PublicUpdates: []*kvrwset.KVWrite{
					{
						Key:   "doesNotMatterKey",
						Value: []byte("doesNotMatterVal"),
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// event triggered should only contain "coll3" as this is the only collection
	// for which peer became from ineligile to eligible by upgrade tx
	require.Equal(t, uint64(500), mockCollElgListener.receivedCommittingBlk)
	require.Equal(t,
		map[string][]string{
			"cc1": {"coll3"},
		},
		mockCollElgListener.receivedNsCollMap,
	)
}

type mockCollElgListener struct {
	receivedCommittingBlk uint64
	receivedNsCollMap     map[string][]string
}

func (m *mockCollElgListener) ProcessCollsEligibilityEnabled(commitingBlk uint64, nsCollMap map[string][]string) error {
	m.receivedCommittingBlk = commitingBlk
	m.receivedNsCollMap = nsCollMap
	return nil
}

func testutilPrepapreMockCollectionConfigPkg(collEligibilityMap map[string]bool) *peer.CollectionConfigPackage {
	pkg := &peer.CollectionConfigPackage{}
	for collName, isEligible := range collEligibilityMap {
		var version int32
		if isEligible {
			version = 1
		}
		policy := &peer.CollectionPolicyConfig{
			Payload: &peer.CollectionPolicyConfig_SignaturePolicy{
				SignaturePolicy: &common.SignaturePolicyEnvelope{Version: version},
			},
		}
		sCollConfig := &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:             collName,
				MemberOrgsPolicy: policy,
			},
		}
		config := &peer.CollectionConfig{Payload: sCollConfig}
		pkg.Config = append(pkg.Config, config)
	}
	return pkg
}

func testutilIsEligibleForMockPolicy(p *peer.CollectionPolicyConfig) bool {
	return p.GetSignaturePolicy().Version == 1
}
