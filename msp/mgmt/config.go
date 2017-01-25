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
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
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

// MSPConfigHandler
type MSPConfigHandler struct {
	config       []*mspprotos.MSPConfig
	proposedMgr  msp.MSPManager
	committedMgr msp.MSPManager
}

// BeginConfig called when a config proposal is begun
func (bh *MSPConfigHandler) BeginConfig() {
	if bh.config != nil || bh.proposedMgr != nil {
		panic("Programming error, called BeginConfig while a proposal was in process")
	}
	bh.config = make([]*mspprotos.MSPConfig, 0)
}

// RollbackConfig called when a config proposal is abandoned
func (bh *MSPConfigHandler) RollbackConfig() {
	bh.config = nil
	bh.proposedMgr = nil
}

// CommitConfig called when a config proposal is committed
func (bh *MSPConfigHandler) CommitConfig() {
	if bh.config == nil {
		panic("Programming error, called CommitConfig with no proposal in process")
	}

	bh.committedMgr = bh.proposedMgr
	bh.config = nil
	bh.proposedMgr = nil
}

// ProposeConfig called when config is added to a proposal
func (bh *MSPConfigHandler) ProposeConfig(configItem *common.ConfigurationItem) error {
	mspconfig := &mspprotos.MSPConfig{}
	err := proto.Unmarshal(configItem.Value, mspconfig)
	if err != nil {
		return fmt.Errorf("Error unmarshalling msp config item, err %s", err)
	}

	bh.config = append(bh.config, []*mspprotos.MSPConfig{mspconfig}...)
	// the only way to make sure that I have a
	// workable config is to toss the proposed
	// manager, create a new one, call setup on
	// it and return whatever error setup gives me
	bh.proposedMgr = msp.NewMSPManager()
	return bh.proposedMgr.Setup(bh.config)
}

// GetMSPManager returns the currently committed MSP manager
func (bh *MSPConfigHandler) GetMSPManager() msp.MSPManager {
	return bh.committedMgr
}

// DesierializeIdentity allows *MSPConfigHandler to implement the msp.Common interface
func (bh *MSPConfigHandler) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	return bh.committedMgr.DeserializeIdentity(serializedIdentity)
}
