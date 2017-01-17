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

package msputils

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
)

// FIXME: is this the right way to place this const?
const MSPKey = "MSP"

func GetMSPManagerConfigFromBlock(b *common.Block) ([]*msp.MSPConfig, error) {
	// TODO: should we check that b is a configuration
	// block or should we just assume our caller will
	// take care of that?

	env := &common.Envelope{}
	err := proto.Unmarshal(b.Data.Data[0], env)
	if err != nil {
		return nil, err
	}

	payl := &common.Payload{}
	err = proto.Unmarshal(env.Payload, payl)
	if err != nil {
		return nil, err
	}

	ctx := &common.ConfigurationEnvelope{}
	err = proto.Unmarshal(payl.Data, ctx)
	if err != nil {
		return nil, err
	}

	mgrConfig := make([]*msp.MSPConfig, 0)

	// NOTE: we do not verify any signature over the config
	// item; the reason is that we expect the block that is
	// supplied as argument to this function to either be
	// the genesis block, obtained by the application via a
	// trusted external channel, or to come from the validated
	// ledger (and hence it's already been validated and accepted
	// and we are re-reading it from there because this peer
	// is re-starting), or to have been validated using the
	// current MSP and the policies that govern changes to it
	for _, entry := range ctx.Items {
		ci := &common.ConfigurationItem{}
		err := proto.Unmarshal(entry.ConfigurationItem, ci)
		if err != nil {
			return nil, err
		}

		if ci.Type != common.ConfigurationItem_MSP {

			continue
		}

		// FIXME: is any more validation required?

		mspConf := &msp.MSPConfig{}
		err = proto.Unmarshal(ci.Value, mspConf)
		if err != nil {
			return nil, err
		}

		mgrConfig = append(mgrConfig, mspConf)
	}

	return mgrConfig, nil
}
