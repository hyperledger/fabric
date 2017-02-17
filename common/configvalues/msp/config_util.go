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
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
)

const (
	MSPKey = "MSP"
)

// TemplateGroupMSP creates an MSP ConfigValue at the given configPath
func TemplateGroupMSP(configPath []string, mspConf *msp.MSPConfig) *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	intermediate := result
	for _, group := range configPath {
		intermediate.Groups[group] = cb.NewConfigGroup()
		intermediate = intermediate.Groups[group]
	}
	intermediate.Values[MSPKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(mspConf),
	}
	return result
}
