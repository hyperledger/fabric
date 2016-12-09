package mspmgmt

import (
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
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

	fakeConfig = &msp.MSPManagerConfig{MspList: []*msp.MSPConfig{conf}, Name: "MGRFORTESTCHAIN"}

	err = GetManagerForChain(util.GetTestChainID()).Setup(fakeConfig)
	if err != nil {
		return err
	}

	return nil
}

// FIXME! Every chain needs an MSP config but for now,
// we don't have support for that; we get around it
// temporarily by storing the config the peer loaded
// and using it every time we're asked to get an MSP
// manager via LoadMSPManagerFromBlock
var fakeConfig *msp.MSPManagerConfig

func GetMSPManagerFromBlock(b *common.Block) (msp.MSPManager, error) {
	// FIXME! We need to extract the config item
	// that relates to MSP from the contig tx
	// inside this block, unmarshal it to extract
	// an *MSPManagerConfig that we can then pass
	// to the Setup method; for now we wing it by
	// passing the same config we got for the
	// local manager; this way chain creation tests
	// can proceed without being block by this

	// this hack is required to give us some configuration
	// so that we can return a valid MSP manager when
	// someone calls this function; it should work, provided
	// that this call occurs after the peer has started
	// and called LoadFakeSetupWithLocalMspAndTestChainMsp.
	// Notice that this happens very early in the peer
	// startup and so the assumption should be safe
	if fakeConfig == nil {
		panic("fakeConfig is nil")
	}

	mgr := msp.NewMSPManager()
	err := mgr.Setup(fakeConfig)
	if err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
