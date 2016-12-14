package mspmgmt

import (
	"github.com/hyperledger/fabric/core/util"
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
func GetMSPManagerFromBlock(b *common.Block) (msp.MSPManager, error) {
	mgrConfig, err := msputils.GetMSPManagerConfigFromBlock(b)
	if err != nil {
		return nil, err
	}

	mgr := msp.NewMSPManager()
	err = mgr.Setup(mgrConfig)
	if err != nil {
		return nil, err
	}

	return mgr, nil
}
