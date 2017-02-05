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
	TLS           TLS
	GenesisMethod string
	GenesisFile   string
	Profile       Profile
	LogLevel      string
	LocalMSPDir   string
}

//TLS contains config used to configure TLS
type TLS struct {
	Enabled           bool
	PrivateKey        string
	Certificate       string
	RootCAs           []string
	ClientAuthEnabled bool
	ClientRootCAs     []string
}

// Genesis contains config which is used by the provisional bootstrapper
type Genesis struct {
	OrdererType  string
	BatchTimeout time.Duration
	BatchSize    BatchSize
	SbftShared   SbftShared
}

// BatchSize contains configuration affecting the size of batches
type BatchSize struct {
	MaxMessageCount   uint32
	AbsoluteMaxBytes  uint32
	PreferredMaxBytes uint32
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
	TLS     TLS
}

// SbftLocal contains config for the SBFT peer/replica
type SbftLocal struct {
	PeerCommAddr string
	CertFile     string
	KeyFile      string
	DataDir      string
}

// SbftShared contains config for the SBFT network
type SbftShared struct {
	N                  uint64
	F                  uint64
	RequestTimeoutNsec uint64
	Peers              map[string]string // Address to Cert mapping
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
	SbftLocal  SbftLocal
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
		LogLevel:    "INFO",
		LocalMSPDir: "../msp/sampleconfig/",
	},
	RAMLedger: RAMLedger{
		HistorySize: 10000,
	},
	FileLedger: FileLedger{
		Location: "",
		Prefix:   "hyperledger-fabric-ordererledger",
	},
	Kafka: Kafka{
		Brokers: []string{"127.0.0.1:9092"},
		Retry: Retry{
			Period: 3 * time.Second,
			Stop:   60 * time.Second,
		},
		Verbose: false,
		Version: sarama.V0_9_0_1,
		TLS: TLS{
			Enabled: false,
		},
	},
	Genesis: Genesis{
		OrdererType:  "solo",
		BatchTimeout: 10 * time.Second,
		BatchSize: BatchSize{
			MaxMessageCount:   10,
			AbsoluteMaxBytes:  100000000,
			PreferredMaxBytes: 512 * 1024,
		},
		SbftShared: SbftShared{
			N:                  1,
			F:                  0,
			RequestTimeoutNsec: uint64(time.Second.Nanoseconds()),
			Peers:              map[string]string{":6101": "sbft/testdata/cert1.pem"},
		},
	},
	SbftLocal: SbftLocal{
		PeerCommAddr: ":6101",
		CertFile:     "sbft/testdata/cert1.pem",
		KeyFile:      "sbft/testdata/key.pem",
		DataDir:      "/tmp",
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
		case c.Kafka.TLS.Enabled && c.Kafka.TLS.Certificate == "":
			logger.Panicf("General.Kafka.TLS.Certificate must be set if General.Kafka.TLS.Enabled is set to true.")
		case c.Kafka.TLS.Enabled && c.Kafka.TLS.PrivateKey == "":
			logger.Panicf("General.Kafka.TLS.PrivateKey must be set if General.Kafka.TLS.Enabled is set to true.")
		case c.Kafka.TLS.Enabled && c.Kafka.TLS.RootCAs == nil:
			logger.Panicf("General.Kafka.TLS.CertificatePool must be set if General.Kafka.TLS.Enabled is set to true.")
		case c.General.Profile.Enabled && (c.General.Profile.Address == ""):
			logger.Infof("Profiling enabled and General.Profile.Address unset, setting to %s", defaults.General.Profile.Address)
			c.General.Profile.Address = defaults.General.Profile.Address
		case c.General.LocalMSPDir == "":
			logger.Infof("General.LocalMSPDir unset, setting to %s", defaults.General.LocalMSPDir)
			// Note, this is a bit of a weird one, the orderer may set the ORDERER_CFG_PATH after
			// the file is initialized, so we cannot initialize this in the structure, so we
			// deference the env portion here
			c.General.LocalMSPDir = filepath.Join(os.Getenv("ORDERER_CFG_PATH"), defaults.General.LocalMSPDir)
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
		case c.Genesis.BatchSize.AbsoluteMaxBytes == 0:
			logger.Infof("Genesis.BatchSize.AbsoluteMaxBytes unset, setting to %s", defaults.Genesis.BatchSize.AbsoluteMaxBytes)
			c.Genesis.BatchSize.AbsoluteMaxBytes = defaults.Genesis.BatchSize.AbsoluteMaxBytes
		case c.Genesis.BatchSize.PreferredMaxBytes == 0:
			logger.Infof("Genesis.BatchSize.PreferredMaxBytes unset, setting to %s", defaults.Genesis.BatchSize.PreferredMaxBytes)
			c.Genesis.BatchSize.PreferredMaxBytes = defaults.Genesis.BatchSize.PreferredMaxBytes
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
	cfgPath := os.Getenv("ORDERER_CFG_PATH")
	if cfgPath == "" {
		logger.Infof("No orderer cfg path set, assuming development environment, deriving from go path")
		// Path to look for the config file in based on GOPATH
		gopath := os.Getenv("GOPATH")
		for _, p := range filepath.SplitList(gopath) {
			ordererPath := filepath.Join(p, "src/github.com/hyperledger/fabric/orderer/")
			if _, err := os.Stat(filepath.Join(ordererPath, "orderer.yaml")); err != nil {
				// The yaml file does not exist in this component of the go src
				continue
			}
			cfgPath = ordererPath
		}
		if cfgPath == "" {
			logger.Fatalf("Could not find orderer.yaml, try setting ORDERER_CFG_PATH or GOPATH correctly")
		}
		logger.Infof("Setting ORDERER_CFG_PATH to: %s", cfgPath)
		os.Setenv("ORDERER_CFG_PATH", cfgPath)
	}
	config.AddConfigPath(cfgPath) // Path to look for the config file in

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
