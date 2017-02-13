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

package localconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/viperutil"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var logger = logging.MustGetLogger("configtx/tool/localconfig")

func init() {
	logging.SetLevel(logging.ERROR, "")
}

// Prefix is the default config prefix for the orderer
const Prefix string = "CONFIGTX"

// TopLevel contains the genesis structures for use by the provisional bootstrapper
type TopLevel struct {
	Orderer Orderer
}

// Orderer contains config which is used for orderer genesis by the provisional bootstrapper
type Orderer struct {
	OrdererType  string
	Addresses    []string
	BatchTimeout time.Duration
	BatchSize    BatchSize
	Kafka        Kafka
}

// BatchSize contains configuration affecting the size of batches
type BatchSize struct {
	MaxMessageCount   uint32
	AbsoluteMaxBytes  uint32
	PreferredMaxBytes uint32
}

// Kafka contains config for the Kafka orderer
type Kafka struct {
	Brokers []string
}

var genesisDefaults = TopLevel{
	Orderer: Orderer{
		OrdererType:  "solo",
		Addresses:    []string{"127.0.0.1:7050"},
		BatchTimeout: 10 * time.Second,
		BatchSize: BatchSize{
			MaxMessageCount:   10,
			AbsoluteMaxBytes:  100000000,
			PreferredMaxBytes: 512 * 1024,
		},
		Kafka: Kafka{
			Brokers: []string{"127.0.0.1:9092"},
		},
	},
}

func (g *TopLevel) completeInitialization() {
	for {
		switch {
		case g.Orderer.OrdererType == "":
			logger.Infof("Orderer.OrdererType unset, setting to %s", genesisDefaults.Orderer.OrdererType)
			g.Orderer.OrdererType = genesisDefaults.Orderer.OrdererType
		case g.Orderer.Addresses == nil:
			logger.Infof("Orderer.Addresses unset, setting to %s", genesisDefaults.Orderer.Addresses)
			g.Orderer.Addresses = genesisDefaults.Orderer.Addresses
		case g.Orderer.BatchTimeout == 0:
			logger.Infof("Orderer.BatchTimeout unset, setting to %s", genesisDefaults.Orderer.BatchTimeout)
			g.Orderer.BatchTimeout = genesisDefaults.Orderer.BatchTimeout
		case g.Orderer.BatchTimeout == 0:
			logger.Infof("Orderer.BatchTimeout unset, setting to %s", genesisDefaults.Orderer.BatchTimeout)
			g.Orderer.BatchTimeout = genesisDefaults.Orderer.BatchTimeout
		case g.Orderer.BatchSize.MaxMessageCount == 0:
			logger.Infof("Orderer.BatchSize.MaxMessageCount unset, setting to %s", genesisDefaults.Orderer.BatchSize.MaxMessageCount)
			g.Orderer.BatchSize.MaxMessageCount = genesisDefaults.Orderer.BatchSize.MaxMessageCount
		case g.Orderer.BatchSize.AbsoluteMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.AbsoluteMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes)
			g.Orderer.BatchSize.AbsoluteMaxBytes = genesisDefaults.Orderer.BatchSize.AbsoluteMaxBytes
		case g.Orderer.BatchSize.PreferredMaxBytes == 0:
			logger.Infof("Orderer.BatchSize.PreferredMaxBytes unset, setting to %s", genesisDefaults.Orderer.BatchSize.PreferredMaxBytes)
			g.Orderer.BatchSize.PreferredMaxBytes = genesisDefaults.Orderer.BatchSize.PreferredMaxBytes
		case g.Orderer.Kafka.Brokers == nil:
			logger.Infof("Orderer.Kafka.Brokers unset, setting to %v", genesisDefaults.Orderer.Kafka.Brokers)
			g.Orderer.Kafka.Brokers = genesisDefaults.Orderer.Kafka.Brokers
		default:
			return
		}
	}
}

func Load() *TopLevel {
	config := viper.New()

	config.SetConfigName("genesis")

	var cfgPath string

	// Path to look for the config file in based on ORDERER_CFG_PATH and GOPATH
	searchPath := os.Getenv("ORDERER_CFG_PATH") + ":" + os.Getenv("GOPATH")
	for _, p := range filepath.SplitList(searchPath) {
		genesisPath := filepath.Join(p, "src/github.com/hyperledger/fabric/common/configtx/tool/")
		if _, err := os.Stat(filepath.Join(genesisPath, "genesis.yaml")); err != nil {
			// The yaml file does not exist in this component of the go src
			continue
		}
		cfgPath = genesisPath
	}
	if cfgPath == "" {
		logger.Fatalf("Could not find genesis.yaml, try setting GOPATH correctly")
	}
	config.AddConfigPath(cfgPath) // Path to look for the config file in

	// for environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Error reading %s plugin config from %s: %s", Prefix, cfgPath, err))
	}

	var uconf TopLevel

	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling into structure: %s", err))
	}

	uconf.completeInitialization()

	return &uconf
}
