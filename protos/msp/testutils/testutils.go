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

package msptestutils

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/msp/utils"
	"github.com/hyperledger/fabric/protos/utils"
)

//GetTestBlockFromMspConfig gets a test block using provided conf with a config transaction in it
func GetTestBlockFromMspConfig(conf *msp.MSPConfig) (*common.Block, error) {
	confBytes, err := proto.Marshal(conf)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal failed for a configuration item payload, err %s", err)
	}

	ci := &common.ConfigurationItem{Type: common.ConfigurationItem_MSP, Key: msputils.MSPKey, Value: confBytes}
	ciBytes, err := proto.Marshal(ci)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal failed for a configuration item, err %s", err)
	}

	sci := &common.SignedConfigurationItem{ConfigurationItem: ciBytes}
	ce := &common.ConfigurationEnvelope{Items: []*common.SignedConfigurationItem{sci}}
	ceBytes, err := proto.Marshal(ce)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal failed for a ConfigurationEnvelope, err %s", err)
	}

	payl := &common.Payload{Data: ceBytes, Header: &common.Header{ChainHeader: &common.ChainHeader{Type: int32(common.HeaderType_CONFIGURATION_TRANSACTION), ChainID: "testchain"}}}
	paylBytes, err := utils.GetBytesPayload(payl)
	if err != nil {
		return nil, fmt.Errorf("GetBytesPayload failed, err %s", err)
	}

	env := &common.Envelope{Payload: paylBytes, Signature: []byte("signature")}
	envBytes, err := utils.GetBytesEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("GetBytesEnvelope failed, err %s", err)
	}

	return &common.Block{Data: &common.BlockData{Data: [][]byte{envBytes}}}, nil
}
