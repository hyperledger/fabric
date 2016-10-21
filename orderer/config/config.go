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
	logging.SetLevel(logging.DEBUG, "")
}

// Prefix is the default config prefix for the orderer
const Prefix string = "ORDERER"

// General contains config which should be common among all orderer types
type General struct {
	OrdererType   string
	LedgerType    string
	BatchTimeout  time.Duration
	BatchSize     uint
	QueueSize     uint
	MaxWindowSize uint
	ListenAddress string
	ListenPort    uint16
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
	Brokers     []string
	Topic       string
	PartitionID int32
	Retry       Retry
	Version     sarama.KafkaVersion // TODO For now set this in code
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
}

var defaults = TopLevel{
	General: General{
		OrdererType:   "solo",
		LedgerType:    "ram",
		BatchTimeout:  10 * time.Second,
		BatchSize:     10,
		QueueSize:     1000,
		MaxWindowSize: 1000,
		ListenAddress: "127.0.0.1",
		ListenPort:    5151,
	},
	RAMLedger: RAMLedger{
		HistorySize: 10000,
	},
	FileLedger: FileLedger{
		Location: "",
		Prefix:   "hyperledger-fabric-rawledger",
	},
	Kafka: Kafka{
		Brokers:     []string{"127.0.0.1:9092"},
		Topic:       "test",
		PartitionID: 0,
		Version:     sarama.V0_9_0_1,
		Retry: Retry{
			Period: 3 * time.Second,
			Stop:   60 * time.Second,
		},
	},
}

func (c *TopLevel) completeInitialization() {
	defer logger.Infof("Validated configuration to: %+v", c)

	for {
		switch {
		case c.General.OrdererType == "":
			logger.Infof("General.OrdererType unset, setting to %s", defaults.General.OrdererType)
			c.General.OrdererType = defaults.General.OrdererType
		case c.General.LedgerType == "":
			logger.Infof("General.LedgerType unset, setting to %s", defaults.General.LedgerType)
			c.General.LedgerType = defaults.General.LedgerType
		case c.General.BatchTimeout == 0:
			logger.Infof("General.BatchTimeout unset, setting to %s", defaults.General.BatchTimeout)
			c.General.BatchTimeout = defaults.General.BatchTimeout
		case c.General.BatchSize == 0:
			logger.Infof("General.BatchSize unset, setting to %s", defaults.General.BatchSize)
			c.General.BatchSize = defaults.General.BatchSize
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
		case c.FileLedger.Prefix == "":
			logger.Infof("FileLedger.Prefix unset, setting to %s", defaults.FileLedger.Prefix)
			c.FileLedger.Prefix = defaults.FileLedger.Prefix
		case c.Kafka.Brokers == nil:
			logger.Infof("Kafka.Brokers unset, setting to %v", defaults.Kafka.Brokers)
			c.Kafka.Brokers = defaults.Kafka.Brokers
		case c.Kafka.Topic == "":
			logger.Infof("Kafka.Topic unset, setting to %v", defaults.Kafka.Topic)
			c.Kafka.Topic = defaults.Kafka.Topic
		case c.Kafka.Retry.Period == 0*time.Second:
			logger.Infof("Kafka.Retry.Period unset, setting to %v", defaults.Kafka.Retry.Period)
			c.Kafka.Retry.Period = defaults.Kafka.Retry.Period
		case c.Kafka.Retry.Stop == 0*time.Second:
			logger.Infof("Kafka.Retry.Stop unset, setting to %v", defaults.Kafka.Retry.Stop)
			c.Kafka.Retry.Stop = defaults.Kafka.Retry.Stop
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

	// for environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	config.SetConfigName("orderer")
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
