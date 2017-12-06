/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("eventmgmt", "debug")
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

	handler1, handler2 := &mockHandler{}, &mockHandler{}
	eventMgr := GetMgr()
	assert.NotNil(t, eventMgr)
	eventMgr.Register("channel1", handler1)
	eventMgr.Register("channel2", handler2)

	cc2ExpectedEvent := &mockEvent{cc2Def, cc2DBArtifactsTar}
	cc3ExpectedEvent := &mockEvent{cc3Def, cc3DBArtifactsTar}

	// Deploy cc3 on chain1 - only handler1 should recieve event because cc3 is being deployed only on chain1
	eventMgr.HandleChaincodeDeploy("channel1", []*ChaincodeDefinition{cc3Def})
	assert.Contains(t, handler1.eventsRecieved, cc3ExpectedEvent)
	assert.NotContains(t, handler2.eventsRecieved, cc3ExpectedEvent)

	// Deploy cc3 on chain2 as well and this time handler2 should also recieve event
	eventMgr.HandleChaincodeDeploy("channel2", []*ChaincodeDefinition{cc3Def})
	assert.Contains(t, handler2.eventsRecieved, cc3ExpectedEvent)

	// Install CC2 - only handler1 should receive event because cc2 is deployed only on chain1 and not on chain2
	eventMgr.HandleChaincodeInstall(cc2Def, cc2DBArtifactsTar)
	assert.Contains(t, handler1.eventsRecieved, cc2ExpectedEvent)
	assert.NotContains(t, handler2.eventsRecieved, cc2ExpectedEvent)
}

func TestLSCCListener(t *testing.T) {
	channelName := "testChannel"
	cc1Def := &ChaincodeDefinition{Name: "testChaincode", Version: "v1", Hash: []byte("hash_testChaincode")}
	cc1DBArtifactsTar := []byte("cc1DBArtifacts")
	// cc1 is installed but not deployed
	mockProvider := newMockProvider()
	mockProvider.setChaincodeInstalled(cc1Def, cc1DBArtifactsTar)
	setEventMgrForTest(newMgr(mockProvider))
	defer clearEventMgrForTest()
	handler1 := &mockHandler{}
	GetMgr().Register(channelName, handler1)
	lsccStateListener := &KVLedgerLSCCStateListener{}

	sampleChaincodeData := &ccprovider.ChaincodeData{Name: cc1Def.Name, Version: cc1Def.Version, Id: cc1Def.Hash}
	sampleChaincodeDataBytes, err := proto.Marshal(sampleChaincodeData)
	assert.NoError(t, err, "")
	lsccStateListener.HandleStateUpdates(channelName, []*kvrwset.KVWrite{
		&kvrwset.KVWrite{Key: cc1Def.Name, Value: sampleChaincodeDataBytes},
	})
	assert.Contains(t, handler1.eventsRecieved, &mockEvent{cc1Def, cc1DBArtifactsTar})
}

type mockProvider struct {
	chaincodesDeployed  map[[3]string]bool
	chaincodesInstalled map[[2]string][]byte
}

type mockHandler struct {
	eventsRecieved []*mockEvent
}

type mockEvent struct {
	def            *ChaincodeDefinition
	dbArtifactsTar []byte
}

func (l *mockHandler) HandleChaincodeDeploy(chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) error {
	l.eventsRecieved = append(l.eventsRecieved, &mockEvent{def: chaincodeDefinition, dbArtifactsTar: dbArtifactsTar})
	return nil
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

func (p *mockProvider) IsChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition) (bool, error) {
	return p.chaincodesDeployed[[3]string{chainid, chaincodeDefinition.Name, chaincodeDefinition.Version}], nil
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
