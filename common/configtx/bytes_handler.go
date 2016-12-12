/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package configtx

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

// BytesHandler is a trivial ConfigHandler which simpy tracks the bytes stores in a config
type BytesHandler struct {
	config   map[string][]byte
	proposed map[string][]byte
}

// NewBytesHandler creates a new BytesHandler
func NewBytesHandler() *BytesHandler {
	return &BytesHandler{
		config: make(map[string][]byte),
	}
}

// BeginConfig called when a config proposal is begun
func (bh *BytesHandler) BeginConfig() {
	if bh.proposed != nil {
		panic("Programming error, called BeginConfig while a proposal was in process")
	}
	bh.proposed = make(map[string][]byte)
}

// RollbackConfig called when a config proposal is abandoned
func (bh *BytesHandler) RollbackConfig() {
	bh.proposed = nil
}

// CommitConfig called when a config proposal is committed
func (bh *BytesHandler) CommitConfig() {
	if bh.proposed == nil {
		panic("Programming error, called CommitConfig with no proposal in process")
	}
	bh.config = bh.proposed
	bh.proposed = nil
}

// ProposeConfig called when config is added to a proposal
func (bh *BytesHandler) ProposeConfig(configItem *cb.ConfigurationItem) error {
	bh.proposed[configItem.Key] = configItem.Value
	return nil
}

// GetBytes allows the caller to retrieve the bytes for a config
func (bh *BytesHandler) GetBytes(id string) []byte {
	return bh.config[id]
}
