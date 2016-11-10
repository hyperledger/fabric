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

package ledgerconfig

import "github.com/spf13/viper"

var stateDatabase = "goleveldb"
var couchDBAddress = "127.0.0.1:5984"
var username = ""
var password = ""
var historyDatabase = true

// CouchDBDef contains parameters
type CouchDBDef struct {
	URL      string
	Username string
	Password string
}

//IsCouchDBEnabled exposes the useCouchDB variable
func IsCouchDBEnabled() bool {
	stateDatabase = viper.GetString("ledger.state.stateDatabase")
	if stateDatabase == "CouchDB" {
		return true
	}
	return false
}

//GetCouchDBDefinition exposes the useCouchDB variable
func GetCouchDBDefinition() *CouchDBDef {

	couchDBAddress = viper.GetString("ledger.state.couchDBConfig.couchDBAddress")
	username = viper.GetString("ledger.state.couchDBConfig.username")
	password = viper.GetString("ledger.state.couchDBConfig.password")

	return &CouchDBDef{couchDBAddress, username, password}
}

//IsHistoryDBEnabled exposes the historyDatabase variable
//History database can only be enabled if couchDb is enabled
//as it the history stored in the same couchDB instance.
//TODO put History DB in it's own instance
func IsHistoryDBEnabled() bool {
	historyDatabase = viper.GetBool("ledger.state.historyDatabase")
	if IsCouchDBEnabled() && historyDatabase {
		return historyDatabase
	}
	return false
}
