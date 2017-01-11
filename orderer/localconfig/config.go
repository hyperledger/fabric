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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

var logger = logging.MustGetLogger("orderer/config")

func init() {
	logging.SetLevel(logging.ERROR, "")
}

// Prefix is the default config prefix for the orderer
const Prefix string = "ORDERER"

// General contains config which should be common among all orderer types
type General struct {
	LedgerType    string
	QueueSize     uint32
	MaxWindowSize uint32
	ListenAddress string
	ListenPort    uint16
	GenesisMethod string
	GenesisFile   string
	Profile       Profile
	LogLevel      string
}

// Genesis contains config which is used by the provisional bootstrapper
type Genesis struct {
	OrdererType  string
	BatchTimeout time.Duration
	BatchSize    BatchSize
}

// BatchSize contains configuration affecting the size of batches
type BatchSize struct {
	MaxMessageCount uint32
}

// Profile contains configuration for Go pprof profiling
type Profile struct {
	Enabled bool
	Address string
}

// RAMLedger contains config for the RAM ledger
type RAMLedger struct {
	HistorySize uint
}

// FileLedger contains config for the File ledger
type FileLedger struct {
	Location string
	Prefix   string
}

// Kafka contains config for the Kafka orderer
type Kafka struct {
	Brokers []string // TODO This should be deprecated and this information should be stored in the config block
	Retry   Retry
	Verbose bool
	Version sarama.KafkaVersion
}

// Retry contains config for the reconnection attempts to the Kafka brokers
type Retry struct {
	Period time.Duration
	Stop   time.Duration
}

// TopLevel directly corresponds to the orderer config yaml
// Note, for non 1-1 mappings, you may append
// something like `mapstructure:"weirdFoRMat"` to
// modify the default mapping, see the "Unmarshal"
// section of https://github.com/spf13/viper for more info
type TopLevel struct {
	General    General
	RAMLedger  RAMLedger
	FileLedger FileLedger
	Kafka      Kafka
	Genesis    Genesis
}

var defaults = TopLevel{
	General: General{
		LedgerType:    "ram",
		QueueSize:     1000,
		MaxWindowSize: 1000,
		ListenAddress: "127.0.0.1",
		ListenPort:    7050,
		GenesisMethod: "provisional",
		GenesisFile:   "./genesisblock",
		Profile: Profile{
			Enabled: false,
			Address: "0.0.0.0:6060",
		},
		LogLevel: "INFO",
	},
	RAMLedger: RAMLedger{
		HistorySize: 10000,
	},
	FileLedger: FileLedger{
		Location: "",
		Prefix:   "hyperledger-fabric-rawledger",
	},
	Kafka: Kafka{
		Brokers: []string{"127.0.0.1:9092"},
		Retry: Retry{
			Period: 3 * time.Second,
			Stop:   60 * time.Second,
		},
		Verbose: false,
		Version: sarama.V0_9_0_1,
	},
	Genesis: Genesis{
		OrdererType:  "solo",
		BatchTimeout: 10 * time.Second,
		BatchSize: BatchSize{
			MaxMessageCount: 10,
		},
	},
}

func (c *TopLevel) completeInitialization() {
	defer logger.Infof("Validated configuration to: %+v", c)

	for {
		switch {
		case c.General.LedgerType == "":
			logger.Infof("General.LedgerType unset, setting to %s", defaults.General.LedgerType)
			c.General.LedgerType = defaults.General.LedgerType
		case c.General.QueueSize == 0:
			logger.Infof("General.QueueSize unset, setting to %s", defaults.General.QueueSize)
			c.General.QueueSize = defaults.General.QueueSize
		case c.General.MaxWindowSize == 0:
			logger.Infof("General.MaxWindowSize unset, setting to %s", defaults.General.MaxWindowSize)
			c.General.MaxWindowSize = defaults.General.MaxWindowSize
		case c.General.ListenAddress == "":
			logger.Infof("General.ListenAddress unset, setting to %s", defaults.General.ListenAddress)
			c.General.ListenAddress = defaults.General.ListenAddress
		case c.General.ListenPort == 0:
			logger.Infof("General.ListenPort unset, setting to %s", defaults.General.ListenPort)
			c.General.ListenPort = defaults.General.ListenPort
		case c.General.LogLevel == "":
			logger.Infof("General.LogLevel unset, setting to %s", defaults.General.LogLevel)
			c.General.LogLevel = defaults.General.LogLevel
		case c.General.GenesisMethod == "":
			c.General.GenesisMethod = defaults.General.GenesisMethod
		case c.General.GenesisFile == "":
			c.General.GenesisFile = defaults.General.GenesisFile
		case c.General.Profile.Enabled && (c.General.Profile.Address == ""):
			logger.Infof("Profiling enabled and General.Profile.Address unset, setting to %s", defaults.General.Profile.Address)
			c.General.Profile.Address = defaults.General.Profile.Address
		case c.FileLedger.Prefix == "":
			logger.Infof("FileLedger.Prefix unset, setting to %s", defaults.FileLedger.Prefix)
			c.FileLedger.Prefix = defaults.FileLedger.Prefix
		case c.Kafka.Brokers == nil:
			logger.Infof("Kafka.Brokers unset, setting to %v", defaults.Kafka.Brokers)
			c.Kafka.Brokers = defaults.Kafka.Brokers
		case c.Kafka.Retry.Period == 0*time.Second:
			logger.Infof("Kafka.Retry.Period unset, setting to %v", defaults.Kafka.Retry.Period)
			c.Kafka.Retry.Period = defaults.Kafka.Retry.Period
		case c.Kafka.Retry.Stop == 0*time.Second:
			logger.Infof("Kafka.Retry.Stop unset, setting to %v", defaults.Kafka.Retry.Stop)
			c.Kafka.Retry.Stop = defaults.Kafka.Retry.Stop
		case c.Genesis.OrdererType == "":
			logger.Infof("Genesis.OrdererType unset, setting to %s", defaults.Genesis.OrdererType)
			c.Genesis.OrdererType = defaults.Genesis.OrdererType
		case c.Genesis.BatchTimeout == 0:
			logger.Infof("Genesis.BatchTimeout unset, setting to %s", defaults.Genesis.BatchTimeout)
			c.Genesis.BatchTimeout = defaults.Genesis.BatchTimeout
		case c.Genesis.BatchSize.MaxMessageCount == 0:
			logger.Infof("Genesis.BatchSize.MaxMessageCount unset, setting to %s", defaults.Genesis.BatchSize.MaxMessageCount)
			c.Genesis.BatchSize.MaxMessageCount = defaults.Genesis.BatchSize.MaxMessageCount
		default:
			// A bit hacky, but its type makes it impossible to test for a nil value.
			// This may be overwritten by the Kafka orderer upon instantiation.
			c.Kafka.Version = defaults.Kafka.Version
			return
		}
	}
}

// Load parses the orderer.yaml file and environment, producing a struct suitable for config use
func Load() *TopLevel {
	config := viper.New()

	config.SetConfigName("orderer")
	alternativeCfgPath := os.Getenv("ORDERER_CFG_PATH")
	if alternativeCfgPath != "" {
		logger.Infof("User defined config file path: %s", alternativeCfgPath)
		config.AddConfigPath(alternativeCfgPath) // Path to look for the config file in
	} else {
		config.AddConfigPath("./")
		config.AddConfigPath("../../.")
		config.AddConfigPath("../orderer/")
		config.AddConfigPath("../../orderer/")
		// Path to look for the config file in based on GOPATH
		gopath := os.Getenv("GOPATH")
		for _, p := range filepath.SplitList(gopath) {
			ordererPath := filepath.Join(p, "src/github.com/hyperledger/fabric/orderer/")
			config.AddConfigPath(ordererPath)
		}
	}

	// for environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Error reading %s plugin config: %s", Prefix, err))
	}

	var uconf TopLevel

	err = ExactWithDateUnmarshal(config, &uconf)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling into structure: %s", err))
	}

	uconf.completeInitialization()

	return &uconf
}
