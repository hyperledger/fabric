/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

	"encoding/asn1"

	"github.com/op/go-logging"
)

var mspLogger = logging.MustGetLogger("msp")

type peerMspManagerImpl struct {
	// map that contains all MSPs that we have setup or otherwise added
	mspsMap map[string]PeerMSP

	// name of this manager
	mgrName string

	// error that might have occurred at startup
	up bool
}

func (mgr *peerMspManagerImpl) Setup(config *MSPManagerConfig) error {
	if mgr.up {
		mspLogger.Warningf("MSP manager already up")
		return nil
	}

	if config == nil {
		mspLogger.Errorf("Setup error: nil config object")
		return fmt.Errorf("Setup error: nil config object")
	}

	mspLogger.Infof("Setting up the MSP manager %s", config.Name)
	mgr.mgrName = config.Name

	// create the map that assigns MSP IDs to their manager instance - once
	mgr.mspsMap = make(map[string]PeerMSP)

	for _, mspConf := range config.MspList {
		// check that the type for that MSP is supported
		if mspConf.Type != FABRIC {
			mspLogger.Errorf("Setup error: unsupported msp type %d", mspConf.Type)
			return fmt.Errorf("Setup error: unsupported msp type %d", mspConf.Type)
		}

		mspLogger.Infof("Setting up MSP")

		// create the msp instance
		msp, err := newBccspMsp()
		if err != nil {
			mspLogger.Errorf("Creating the MSP manager failed, err %s", err)
			return fmt.Errorf("Creating the MSP manager failed, err %s", err)
		}

		// set it up
		err = msp.Setup(mspConf)
		if err != nil {
			mspLogger.Errorf("Setting up the MSP manager failed, err %s", err)
			return fmt.Errorf("Setting up the MSP manager failed, err %s", err)
		}

		// add the MSP to the map of active MSPs
		mspID, err := msp.Identifier()
		if err != nil {
			mspLogger.Errorf("Could not extract msp identifier, err %s", err)
			return fmt.Errorf("Could not extract msp identifier, err %s", err)
		}
		mgr.mspsMap[mspID] = msp
	}

	mgr.up = true

	mspLogger.Infof("MSP manager setup complete, setup %d msps", len(config.MspList))

	return nil
}

func (mgr *peerMspManagerImpl) Reconfig(config []byte) error {
	// TODO
	return nil
}

func (mgr *peerMspManagerImpl) Name() string {
	return mgr.mgrName
}

func (mgr *peerMspManagerImpl) EnlistedMSPs() (map[string]PeerMSP, error) {
	return mgr.mspsMap, nil
}

func (mgr *peerMspManagerImpl) AddMSP(configFile string) (string, error) {
	// TODO
	return "", nil
}

func (mgr *peerMspManagerImpl) RemoveMSP(identifier string) (string, error) {
	// TODO
	return "", nil
}

func (mgr *peerMspManagerImpl) ImportSigningIdentity(req *ImportRequest) (SigningIdentity, error) {
	// TODO
	return nil, nil
}

func (mgr *peerMspManagerImpl) DeserializeIdentity(serializedID []byte) (Identity, error) {
	// We first deserialize to a SerializedIdentity to get the MSP ID
	sId := &SerializedIdentity{}
	_, err := asn1.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, fmt.Errorf("Could not deserialize a SerializedIdentity, err %s", err)
	}

	// we can now attempt to obtain the MSP
	msp := mgr.mspsMap[sId.Mspid]
	if msp == nil {
		return nil, fmt.Errorf("MSP %s is unknown", sId.Mspid)
	}

	// if we have this MSP, we ask it to deserialize
	return msp.DeserializeIdentity(serializedID)
}

func (mgr *peerMspManagerImpl) DeleteSigningIdentity(identifier string) (bool, error) {
	// TODO
	return true, nil
}

// isValid checks whether the supplied identity is valid
func (mgr *peerMspManagerImpl) IsValid(id Identity, mspId string) (bool, error) {
	mspLogger.Infof("Looking up MSP with ID %s", mspId)
	msp := mgr.mspsMap[mspId]
	if msp == nil {
		return false, fmt.Errorf("No MSP registered for MSP ID %s", mspId)
	}

	return msp.IsValid(id)
}
