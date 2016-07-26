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
package ledger_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var testDBWrapper *db.TestDBWrapper

func SetupTestConfig() {
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	viper.SetDefault("peer.ledger.test.loadYAML", true)
	loadYAML := viper.GetBool("peer.ledger.test.loadYAML")
	if loadYAML {
		viper.SetConfigName("test")
		err := viper.ReadInConfig()
		if err != nil { // Handle errors reading the config file
			panic(fmt.Errorf("Fatal error config file: %s \n", err))
		}
	}
	var formatter = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} [%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)
}

func InitSpec() *ledger.Ledger {
	if testDBWrapper == nil {
		testDBWrapper = db.NewTestDBWrapper()
	}
	testDBWrapper.CreateFreshDBGinkgo()
	ledgerPtr, err := ledger.GetNewLedger()
	if err != nil {
		Fail("failed to get a fresh ledger")
	}
	return ledgerPtr
}

func TestLedger(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ledger Suite")
}
