/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"bytes"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("cceventmgmt")

var mgr *Mgr

// Initialize initializes event mgmt
func Initialize() {
	initialize(&chaincodeInfoProviderImpl{})
}

func initialize(ccInfoProvider ChaincodeInfoProvider) {
	mgr = newMgr(ccInfoProvider)
}

// GetMgr returns the reference to singleton event manager
func GetMgr() *Mgr {
	return mgr
}

// Mgr encapsulate important interactions for events related to the interest of ledger
type Mgr struct {
	// rwlock is mainly used to synchronize across deploy transaction, chaincode install, and channel creation
	// Ideally, different services in the peer should be designed such that they expose locks for different important
	// events so that a code on top can synchronize across if needs to. However, in the lack of any such system-wide design,
	// we use this lock for contextual use
	rwlock               sync.RWMutex
	infoProvider         ChaincodeInfoProvider
	ccLifecycleListeners map[string]ChaincodeLifecycleEventListener
	// latestChaincodeDeploys maintains last chaincode deployed for a ledger. As stated in the above comment,
	// since it is not easy to synchronize across block commit and install activity, this leaves a small window
	// where we could miss 'deployed AND installed' state. So, we explicitly maintain the chaincodes deplyed
	// in the last block
	latestChaincodeDeploys map[string][]*ChaincodeDefinition
}

func newMgr(chaincodeInfoProvider ChaincodeInfoProvider) *Mgr {
	return &Mgr{
		infoProvider:           chaincodeInfoProvider,
		ccLifecycleListeners:   make(map[string]ChaincodeLifecycleEventListener),
		latestChaincodeDeploys: make(map[string][]*ChaincodeDefinition)}
}

// Register registers a ChaincodeLifecycleEventListener for given ledgerid
// Since, `Register` is expected to be invoked when creating/opening a ledger instance
func (m *Mgr) Register(ledgerid string, l ChaincodeLifecycleEventListener) {
	// write lock to synchronize concurrent 'chaincode install' operations with ledger creation/open
	m.rwlock.Lock()
	defer m.rwlock.Unlock()
	m.ccLifecycleListeners[ledgerid] = l
}

// HandleChaincodeDeploy is expected to be invoked when a chaincode is deployed via a deploy transaction
// The `chaincodeDefinitions` parameter contains all the chaincodes deployed in a block
// We need to store the last received `chaincodeDefinitions` because this function is expected to be invoked
// after the deploy transactions validation is performed but not committed yet to the ledger. Further, we
// release the read lock after this function. This leaves a small window when a `chaincode install` can happen
// before the deploy transaction is committed and hence the function `HandleChaincodeInstall` may miss finding
// the deployed chaincode. So, in function `HandleChaincodeInstall`, we explicitly check for chaincode deployed
// in this stored `chaincodeDefinitions`
func (m *Mgr) HandleChaincodeDeploy(chainid string, chaincodeDefinitions []*ChaincodeDefinition) error {
	logger.Debugf("Channel [%s]: Handling chaincode deploy event for chaincode [%s]", chainid, chaincodeDefinitions)
	// Read lock to allow concurrent deploy on multiple channels but to synchronize concurrent `chaincode insall` operation
	m.rwlock.RLock()
	defer m.rwlock.RUnlock()
	// TODO, device a mechanism to cleanup entries in this map
	m.latestChaincodeDeploys[chainid] = chaincodeDefinitions
	for _, chaincodeDefinition := range chaincodeDefinitions {
		installed, dbArtifacts, err := m.infoProvider.RetrieveChaincodeArtifacts(chaincodeDefinition)
		if err != nil {
			return err
		}
		if !installed {
			logger.Infof("Channel [%s]: Chaincode [%s] is not installed hence no need to create chaincode artifacts for endorsement",
				chainid, chaincodeDefinition)
			continue
		}
		if err := m.invokeHandler(chainid, chaincodeDefinition, dbArtifacts); err != nil {
			logger.Warningf("Channel [%s]: Error while invoking a listener for handling chaincode install event: %s", chainid, err)
			return err
		}
		logger.Debugf("Channel [%s]: Handled chaincode deploy event for chaincode [%s]", chainid, chaincodeDefinitions)
	}
	return nil
}

// HandleChaincodeInstall is expected to gets invoked when a during installation of a chaincode package
func (m *Mgr) HandleChaincodeInstall(chaincodeDefinition *ChaincodeDefinition, dbArtifacts []byte) error {
	logger.Debugf("HandleChaincodeInstall() - chaincodeDefinition=%#v", chaincodeDefinition)
	// Write lock prevents concurrent deploy operations
	m.rwlock.Lock()
	defer m.rwlock.Unlock()
	for chainid := range m.ccLifecycleListeners {
		logger.Debugf("Channel [%s]: Handling chaincode install event for chaincode [%s]", chainid, chaincodeDefinition)
		var deployed bool
		var err error
		deployed = m.isChaincodePresentInLatestDeploys(chainid, chaincodeDefinition)
		if !deployed {
			if deployed, err = m.infoProvider.IsChaincodeDeployed(chainid, chaincodeDefinition); err != nil {
				logger.Warningf("Channel [%s]: Error while getting the deployment status of chaincode: %s", chainid, err)
				return err
			}
		}
		if !deployed {
			logger.Debugf("Channel [%s]: Chaincode [%s] is not deployed on channel hence not creating chaincode artifacts.",
				chainid, chaincodeDefinition)
			continue
		}
		if err := m.invokeHandler(chainid, chaincodeDefinition, dbArtifacts); err != nil {
			logger.Warningf("Channel [%s]: Error while invoking a listener for handling chaincode install event: %s", chainid, err)
			return err
		}
		logger.Debugf("Channel [%s]: Handled chaincode install event for chaincode [%s]", chainid, chaincodeDefinition)
	}
	return nil
}

func (m *Mgr) isChaincodePresentInLatestDeploys(chainid string, chaincodeDefinition *ChaincodeDefinition) bool {
	ccDefs, ok := m.latestChaincodeDeploys[chainid]
	if !ok {
		return false
	}
	for _, ccDef := range ccDefs {
		if ccDef.Name == chaincodeDefinition.Name && ccDef.Version == chaincodeDefinition.Version && bytes.Equal(ccDef.Hash, chaincodeDefinition.Hash) {
			return true
		}
	}
	return false
}

func (m *Mgr) invokeHandler(chainid string, chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) error {
	listener := m.ccLifecycleListeners[chainid]
	if listener == nil {
		return nil
	}
	return listener.HandleChaincodeDeploy(chaincodeDefinition, dbArtifactsTar)
}
