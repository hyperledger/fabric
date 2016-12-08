package msp

import "sync"

var m sync.Mutex
var localMsp PeerMSP
var mspMap map[string]PeerMSPManager = make(map[string]PeerMSPManager)

// GetManagerForChain returns the msp manager for the supplied
// chain; if no such manager exists, one is created
func GetManagerForChain(ChainID string) PeerMSPManager {
	var msp PeerMSPManager
	var created bool = false
	{
		m.Lock()
		defer m.Unlock()

		msp = mspMap[ChainID]
		if msp == nil {
			created = true
			msp = &peerMspManagerImpl{}
			mspMap[ChainID] = msp
		}
	}

	if created {
		mspLogger.Infof("Created new msp manager for chain %s", ChainID)
	} else {
		mspLogger.Infof("Returinging existing manager for chain %s", ChainID)
	}

	return msp
}

// GetLocalMSP returns the local msp (and creates it if it doesn't exist)
func GetLocalMSP() PeerMSP {
	var msp PeerMSP
	var created bool = false
	{
		m.Lock()
		defer m.Unlock()

		msp = localMsp
		if msp == nil {
			var err error
			created = true
			msp, err = newBccspMsp()
			if err != nil {
				mspLogger.Fatalf("Failed to initlaize local MSP, received err %s", err)
			}
			localMsp = msp
		}
	}

	if created {
		mspLogger.Infof("Created new local MSP")
	} else {
		mspLogger.Infof("Returning existing local MSP")
	}

	return msp
}
