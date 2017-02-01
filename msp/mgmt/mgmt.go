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

package mgmt

import (
	"sync"

	"github.com/hyperledger/fabric/msp"
	"github.com/op/go-logging"
)

// FIXME: AS SOON AS THE CHAIN MANAGEMENT CODE IS COMPLETE,
// THESE MAPS AND HELPSER FUNCTIONS SHOULD DISAPPEAR BECAUSE
// OWNERSHIP OF PER-CHAIN MSP MANAGERS WILL BE HANDLED BY IT;
// HOWEVER IN THE INTERIM, THESE HELPER FUNCTIONS ARE REQUIRED

var m sync.Mutex
var localMsp msp.MSP
var mspMap map[string]msp.MSPManager = make(map[string]msp.MSPManager)
var mspLogger = logging.MustGetLogger("msp")

// GetManagerForChain returns the msp manager for the supplied
// chain; if no such manager exists, one is created
func GetManagerForChain(ChainID string) msp.MSPManager {
	var mspMgr msp.MSPManager
	var created bool = false
	{
		m.Lock()
		defer m.Unlock()

		mspMgr = mspMap[ChainID]
		if mspMgr == nil {
			created = true
			mspMgr = msp.NewMSPManager()
			mspMap[ChainID] = mspMgr
		}
	}

	if created {
		mspLogger.Debugf("Created new msp manager for chain %s", ChainID)
	} else {
		mspLogger.Debugf("Returning existing manager for chain %s", ChainID)
	}

	return mspMgr
}

// GetManagers returns all the managers registered
func GetManagers() map[string]msp.MSPManager {
	m.Lock()
	defer m.Unlock()

	clone := make(map[string]msp.MSPManager)

	for key, mspManager := range mspMap {
		clone[key] = mspManager
	}

	return clone
}

// GetManagerForChainIfExists returns the MSPManager associated to ChainID
// it it exists
func GetManagerForChainIfExists(ChainID string) msp.MSPManager {
	m.Lock()
	defer m.Unlock()

	return mspMap[ChainID]
}

// GetLocalMSP returns the local msp (and creates it if it doesn't exist)
func GetLocalMSP() msp.MSP {
	var lclMsp msp.MSP
	var created bool = false
	{
		m.Lock()
		defer m.Unlock()

		lclMsp = localMsp
		if lclMsp == nil {
			var err error
			created = true
			lclMsp, err = msp.NewBccspMsp()
			if err != nil {
				mspLogger.Fatalf("Failed to initialize local MSP, received err %s", err)
			}
			localMsp = lclMsp
		}
	}

	if created {
		mspLogger.Debugf("Created new local MSP")
	} else {
		mspLogger.Debugf("Returning existing local MSP")
	}

	return lclMsp
}

//GetMSPCommon returns the common interface
func GetMSPCommon(chainID string) msp.Common {
	if chainID == "" {
		return GetLocalMSP()
	}

	return GetManagerForChain(chainID)
}

// GetLocalSigningIdentityOrPanic returns the local signing identity or panic in case
// or error
func GetLocalSigningIdentityOrPanic() msp.SigningIdentity {
	id, err := GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		mspLogger.Panicf("Failed getting local signing identity [%s]", err)
	}
	return id
}
