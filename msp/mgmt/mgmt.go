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
	"os"
	"path/filepath"
	"sync"

	"github.com/hyperledger/fabric/common/util"
	mspprotos "github.com/hyperledger/fabric/protos/msp"

	"github.com/hyperledger/fabric/msp"
	"github.com/op/go-logging"
)

func LoadLocalMsp(dir string) error {
	conf, err := msp.GetLocalMspConfig(dir)
	if err != nil {
		return err
	}

	return GetLocalMSP().Setup(conf)
}

func getConfigPath(dir string) (string, error) {
	// Try to read the dir
	if _, err := os.Stat(dir); err != nil {
		cfg := os.Getenv("PEER_CFG_PATH")
		if cfg != "" {
			dir = filepath.Join(cfg, dir)
		} else {
			dir = filepath.Join(os.Getenv("GOPATH"), "/src/github.com/hyperledger/fabric/msp/sampleconfig/")
		}
		if _, err := os.Stat(dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// FIXME: this is required for now because we need a local MSP
// and also the MSP mgr for the test chain; as soon as the code
// to setup chains is ready, the chain should be setup using
// the method below and this method should disappear
func LoadFakeSetupWithLocalMspAndTestChainMsp(dir string) error {
	var err error
	if dir, err = getConfigPath(dir); err != nil {
		return err
	}
	conf, err := msp.GetLocalMspConfig(dir)
	if err != nil {
		return err
	}

	err = GetLocalMSP().Setup(conf)
	if err != nil {
		return err
	}

	fakeConfig := []*mspprotos.MSPConfig{conf}

	err = GetManagerForChain(util.GetTestChainID()).Setup(fakeConfig)
	if err != nil {
		return err
	}

	return nil
}

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
func GetManagerForChain(chainID string) msp.MSPManager {
	m.Lock()
	defer m.Unlock()

	mspMgr, ok := mspMap[chainID]
	if !ok {
		mspLogger.Debugf("Created new msp manager for chain %s", chainID)
		mspMgr = msp.NewMSPManager()
		mspMap[chainID] = mspMgr
	} else {
		mspLogger.Debugf("Returning existing manager for chain %s", chainID)
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

// XXXSetMSPManager is a stopgap solution to transition from the custom MSP config block
// parsing to the configtx.Manager interface, while preserving the problematic singleton
// nature of the MSP manager
func XXXSetMSPManager(chainID string, manager msp.MSPManager) {
	m.Lock()
	defer m.Unlock()

	mspMap[chainID] = manager
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

// GetIdentityDeserializer returns the IdentityDeserializer for the given chain
func GetIdentityDeserializer(chainID string) msp.IdentityDeserializer {
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
