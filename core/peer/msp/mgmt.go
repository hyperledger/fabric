package mspmgmt

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
var peerLogger = logging.MustGetLogger("peer")

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
		peerLogger.Infof("Created new msp manager for chain %s", ChainID)
	} else {
		peerLogger.Infof("Returinging existing manager for chain %s", ChainID)
	}

	return mspMgr
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
				peerLogger.Fatalf("Failed to initlaize local MSP, received err %s", err)
			}
			localMsp = lclMsp
		}
	}

	if created {
		peerLogger.Infof("Created new local MSP")
	} else {
		peerLogger.Infof("Returning existing local MSP")
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
