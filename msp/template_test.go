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

package msp_test

import (
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	. "github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func TestTemplate(t *testing.T) {
	conf, err := GetLocalMspConfig("sampleconfig/", "DEFAULT")
	assert.NoError(t, err)

	confBytes, err := proto.Marshal(conf)
	assert.NoError(t, err)

	// XXX We should really get the MSP name by inspecting it, but, we know it is DEFAULT so hardcoding for now
	ci := &common.ConfigItem{Type: common.ConfigItem_MSP, Key: "DEFAULT", Value: confBytes}

	configtxtest.WriteTemplate(configtxtest.MSPTemplateName, ci)
}
