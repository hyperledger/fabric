/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/cache"
	mspprotos "github.com/hyperledger/fabric/protos/msp"

	"github.com/golang/protobuf/proto"
)

type pendingMSPConfig struct {
	mspConfig *mspprotos.MSPConfig
	msp       msp.MSP
}

// MSPConfigHandler
type MSPConfigHandler struct {
	idMap map[string]*pendingMSPConfig
}

func NewMSPConfigHandler() *MSPConfigHandler {
	return &MSPConfigHandler{
		idMap: make(map[string]*pendingMSPConfig),
	}
}

// ProposeValue called when an org defines an MSP
func (bh *MSPConfigHandler) ProposeMSP(mspConfig *mspprotos.MSPConfig) (msp.MSP, error) {
	// check that the type for that MSP is supported
	if mspConfig.Type != int32(msp.FABRIC) {
		return nil, fmt.Errorf("Setup error: unsupported msp type %d", mspConfig.Type)
	}

	// create the msp instance
	bccspMSP, err := msp.NewBccspMsp()
	if err != nil {
		return nil, fmt.Errorf("Creating the MSP manager failed, err %s", err)
	}

	mspInst, err := cache.New(bccspMSP)
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

	existingPendingMSPConfig, ok := bh.idMap[mspID]
	if ok && !proto.Equal(existingPendingMSPConfig.mspConfig, mspConfig) {
		return nil, fmt.Errorf("Attempted to define two different versions of MSP: %s", mspID)
	}

	if !ok {
		bh.idMap[mspID] = &pendingMSPConfig{
			mspConfig: mspConfig,
			msp:       mspInst,
		}
	}

	return mspInst, nil
}

func (bh *MSPConfigHandler) CreateMSPManager() (msp.MSPManager, error) {
	mspList := make([]msp.MSP, len(bh.idMap))
	i := 0
	for _, pendingMSP := range bh.idMap {
		mspList[i] = pendingMSP.msp
		i++
	}

	manager := msp.NewMSPManager()
	err := manager.Setup(mspList)
	return manager, err
}
