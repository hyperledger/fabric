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

package genesis

import (
	"sync"

	"github.com/spf13/viper"
)

var loadConfigOnce sync.Once

var genesis map[string]interface{}

func initConfigs() {
	loadConfigOnce.Do(func() { loadConfigs() })
}

func loadConfigs() {
	genesisLogger.Info("Loading configurations...")
	genesis = viper.GetStringMap("ledger.blockchain.genesisBlock")
	genesisLogger.Info("Configurations loaded: genesis=%s", genesis)
}

func getGenesis() map[string]interface{} {
	initConfigs()
	return genesis
}
