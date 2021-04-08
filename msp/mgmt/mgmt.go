/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mgmt

import (
	"sync"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/cache"
	"github.com/spf13/viper"
)

var (
	mspLogger = flogging.MustGetLogger("msp")

	m        sync.Mutex
	localMsp msp.MSP
)

// GetLocalMSP returns the local msp (and creates it if it doesn't exist)
func GetLocalMSP(cryptoProvider bccsp.BCCSP) msp.MSP {
	m.Lock()
	defer m.Unlock()

	if localMsp == nil {
		localMsp = loadLocalMSP(cryptoProvider)
	}
	return localMsp
}

func loadLocalMSP(bccsp bccsp.BCCSP) msp.MSP {
	// determine the type of MSP (by default, we'll use bccspMSP)
	mspType := viper.GetString("peer.localMspType")
	if mspType == "" {
		mspType = msp.ProviderTypeToString(msp.FABRIC)
	}

	newOpts, found := msp.Options[mspType]
	if !found {
		mspLogger.Panicf("msp type " + mspType + " unknown")
	}

	mspInst, err := msp.New(newOpts, bccsp)
	if err != nil {
		mspLogger.Fatalf("Failed to initialize local MSP, received err %+v", err)
	}
	switch mspType {
	case msp.ProviderTypeToString(msp.FABRIC):
		mspInst, err = cache.New(mspInst)
		if err != nil {
			mspLogger.Fatalf("Failed to initialize local MSP, received err %+v", err)
		}
	case msp.ProviderTypeToString(msp.IDEMIX):
		// Do nothing
	default:
		panic("msp type " + mspType + " unknown")
	}

	mspLogger.Debugf("Created new local MSP")

	return mspInst
}
