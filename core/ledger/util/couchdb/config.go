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

package couchdb

import (
	"github.com/spf13/viper"
)

//GetCouchDBDefinition exposes the useCouchDB variable
func GetCouchDBDefinition() Config {
	return Config{
		Address:               viper.GetString("ledger.state.couchDBConfig.couchDBAddress"),
		Username:              viper.GetString("ledger.state.couchDBConfig.username"),
		Password:              viper.GetString("ledger.state.couchDBConfig.password"),
		MaxRetries:            viper.GetInt("ledger.state.couchDBConfig.maxRetries"),
		MaxRetriesOnStartup:   viper.GetInt("ledger.state.couchDBConfig.maxRetriesOnStartup"),
		RequestTimeout:        viper.GetDuration("ledger.state.couchDBConfig.requestTimeout"),
		CreateGlobalChangesDB: viper.GetBool("ledger.state.couchDBConfig.createGlobalChangesDB"),
	}
}
