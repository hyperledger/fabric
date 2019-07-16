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

func TestLedgerConfigPathDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	assert.Equal(t, "/var/hyperledger/production/ledgersData", GetRootPath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/ledgerProvider", GetLedgerProviderPath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/stateLeveldb", GetStateLevelDBPath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/historyLeveldb", GetHistoryLevelDBPath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/chains", GetBlockStorePath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/pvtdataStore", GetPvtdataStorePath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/bookkeeper", GetInternalBookkeeperPath())
	assert.Equal(t, "/var/hyperledger/production/ledgersData/fileLock", GetFileLockPath())
}

func TestLedgerConfigPath(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("peer.fileSystemPath", "/tmp/hyperledger/production")
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData", GetRootPath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/ledgerProvider", GetLedgerProviderPath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/stateLeveldb", GetStateLevelDBPath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/historyLeveldb", GetHistoryLevelDBPath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/chains", GetBlockStorePath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/pvtdataStore", GetPvtdataStorePath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/bookkeeper", GetInternalBookkeeperPath())
	assert.Equal(t, "/tmp/hyperledger/production/ledgersData/fileLock", GetFileLockPath())
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

func TestGetQueryLimitDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := GetInternalQueryLimit()
	assert.Equal(t, 1000, defaultValue) //test default config is 1000
}

func TestGetQueryLimitUnset(t *testing.T) {
	viper.Reset()
	defaultValue := GetInternalQueryLimit()
	assert.Equal(t, 1000, defaultValue) //test default config is 1000
}

func TestGetQueryLimit(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.couchDBConfig.internalQueryLimit", 5000)
	updatedValue := GetInternalQueryLimit()
	assert.Equal(t, 5000, updatedValue) //test config returns 5000
}

func TestMaxBatchUpdateSizeDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := GetMaxBatchUpdateSize()
	assert.Equal(t, 1000, defaultValue) //test default config is 1000
}

func TestMaxBatchUpdateSizeUnset(t *testing.T) {
	viper.Reset()
	defaultValue := GetMaxBatchUpdateSize()
	assert.Equal(t, 500, defaultValue) // 500 if maxBatchUpdateSize is not set
}

func TestMaxBatchUpdateSize(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.couchDBConfig.maxBatchUpdateSize", 2000)
	updatedValue := GetMaxBatchUpdateSize()
	assert.Equal(t, 2000, updatedValue) //test config returns 2000
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

func TestIsQueryReadHasingEnabled(t *testing.T) {
	assert.True(t, IsQueryReadsHashingEnabled())
}

func TestGetMaxDegreeQueryReadsHashing(t *testing.T) {
	assert.Equal(t, uint32(50), GetMaxDegreeQueryReadsHashing())
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

func TestIsAutoWarmIndexesEnabledDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := IsAutoWarmIndexesEnabled()
	assert.True(t, defaultValue) //test default config is true
}

func TestIsAutoWarmIndexesEnabledUnset(t *testing.T) {
	viper.Reset()
	defaultValue := IsAutoWarmIndexesEnabled()
	assert.True(t, defaultValue) //test default config is true
}

func TestIsAutoWarmIndexesEnabledTrue(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.couchDBConfig.autoWarmIndexes", true)
	updatedValue := IsAutoWarmIndexesEnabled()
	assert.True(t, updatedValue) //test config returns true
}

func TestIsAutoWarmIndexesEnabledFalse(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.couchDBConfig.autoWarmIndexes", false)
	updatedValue := IsAutoWarmIndexesEnabled()
	assert.False(t, updatedValue) //test config returns false
}

func TestGetWarmIndexesAfterNBlocksDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := GetWarmIndexesAfterNBlocks()
	assert.Equal(t, 1, defaultValue) //test default config is true
}

func TestGetWarmIndexesAfterNBlocksUnset(t *testing.T) {
	viper.Reset()
	defaultValue := GetWarmIndexesAfterNBlocks()
	assert.Equal(t, 1, defaultValue) //test default config is true
}

func TestGetWarmIndexesAfterNBlocks(t *testing.T) {
	setUpCoreYAMLConfig()
	defer ledgertestutil.ResetConfigToDefaultValues()
	viper.Set("ledger.state.couchDBConfig.warmIndexesAfterNBlocks", 10)
	updatedValue := GetWarmIndexesAfterNBlocks()
	assert.Equal(t, 10, updatedValue)
}

func TestGetMaxBlockfileSize(t *testing.T) {
	assert.Equal(t, 67108864, GetMaxBlockfileSize())
}

func setUpCoreYAMLConfig() {
	//call a helper method to load the core.yaml
	ledgertestutil.SetupCoreYAMLConfig()
}
