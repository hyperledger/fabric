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
	"reflect"
	"sync"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/cache"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// LoadLocalMspWithType loads the local MSP with the specified type from the specified directory
func LoadLocalMspWithType(dir string, bccspConfig *factory.FactoryOpts, mspID, mspType string) error {
	if mspID == "" {
		return errors.New("the local MSP must have an ID")
	}

	conf, err := msp.GetLocalMspConfigWithType(dir, bccspConfig, mspID, mspType)
	if err != nil {
		return err
	}

	return GetLocalMSP().Setup(conf)
}

// LoadLocalMsp loads the local MSP from the specified directory
func LoadLocalMsp(dir string, bccspConfig *factory.FactoryOpts, mspID string) error {
	if mspID == "" {
		return errors.New("the local MSP must have an ID")
	}

	conf, err := msp.GetLocalMspConfig(dir, bccspConfig, mspID)
	if err != nil {
		return err
	}

	return GetLocalMSP().Setup(conf)
}

// Loads the development local MSP for use in testing.  Not valid for production/runtime context
func LoadDevMsp() error {
	mspDir, err := config.GetDevMspDir()
	if err != nil {
		return err
	}

	return LoadLocalMsp(mspDir, nil, "DEFAULT")
}

// FIXME: AS SOON AS THE CHAIN MANAGEMENT CODE IS COMPLETE,
// THESE MAPS AND HELPSER FUNCTIONS SHOULD DISAPPEAR BECAUSE
// OWNERSHIP OF PER-CHAIN MSP MANAGERS WILL BE HANDLED BY IT;
// HOWEVER IN THE INTERIM, THESE HELPER FUNCTIONS ARE REQUIRED

var m sync.Mutex
var localMsp msp.MSP
var mspMap map[string]msp.MSPManager = make(map[string]msp.MSPManager)
var mspLogger = flogging.MustGetLogger("msp")

// TODO - this is a temporary solution to allow the peer to track whether the
// MSPManager has been setup for a channel, which indicates whether the channel
// exists or not
type mspMgmtMgr struct {
	msp.MSPManager
	// track whether this MSPManager has been setup successfully
	up bool
}

func (mgr *mspMgmtMgr) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	if !mgr.up {
		return nil, errors.New("channel doesn't exist")
	}
	return mgr.MSPManager.DeserializeIdentity(serializedIdentity)
}

func (mgr *mspMgmtMgr) Setup(msps []msp.MSP) error {
	err := mgr.MSPManager.Setup(msps)
	if err == nil {
		mgr.up = true
	}
	return err
}

// GetManagerForChain returns the msp manager for the supplied
// chain; if no such manager exists, one is created
func GetManagerForChain(chainID string) msp.MSPManager {
	m.Lock()
	defer m.Unlock()

	mspMgr, ok := mspMap[chainID]
	if !ok {
		mspLogger.Debugf("Created new msp manager for channel `%s`", chainID)
		mspMgmtMgr := &mspMgmtMgr{msp.NewMSPManager(), false}
		mspMap[chainID] = mspMgmtMgr
		mspMgr = mspMgmtMgr
	} else {
		// check for internal mspManagerImpl and mspMgmtMgr types. if a different
		// type is found, it's because a developer has added a new type that
		// implements the MSPManager interface and should add a case to the logic
		// above to handle it.
		if !(reflect.TypeOf(mspMgr).Elem().Name() == "mspManagerImpl" || reflect.TypeOf(mspMgr).Elem().Name() == "mspMgmtMgr") {
			panic("Found unexpected MSPManager type.")
		}
		mspLogger.Debugf("Returning existing manager for channel '%s'", chainID)
	}
	return mspMgr
}

// GetManagers returns all the managers registered
func GetDeserializers() map[string]msp.IdentityDeserializer {
	m.Lock()
	defer m.Unlock()

	clone := make(map[string]msp.IdentityDeserializer)

	for key, mspManager := range mspMap {
		clone[key] = mspManager
	}

	return clone
}

// XXXSetMSPManager is a stopgap solution to transition from the custom MSP config block
// parsing to the channelconfig.Resources interface, while preserving the problematic singleton
// nature of the MSP manager
func XXXSetMSPManager(chainID string, manager msp.MSPManager) {
	m.Lock()
	defer m.Unlock()

	mspMap[chainID] = &mspMgmtMgr{manager, true}
}

// GetLocalMSP returns the local msp (and creates it if it doesn't exist)
func GetLocalMSP() msp.MSP {
	var lclMsp msp.MSP
	var created bool = false
	{
		// determine the type of MSP (by default, we'll use bccspMSP)
		mspType := viper.GetString("peer.localMspType")
		if mspType == "" {
			mspType = msp.ProviderTypeToString(msp.FABRIC)
		}

		// based on the MSP type, generate the new opts
		var newOpts msp.NewOpts
		switch mspType {
		case msp.ProviderTypeToString(msp.FABRIC):
			newOpts = &msp.BCCSPNewOpts{NewBaseOpts: msp.NewBaseOpts{Version: msp.MSPv1_0}}
		case msp.ProviderTypeToString(msp.IDEMIX):
			newOpts = &msp.IdemixNewOpts{msp.NewBaseOpts{Version: msp.MSPv1_1}}
		default:
			panic("msp type " + mspType + " unknown")
		}

		m.Lock()
		defer m.Unlock()

		lclMsp = localMsp
		if lclMsp == nil {
			var err error
			created = true

			mspInst, err := msp.New(newOpts)
			if err != nil {
				mspLogger.Fatalf("Failed to initialize local MSP, received err %+v", err)
			}

			switch mspType {
			case msp.ProviderTypeToString(msp.FABRIC):
				lclMsp, err = cache.New(mspInst)
				if err != nil {
					mspLogger.Fatalf("Failed to initialize local MSP, received err %+v", err)
				}
			case msp.ProviderTypeToString(msp.IDEMIX):
				lclMsp = mspInst
			default:
				panic("msp type " + mspType + " unknown")
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
		mspLogger.Panicf("Failed getting local signing identity [%+v]", err)
	}
	return id
}
