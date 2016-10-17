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

package state

import (
	"fmt"
	"sync"

	"github.com/spf13/viper"
)

var loadConfigOnce sync.Once

var stateImplName stateImplType
var stateImplConfigs map[string]interface{}
var deltaHistorySize int

func initConfig() {
	loadConfigOnce.Do(func() { loadConfig() })
}

func loadConfig() {
	logger.Info("Loading configurations...")
	stateImplName = stateImplType(viper.GetString("ledger.state.dataStructure.name"))
	stateImplConfigs = viper.GetStringMap("ledger.state.dataStructure.configs")
	deltaHistorySize = viper.GetInt("ledger.state.deltaHistorySize")
	logger.Infof("Configurations loaded. stateImplName=[%s], stateImplConfigs=%s, deltaHistorySize=[%d]",
		stateImplName, stateImplConfigs, deltaHistorySize)

	if len(stateImplName) == 0 {
		stateImplName = defaultStateImpl
		stateImplConfigs = nil
	} else if stateImplName != buckettreeType && stateImplName != trieType && stateImplName != rawType {
		panic(fmt.Errorf("Error during initialization of state implementation. State data structure '%s' is not valid.", stateImplName))
	}

	if deltaHistorySize < 0 {
		panic(fmt.Errorf("Delta history size must be greater than or equal to 0. Current value is %d.", deltaHistorySize))
	}
}
