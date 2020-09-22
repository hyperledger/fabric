/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistorytest

import (
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/mock"
)

type Mgr struct {
	*confighistory.Mgr
	MockCCInfoProvider *mock.DeployedChaincodeInfoProvider
}

func NewMgr(dbPath string) (*Mgr, error) {
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	configHistory, err := confighistory.NewMgr(dbPath, mockCCInfoProvider)
	if err != nil {
		return nil, err
	}
	return &Mgr{
		Mgr:                configHistory,
		MockCCInfoProvider: mockCCInfoProvider,
	}, nil
}

func (m *Mgr) Setup(ledgerID, namespace string, configHistory map[uint64][]*peer.StaticCollectionConfig) error {
	for committingBlk, config := range configHistory {
		m.MockCCInfoProvider.UpdatedChaincodesReturns(
			[]*ledger.ChaincodeLifecycleInfo{
				{
					Name: namespace,
				},
			}, nil,
		)

		m.MockCCInfoProvider.ChaincodeInfoReturns(
			&ledger.DeployedChaincodeInfo{
				Name:                        namespace,
				ExplicitCollectionConfigPkg: BuildCollConfigPkg(config),
			},
			nil,
		)

		err := m.HandleStateUpdates(
			&ledger.StateUpdateTrigger{
				LedgerID:           ledgerID,
				CommittingBlockNum: committingBlk,
			},
		)
		defer m.StateCommitDone(ledgerID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Mgr) Close() {
	m.Mgr.Close()
}

func BuildCollConfigPkg(staticCollectionConfigs []*peer.StaticCollectionConfig) *peer.CollectionConfigPackage {
	if len(staticCollectionConfigs) == 0 {
		return nil
	}
	pkg := &peer.CollectionConfigPackage{
		Config: []*peer.CollectionConfig{},
	}
	for _, c := range staticCollectionConfigs {
		pkg.Config = append(pkg.Config,
			&peer.CollectionConfig{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: c,
				},
			},
		)
	}
	return pkg
}
