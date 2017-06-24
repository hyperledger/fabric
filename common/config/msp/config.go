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

package msp

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/hyperledger/fabric/msp"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
)

type pendingMSPConfig struct {
	mspConfig *mspprotos.MSPConfig
	msp       msp.MSP
}

type mspConfigStore struct {
	idMap       map[string]*pendingMSPConfig
	proposedMgr msp.MSPManager
}

// MSPConfigHandler
type MSPConfigHandler struct {
	pendingConfig map[interface{}]*mspConfigStore
	pendingLock   sync.RWMutex
	msp.MSPManager
}

func NewMSPConfigHandler() *MSPConfigHandler {
	return &MSPConfigHandler{
		pendingConfig: make(map[interface{}]*mspConfigStore),
	}
}

// BeginConfig called when a config proposal is begun
func (bh *MSPConfigHandler) BeginConfig(tx interface{}) {
	bh.pendingLock.Lock()
	defer bh.pendingLock.Unlock()
	_, ok := bh.pendingConfig[tx]
	if ok {
		panic("Programming error, called BeginConfig multiply for the same tx")
	}
	bh.pendingConfig[tx] = &mspConfigStore{
		idMap: make(map[string]*pendingMSPConfig),
	}
}

// RollbackProposals called when a config proposal is abandoned
func (bh *MSPConfigHandler) RollbackProposals(tx interface{}) {
	bh.pendingLock.Lock()
	defer bh.pendingLock.Unlock()
	delete(bh.pendingConfig, tx)
}

// CommitProposals called when a config proposal is committed
func (bh *MSPConfigHandler) CommitProposals(tx interface{}) {
	bh.pendingLock.Lock()
	defer bh.pendingLock.Unlock()
	pendingConfig, ok := bh.pendingConfig[tx]
	if !ok {
		panic("Programming error, called BeginConfig multiply for the same tx")
	}

	bh.MSPManager = pendingConfig.proposedMgr
	delete(bh.pendingConfig, tx)
}

// ProposeValue called when config is added to a proposal
func (bh *MSPConfigHandler) ProposeMSP(tx interface{}, mspConfig *mspprotos.MSPConfig) (msp.MSP, error) {
	bh.pendingLock.RLock()
	pendingConfig, ok := bh.pendingConfig[tx]
	bh.pendingLock.RUnlock()
	if !ok {
		panic("Programming error, called BeginConfig multiply for the same tx")
	}

	// check that the type for that MSP is supported
	if mspConfig.Type != int32(msp.FABRIC) {
		return nil, fmt.Errorf("Setup error: unsupported msp type %d", mspConfig.Type)
	}

	// create the msp instance
	mspInst, err := msp.NewBccspMsp()
	if err != nil {
		return nil, fmt.Errorf("Creating the MSP manager failed, err %s", err)
	}

	// set it up
	err = mspInst.Setup(mspConfig)
	if err != nil {
		return nil, fmt.Errorf("Setting up the MSP manager failed, err %s", err)
	}

	// add the MSP to the map of pending MSPs
	mspID, _ := mspInst.GetIdentifier()

	existingPendingMSPConfig, ok := pendingConfig.idMap[mspID]
	if ok && !reflect.DeepEqual(existingPendingMSPConfig.mspConfig, mspConfig) {
		return nil, fmt.Errorf("Attempted to define two different versions of MSP: %s", mspID)
	}

	pendingConfig.idMap[mspID] = &pendingMSPConfig{
		mspConfig: mspConfig,
		msp:       mspInst,
	}

	return mspInst, nil
}

// PreCommit instantiates the MSP manager
func (bh *MSPConfigHandler) PreCommit(tx interface{}) error {
	bh.pendingLock.RLock()
	pendingConfig, ok := bh.pendingConfig[tx]
	bh.pendingLock.RUnlock()
	if !ok {
		panic("Programming error, called PreCommit for tx which was not started")
	}

	if len(pendingConfig.idMap) == 0 {
		// Cannot instantiate an MSP manager with no MSPs
		return nil
	}

	mspList := make([]msp.MSP, len(pendingConfig.idMap))
	i := 0
	for _, pendingMSP := range pendingConfig.idMap {
		mspList[i] = pendingMSP.msp
		i++
	}

	pendingConfig.proposedMgr = msp.NewMSPManager()
	err := pendingConfig.proposedMgr.Setup(mspList)
	return err
}
