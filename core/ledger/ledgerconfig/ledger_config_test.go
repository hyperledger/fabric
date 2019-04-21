/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerconfig

import (
	"testing"

	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetTotalLimitDefault(t *testing.T) {
	setUpCoreYAMLConfig()
	defaultValue := GetTotalQueryLimit()
	assert.Equal(t, 100000, defaultValue) //test default config is 100000
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
