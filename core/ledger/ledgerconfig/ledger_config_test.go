/*h
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

import (
	"testing"

	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestIsCouchDBEnabledDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	// During a build the default values should be false.

	// If the  ledger test are run with CouchDb enabled, need to provide a mechanism
	// To let this test run but still test default values.
	if IsCouchDBEnabled() == true {
		ledgertestutil.ResetConfigToDefaultValues()
		defer viper.Set("ledger.state.stateDatabase", "CouchDB")
	}
	defaultValue := IsCouchDBEnabled()
	assert.False(t, defaultValue) //test default config is false
}

func TestIsCouchDBEnabled(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.stateDatabase", "CouchDB")
	updatedValue := IsCouchDBEnabled()
	assert.True(t, updatedValue) //test config returns true
}

func TestGetTotalLimitDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := GetTotalQueryLimit()
	assert.Equal(t, 10000, defaultValue) //test default config is 1000
}

func TestGetTotalLimitUnset(t *testing.T) {
	viper.Reset()
	defaultValue := GetTotalQueryLimit()
	assert.Equal(t, 10000, defaultValue) //test default config is 1000
}

func TestGetTotalLimit(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.totalQueryLimit", 5000)
	updatedValue := GetTotalQueryLimit()
	assert.Equal(t, 5000, updatedValue) //test config returns 5000
}

func TestPvtdataStorePurgeIntervalDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := GetPvtdataStorePurgeInterval()
	assert.Equal(t, uint64(100), defaultValue) //test default config is 100
}

func TestPvtdataStorePurgeIntervalUnset(t *testing.T) {
	viper.Reset()
	defaultValue := GetPvtdataStorePurgeInterval()
	assert.Equal(t, uint64(100), defaultValue) // 100 if purgeInterval is not set
}

func TestPvtdataStorePurgeInterval(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.pvtdataStore.purgeInterval", 1000)
	updatedValue := GetPvtdataStorePurgeInterval()
	assert.Equal(t, uint64(1000), updatedValue) //test config returns 1000
}

func TestPvtdataStoreCollElgProcMaxDbBatchSize(t *testing.T) {
	defaultVal := confCollElgProcMaxDbBatchSize.DefaultVal
	testVal := defaultVal + 1
	assert.Equal(t, defaultVal, GetPvtdataStoreCollElgProcMaxDbBatchSize())
	viper.Set("ledger.pvtdataStore.collElgProcMaxDbBatchSize", testVal)
	assert.Equal(t, testVal, GetPvtdataStoreCollElgProcMaxDbBatchSize())
}

func TestCollElgProcDbBatchesInterval(t *testing.T) {
	defaultVal := confCollElgProcDbBatchesInterval.DefaultVal
	testVal := defaultVal + 1
	assert.Equal(t, defaultVal, GetPvtdataStoreCollElgProcDbBatchesInterval())
	viper.Set("ledger.pvtdataStore.collElgProcDbBatchesInterval", testVal)
	assert.Equal(t, testVal, GetPvtdataStoreCollElgProcDbBatchesInterval())
}

func TestIsHistoryDBEnabledDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := IsHistoryDBEnabled()
	assert.False(t, defaultValue) //test default config is false
}

func TestIsHistoryDBEnabledTrue(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.history.enableHistoryDatabase", true)
	updatedValue := IsHistoryDBEnabled()
	assert.True(t, updatedValue) //test config returns true
}

func TestIsHistoryDBEnabledFalse(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.history.enableHistoryDatabase", false)
	updatedValue := IsHistoryDBEnabled()
	assert.False(t, updatedValue) //test config returns false
}

func setUpCoreYAMLConfig() {
	//call a helper method to load the core.yaml
	ledgertestutil.SetupCoreYAMLConfig()
}
