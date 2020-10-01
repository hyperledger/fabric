/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("eventmgmt=debug")
	os.Exit(m.Run())
}

func TestCCEventMgmt(t *testing.T) {
	cc1Def := &ChaincodeDefinition{Name: "cc1", Version: "v1", Hash: []byte("cc1")}
	cc1DBArtifactsTar := []byte("cc1DBArtifacts")

	cc2Def := &ChaincodeDefinition{Name: "cc2", Version: "v1", Hash: []byte("cc2")}
	cc2DBArtifactsTar := []byte("cc2DBArtifacts")

	cc3Def := &ChaincodeDefinition{Name: "cc3", Version: "v1", Hash: []byte("cc3")}
	cc3DBArtifactsTar := []byte("cc3DBArtifacts")

	// cc1 is deployed and installed. cc2 is deployed but not installed. cc3 is not deployed but installed
	mockProvider := newMockProvider()
	mockProvider.setChaincodeInstalled(cc1Def, cc1DBArtifactsTar)
	mockProvider.setChaincodeDeployed("channel1", cc1Def, true)
	mockProvider.setChaincodeDeployed("channel1", cc2Def, true)
	mockProvider.setChaincodeInstalled(cc3Def, cc3DBArtifactsTar)
	setEventMgrForTest(newMgr(mockProvider))
	defer clearEventMgrForTest()

	handler1, handler2, handler3 := &mockHandler{}, &mockHandler{}, &mockHandler{}
	eventMgr := GetMgr()
	require.NotNil(t, eventMgr)
	eventMgr.Register("channel1", handler1)
	eventMgr.Register("channel2", handler2)
	eventMgr.Register("channel1", handler3)
	eventMgr.Register("channel2", handler3)

	cc1ExpectedEvent := &mockEvent{cc1Def, cc1DBArtifactsTar}
	cc2ExpectedEvent := &mockEvent{cc2Def, cc2DBArtifactsTar}
	cc3ExpectedEvent := &mockEvent{cc3Def, cc3DBArtifactsTar}

	// Deploy cc3 on chain1 - handler1 and handler3 should receive event because cc3 is being deployed only on chain1
	require.NoError(t,
		eventMgr.HandleChaincodeDeploy("channel1", []*ChaincodeDefinition{cc3Def}),
	)
	eventMgr.ChaincodeDeployDone("channel1")
	require.Contains(t, handler1.eventsRecieved, cc3ExpectedEvent)
	require.NotContains(t, handler2.eventsRecieved, cc3ExpectedEvent)
	require.Contains(t, handler3.eventsRecieved, cc3ExpectedEvent)
	require.Equal(t, 1, handler1.doneRecievedCount)
	require.Equal(t, 0, handler2.doneRecievedCount)
	require.Equal(t, 1, handler3.doneRecievedCount)

	// Deploy cc3 on chain2 as well and this time handler2 should also receive event
	require.NoError(t,
		eventMgr.HandleChaincodeDeploy("channel2", []*ChaincodeDefinition{cc3Def}),
	)
	eventMgr.ChaincodeDeployDone("channel2")
	require.Contains(t, handler2.eventsRecieved, cc3ExpectedEvent)
	require.Equal(t, 1, handler1.doneRecievedCount)
	require.Equal(t, 1, handler2.doneRecievedCount)
	require.Equal(t, 2, handler3.doneRecievedCount)

	// Install CC2 - handler1 and handler 3 should receive event because cc2 is deployed only on chain1 and not on chain2
	require.NoError(t,
		eventMgr.HandleChaincodeInstall(cc2Def, cc2DBArtifactsTar),
	)
	eventMgr.ChaincodeInstallDone(true)
	require.Contains(t, handler1.eventsRecieved, cc2ExpectedEvent)
	require.NotContains(t, handler2.eventsRecieved, cc2ExpectedEvent)
	require.Contains(t, handler3.eventsRecieved, cc2ExpectedEvent)
	require.Equal(t, 2, handler1.doneRecievedCount)
	require.Equal(t, 1, handler2.doneRecievedCount)
	require.Equal(t, 3, handler3.doneRecievedCount)

	// setting cc2Def as a new lifecycle definition should cause install not to trigger event
	mockProvider.setChaincodeDeployed("channel1", cc2Def, false)
	handler1.eventsRecieved = []*mockEvent{}
	require.NoError(t,
		eventMgr.HandleChaincodeInstall(cc2Def, cc2DBArtifactsTar),
	)
	eventMgr.ChaincodeInstallDone(true)
	require.NotContains(t, handler1.eventsRecieved, cc2ExpectedEvent)

	mockListener := &mockHandler{}
	require.NoError(t,
		mgr.RegisterAndInvokeFor([]*ChaincodeDefinition{cc1Def, cc2Def, cc3Def},
			"test-ledger", mockListener,
		),
	)
	require.Contains(t, mockListener.eventsRecieved, cc1ExpectedEvent)
	require.Contains(t, mockListener.eventsRecieved, cc3ExpectedEvent)
	require.NotContains(t, mockListener.eventsRecieved, cc2ExpectedEvent)
	require.Equal(t, 2, mockListener.doneRecievedCount)
	require.Contains(t, mgr.ccLifecycleListeners["test-ledger"], mockListener)
}

func TestLSCCListener(t *testing.T) {
	channelName := "testChannel"

	cc1Def := &ChaincodeDefinition{Name: "testChaincode1", Version: "v1", Hash: []byte("hash_testChaincode")}
	cc2Def := &ChaincodeDefinition{Name: "testChaincode2", Version: "v1", Hash: []byte("hash_testChaincode")}

	ccDBArtifactsTar := []byte("ccDBArtifacts")

	// cc1, cc2 installed but not deployed
	mockProvider := newMockProvider()
	mockProvider.setChaincodeInstalled(cc1Def, ccDBArtifactsTar)
	mockProvider.setChaincodeInstalled(cc2Def, ccDBArtifactsTar)

	setEventMgrForTest(newMgr(mockProvider))
	defer clearEventMgrForTest()
	handler1 := &mockHandler{}
	GetMgr().Register(channelName, handler1)

	mockInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockInfoProvider.UpdatedChaincodesStub =
		func(map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
			return []*ledger.ChaincodeLifecycleInfo{
				{Name: cc1Def.Name},
				{Name: cc2Def.Name},
			}, nil
		}
	mockInfoProvider.ChaincodeInfoStub = func(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
		switch chaincodeName {
		case cc1Def.Name:
			return &ledger.DeployedChaincodeInfo{
				Name:     chaincodeName,
				Hash:     cc1Def.Hash,
				Version:  cc1Def.Version,
				IsLegacy: true, // event for legacy chaincode lifecycle
			}, nil
		case cc2Def.Name:
			return &ledger.DeployedChaincodeInfo{
				Name:     chaincodeName,
				Hash:     cc1Def.Hash,
				Version:  cc1Def.Version,
				IsLegacy: false, // event for new chaincode lifecycle
			}, nil
		default:
			return nil, nil
		}
	}
	lsccStateListener := &KVLedgerLSCCStateListener{mockInfoProvider}

	// test1 regular deploy lscc event gets sent to handler
	t.Run("DeployEvent", func(t *testing.T) {
		require.NoError(t,
			lsccStateListener.HandleStateUpdates(
				&ledger.StateUpdateTrigger{
					LedgerID: channelName,
				},
			),
		)
		// processes legacy event
		require.Contains(t, handler1.eventsRecieved, &mockEvent{cc1Def, ccDBArtifactsTar})
		// does not processes new lifecycle event
		require.NotContains(t, handler1.eventsRecieved, &mockEvent{cc2Def, ccDBArtifactsTar})
	})
}

type mockProvider struct {
	chaincodesDeployed             map[[3]string]bool
	chaincodesDeployedNewLifecycle map[[3]string]bool
	chaincodesInstalled            map[[2]string][]byte
}

type mockHandler struct {
	eventsRecieved    []*mockEvent
	doneRecievedCount int
}

type mockEvent struct {
	def            *ChaincodeDefinition
	dbArtifactsTar []byte
}

func (l *mockHandler) HandleChaincodeDeploy(chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) error {
	l.eventsRecieved = append(l.eventsRecieved, &mockEvent{def: chaincodeDefinition, dbArtifactsTar: dbArtifactsTar})
	return nil
}

func (l *mockHandler) ChaincodeDeployDone(succeeded bool) {
	l.doneRecievedCount++
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		make(map[[3]string]bool),
		make(map[[3]string]bool),
		make(map[[2]string][]byte),
	}
}

func (p *mockProvider) setChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition, isLegacy bool) {
	p.chaincodesDeployed[[3]string{chainid, chaincodeDefinition.Name, chaincodeDefinition.Version}] = isLegacy
}

func (p *mockProvider) setChaincodeInstalled(chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) {
	p.chaincodesInstalled[[2]string{chaincodeDefinition.Name, chaincodeDefinition.Version}] = dbArtifactsTar
}

func (p *mockProvider) GetDeployedChaincodeInfo(chainid string, chaincodeDefinition *ChaincodeDefinition) (*ledger.DeployedChaincodeInfo, error) {
	isLegacy, ok := p.chaincodesDeployed[[3]string{chainid, chaincodeDefinition.Name, chaincodeDefinition.Version}]
	if !ok {
		return nil, nil
	}
	return &ledger.DeployedChaincodeInfo{
		Name:     chaincodeDefinition.Name,
		Version:  chaincodeDefinition.Version,
		IsLegacy: isLegacy,
	}, nil
}

func (p *mockProvider) RetrieveChaincodeArtifacts(chaincodeDefinition *ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error) {
	dbArtifactsTar, ok := p.chaincodesInstalled[[2]string{chaincodeDefinition.Name, chaincodeDefinition.Version}]
	if !ok {
		return false, nil, nil
	}
	return true, dbArtifactsTar, nil
}

func setEventMgrForTest(eventMgr *Mgr) {
	mgr = eventMgr
}

func clearEventMgrForTest() {
	mgr = nil
}
