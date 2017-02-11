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

package msp

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
)

// MSPConfigHandler
type MSPConfigHandler struct {
	msp.MSPManager
	config      []*mspprotos.MSPConfig
	proposedMgr msp.MSPManager
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

	bh.MSPManager = bh.proposedMgr
	bh.config = nil
	bh.proposedMgr = nil
}

// ProposeConfig called when config is added to a proposal
func (bh *MSPConfigHandler) ProposeConfig(configItem *common.ConfigItem) error {
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
