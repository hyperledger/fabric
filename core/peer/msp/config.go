package mspmgmt

import (
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/msp/utils"
)

func LoadLocalMsp(dir string) error {
	conf, err := msp.GetLocalMspConfig(dir)
	if err != nil {
		return err
	}

	return GetLocalMSP().Setup(conf)
}

// FIXME: this is required for now because we need a local MSP
// and also the MSP mgr for the test chain; as soon as the code
// to setup chains is ready, the chain should be setup using
// the method below and this method should disappear
func LoadFakeSetupWithLocalMspAndTestChainMsp(dir string) error {
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

// GetMSPManagerFromBlock returns a new MSP manager from a ConfigurationEnvelope
// Note that chainID should really be obtained from the block. Passing it for
// two reasons (1) efficiency (2) getting chainID from block using protos/utils
// will cause import cycles
func GetMSPManagerFromBlock(cid string, b *common.Block) (msp.MSPManager, error) {
	mgrConfig, err := msputils.GetMSPManagerConfigFromBlock(b)
	if err != nil {
		return nil, err
	}

	mgr := GetManagerForChain(cid)
	err = mgr.Setup(mgrConfig)
	if err != nil {
		return nil, err
	}

	return mgr, nil
}
