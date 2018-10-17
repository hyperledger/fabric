/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
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
	mockProvider.setChaincodeDeployed("channel1", cc1Def)
	mockProvider.setChaincodeDeployed("channel1", cc2Def)
	mockProvider.setChaincodeInstalled(cc3Def, cc3DBArtifactsTar)
	setEventMgrForTest(newMgr(mockProvider))
	defer clearEventMgrForTest()

	handler1, handler2, handler3 := &mockHandler{}, &mockHandler{}, &mockHandler{}
	eventMgr := GetMgr()
	assert.NotNil(t, eventMgr)
	eventMgr.Register("channel1", handler1)
	eventMgr.Register("channel2", handler2)
	eventMgr.Register("channel1", handler3)
	eventMgr.Register("channel2", handler3)

	cc2ExpectedEvent := &mockEvent{cc2Def, cc2DBArtifactsTar}
	_ = cc2ExpectedEvent
	cc3ExpectedEvent := &mockEvent{cc3Def, cc3DBArtifactsTar}

	// Deploy cc3 on chain1 - handler1 and handler3 should recieve event because cc3 is being deployed only on chain1
	eventMgr.HandleChaincodeDeploy("channel1", []*ChaincodeDefinition{cc3Def})
	eventMgr.ChaincodeDeployDone("channel1")
	assert.Contains(t, handler1.eventsRecieved, cc3ExpectedEvent)
	assert.NotContains(t, handler2.eventsRecieved, cc3ExpectedEvent)
	assert.Contains(t, handler3.eventsRecieved, cc3ExpectedEvent)
	assert.Equal(t, 1, handler1.doneRecievedCount)
	assert.Equal(t, 0, handler2.doneRecievedCount)
	assert.Equal(t, 1, handler3.doneRecievedCount)

	// Deploy cc3 on chain2 as well and this time handler2 should also recieve event
	eventMgr.HandleChaincodeDeploy("channel2", []*ChaincodeDefinition{cc3Def})
	eventMgr.ChaincodeDeployDone("channel2")
	assert.Contains(t, handler2.eventsRecieved, cc3ExpectedEvent)
	assert.Equal(t, 1, handler1.doneRecievedCount)
	assert.Equal(t, 1, handler2.doneRecievedCount)
	assert.Equal(t, 2, handler3.doneRecievedCount)

	// Install CC2 - handler1 and handler 3 should receive event because cc2 is deployed only on chain1 and not on chain2
	eventMgr.HandleChaincodeInstall(cc2Def, cc2DBArtifactsTar)
	eventMgr.ChaincodeInstallDone(true)
	assert.Contains(t, handler1.eventsRecieved, cc2ExpectedEvent)
	assert.NotContains(t, handler2.eventsRecieved, cc2ExpectedEvent)
	assert.Contains(t, handler3.eventsRecieved, cc2ExpectedEvent)
	assert.Equal(t, 2, handler1.doneRecievedCount)
	assert.Equal(t, 1, handler2.doneRecievedCount)
	assert.Equal(t, 3, handler3.doneRecievedCount)
}

func TestLSCCListener(t *testing.T) {
	channelName := "testChannel"

	cc1Def := &ChaincodeDefinition{Name: "testChaincode1", Version: "v1", Hash: []byte("hash_testChaincode")}
	cc2Def := &ChaincodeDefinition{Name: "testChaincode2", Version: "v1", Hash: []byte("hash_testChaincode")}
	cc3Def := &ChaincodeDefinition{Name: "testChaincode~collection", Version: "v1", Hash: []byte("hash_testChaincode")}

	ccDBArtifactsTar := []byte("ccDBArtifacts")

	// cc1, cc2, cc3 installed but not deployed
	mockProvider := newMockProvider()
	mockProvider.setChaincodeInstalled(cc1Def, ccDBArtifactsTar)
	mockProvider.setChaincodeInstalled(cc2Def, ccDBArtifactsTar)
	mockProvider.setChaincodeInstalled(cc3Def, ccDBArtifactsTar)

	setEventMgrForTest(newMgr(mockProvider))
	defer clearEventMgrForTest()
	handler1 := &mockHandler{}
	GetMgr().Register(channelName, handler1)

	mockInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockInfoProvider.UpdatedChaincodesStub =
		func(map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
			return []*ledger.ChaincodeLifecycleInfo{
				{Name: cc1Def.Name},
			}, nil
		}
	mockInfoProvider.ChaincodeInfoStub = func(chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
		return &ledger.DeployedChaincodeInfo{
			Name:    chaincodeName,
			Hash:    cc1Def.Hash,
			Version: cc1Def.Version,
		}, nil
	}
	lsccStateListener := &KVLedgerLSCCStateListener{mockInfoProvider}

	// test1 regular deploy lscc event gets sent to handler
	t.Run("DeployEvent", func(t *testing.T) {
		lsccStateListener.HandleStateUpdates(&ledger.StateUpdateTrigger{
			LedgerID:           channelName,
			CommittingBlockNum: 50},
		)
		assert.Contains(t, handler1.eventsRecieved, &mockEvent{cc1Def, ccDBArtifactsTar})
	})

	// test2 delete lscc event NOT sent to handler
	t.Run("DeleteEvent", func(t *testing.T) {
		lsccStateListener.HandleStateUpdates(&ledger.StateUpdateTrigger{
			LedgerID:           channelName,
			CommittingBlockNum: 50},
		)
		assert.NotContains(t, handler1.eventsRecieved, &mockEvent{cc2Def, ccDBArtifactsTar})
	})

	// test3 collection lscc event (with tilda separator in chaincode key) NOT sent to handler
	t.Run("CollectionEvent", func(t *testing.T) {
		lsccStateListener.HandleStateUpdates(&ledger.StateUpdateTrigger{
			LedgerID:           channelName,
			CommittingBlockNum: 50},
		)
		assert.NotContains(t, handler1.eventsRecieved, &mockEvent{cc3Def, ccDBArtifactsTar})
	})
}

type mockProvider struct {
	chaincodesDeployed  map[[3]string]bool
	chaincodesInstalled map[[2]string][]byte
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
		make(map[[2]string][]byte),
	}
}

func (p *mockProvider) setChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition) {
	p.chaincodesDeployed[[3]string{chainid, chaincodeDefinition.Name, chaincodeDefinition.Version}] = true
}

func (p *mockProvider) setChaincodeInstalled(chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) {
	p.chaincodesInstalled[[2]string{chaincodeDefinition.Name, chaincodeDefinition.Version}] = dbArtifactsTar
}

func (p *mockProvider) setChaincodeDeployAndInstalled(chainid string, chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) {
	p.setChaincodeDeployed(chainid, chaincodeDefinition)
	p.setChaincodeInstalled(chaincodeDefinition, dbArtifactsTar)
}

func (p *mockProvider) GetDeployedChaincodeInfo(chainid string, chaincodeDefinition *ChaincodeDefinition) (*ledger.DeployedChaincodeInfo, error) {
	if p.chaincodesDeployed[[3]string{chainid, chaincodeDefinition.Name, chaincodeDefinition.Version}] {
		return &ledger.DeployedChaincodeInfo{
			Name:    chaincodeDefinition.Name,
			Version: chaincodeDefinition.Version,
		}, nil
	}
	return nil, nil
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
