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

package main

import (
	"github.com/hyperledger/fabric/common/configtx"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	//"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

func newChainRequest(consensusType, creationPolicy, newChannelId string) *cb.Envelope {
	//genConf.Orderer.OrdererType = consensusType
	//generator := provisional.New(genConf)
	//channelTemplate := generator.ChannelTemplate()
	channelTemplate := configtxtest.CompositeTemplate()

	signer, err := msp.NewNoopMsp().GetDefaultSigningIdentity()
	if err != nil {
		panic(err)
	}

	env, err := configtx.MakeChainCreationTransaction(creationPolicy, newChannelId, signer, channelTemplate)
	if err != nil {
		panic(err)
	}
	return env
}
