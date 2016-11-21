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
	"sync"

	"github.com/op/go-logging"
)

var mspLogger = logging.MustGetLogger("msp")
var mspManager peerMspManagerImpl
var mspMgrCreateOnce sync.Once

type peerMspManagerImpl struct {
	// map that contains all MSPs that we have setup or otherwise added
	mspsMap map[string]PeerMSP

	// name of this manager
	mgrName string

	// error that might have occurred at startup
	up bool
}

// GetManager returns the one and only Membership Service Provider Manager instance
// This manager still needs to be initialized before use, by calling Setup
func GetManager() PeerMSPManager {
	mspMgrCreateOnce.Do(func() {
		mspLogger.Infof("Creating the msp manager")
		mspManager = peerMspManagerImpl{}
	})

	mspLogger.Infof("Returning MSP manager %p", mspManager)
	return &mspManager
}

func (mgr *peerMspManagerImpl) Setup(configFile string) error {
	if mgr.up {
		mspLogger.Warningf("MSP manager already up")
		return nil
	}

	mspLogger.Infof("Setting up the MSP manager from config file %s", configFile)

	// create the map that assigns MSP IDs to their manager instance - once
	mgr.mspsMap = make(map[string]PeerMSP)

	// TODO: for now we do the following; we expect
	// a single MSP configuration in the file (whose
	// ID we hard-set to DEFAULT). So we create an
	// MSP with that ID and we pass it the config
	// file straight away. The MSP will expect to
	// find its config in there
	mspID := "DEFAULT" // TODO: get the MSP ID from the file
	msp, err := newBccspMsp()
	if err != nil {
		return fmt.Errorf("Creating the MSP manager failed, err %s", err)
	}

	mspLogger.Infof("Setting up MSP %s", mspID)

	// we have the MSP - call setup on it
	err = msp.Setup(configFile)
	if err != nil {
		return fmt.Errorf("Setting up the MSP manager failed, err %s", err)
	}

	// add the MSP to the map of active MSPs
	mgr.mspsMap[mspID] = msp

	mgr.mgrName = "PeerMSPManager" // TODO: load the mgr name from file

	mgr.up = true

	mspLogger.Infof("MSP manager setup complete (config file %s)", configFile)

	return nil
}

func (mgr *peerMspManagerImpl) Reconfig(reconfigMessage string) error {
	// TODO
	return nil
}

func (mgr *peerMspManagerImpl) Name() string {
	return mgr.mgrName
}

func (mgr *peerMspManagerImpl) Policy() string {
	// TODO
	return ""
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

func (mgr *peerMspManagerImpl) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	mspLogger.Infof("Looking up MSP with ID %s", identifier.Mspid)
	msp := mgr.mspsMap[identifier.Mspid.Value]
	if msp == nil {
		return nil, fmt.Errorf("No MSP registered for MSP ID %s", identifier.Mspid)
	}

	return msp.GetSigningIdentity(identifier)
}

func (mgr *peerMspManagerImpl) DeserializeIdentity(serializedID []byte) (Identity, error) {
	/*
	// We first deserialize to a SerializedIdentity to get the MSP ID
	sId := &SerializedIdentity{}
	_, err := asn1.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, fmt.Errorf("Could not deserialize a SerializedIdentity, err %s", err)
	}

	// we can now attempt to obtain the MSP
	msp := mgr.mspsMap[sId.Mspid.Value]
	if msp == nil {
		return nil, fmt.Errorf("MSP %s is unknown", sId.Mspid.Value)
	}

	// if we have this MSP, we ask it to deserialize
	return msp.DeserializeIdentity(sId.IdBytes)
	*/
	return mgr.mspsMap["DEFAULT"].DeserializeIdentity(serializedID) // FIXME!
}

func (mgr *peerMspManagerImpl) DeleteSigningIdentity(identifier string) (bool, error) {
	// TODO
	return true, nil
}

// isValid checks whether the supplied identity is valid
func (mgr *peerMspManagerImpl) IsValid(id Identity, idId *ProviderIdentifier) (bool, error) {
	mspLogger.Infof("Looking up MSP with ID %s", idId.Value)
	msp := mgr.mspsMap[idId.Value]
	if msp == nil {
		return false, fmt.Errorf("No MSP registered for MSP ID %s", idId.Value)
	}

	return msp.IsValid(id)
}
