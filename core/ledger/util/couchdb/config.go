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
	"time"

	"github.com/spf13/viper"
)

// CouchDBDef contains parameters
type CouchDBDef struct {
	URL                   string
	Username              string
	Password              string
	MaxRetries            int
	MaxRetriesOnStartup   int
	RequestTimeout        time.Duration
	CreateGlobalChangesDB bool
}

//GetCouchDBDefinition exposes the useCouchDB variable
func GetCouchDBDefinition() *CouchDBDef {

	couchDBAddress := viper.GetString("ledger.state.couchDBConfig.couchDBAddress")
	username := viper.GetString("ledger.state.couchDBConfig.username")
	password := viper.GetString("ledger.state.couchDBConfig.password")
	maxRetries := viper.GetInt("ledger.state.couchDBConfig.maxRetries")
	maxRetriesOnStartup := viper.GetInt("ledger.state.couchDBConfig.maxRetriesOnStartup")
	requestTimeout := viper.GetDuration("ledger.state.couchDBConfig.requestTimeout")
	createGlobalChangesDB := viper.GetBool("ledger.state.couchDBConfig.createGlobalChangesDB")

	return &CouchDBDef{couchDBAddress, username, password, maxRetries, maxRetriesOnStartup, requestTimeout, createGlobalChangesDB}
}
