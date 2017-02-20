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
	pendingConfig *mspConfigStore
	msp.MSPManager
}

// BeginConfig called when a config proposal is begun
func (bh *MSPConfigHandler) BeginConfig() {
	if bh.pendingConfig != nil {
		panic("Programming error, called BeginValueProposals while a proposal was in process")
	}
	bh.pendingConfig = &mspConfigStore{
		idMap: make(map[string]*pendingMSPConfig),
	}
}

// RollbackProposals called when a config proposal is abandoned
func (bh *MSPConfigHandler) RollbackProposals() {
	bh.pendingConfig = nil
}

// CommitProposals called when a config proposal is committed
func (bh *MSPConfigHandler) CommitProposals() {
	if bh.pendingConfig == nil {
		panic("Programming error, called CommitProposals with no proposal in process")
	}

	bh.MSPManager = bh.pendingConfig.proposedMgr
	bh.pendingConfig = nil
}

// ProposeValue called when config is added to a proposal
func (bh *MSPConfigHandler) ProposeMSP(mspConfig *mspprotos.MSPConfig) (msp.MSP, error) {
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
	mspID, err := mspInst.GetIdentifier()
	if err != nil {
		return nil, fmt.Errorf("Could not extract msp identifier, err %s", err)
	}

	existingPendingMSPConfig, ok := bh.pendingConfig.idMap[mspID]
	if ok && !reflect.DeepEqual(existingPendingMSPConfig.mspConfig, mspConfig) {
		return nil, fmt.Errorf("Attempted to define two different versions of MSP: %s", mspID)
	}

	bh.pendingConfig.idMap[mspID] = &pendingMSPConfig{
		mspConfig: mspConfig,
		msp:       mspInst,
	}

	return mspInst, nil
}

// PreCommit instantiates the MSP manager
func (bh *MSPConfigHandler) PreCommit() error {
	if len(bh.pendingConfig.idMap) == 0 {
		// Cannot instantiate an MSP manager with no MSPs
		return nil
	}

	mspList := make([]msp.MSP, len(bh.pendingConfig.idMap))
	i := 0
	for _, pendingMSP := range bh.pendingConfig.idMap {
		mspList[i] = pendingMSP.msp
		i++
	}

	bh.pendingConfig.proposedMgr = msp.NewMSPManager()
	err := bh.pendingConfig.proposedMgr.Setup(mspList)
	return err
}
