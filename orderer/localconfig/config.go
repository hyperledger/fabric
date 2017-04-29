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
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"

	"github.com/Shopify/sarama"
	"github.com/op/go-logging"
	"github.com/spf13/viper"

	cf "github.com/hyperledger/fabric/core/config"

	"path/filepath"

	bccsp "github.com/hyperledger/fabric/bccsp/factory"
)

const (
	pkgLogID = "orderer/localconfig"

	// Prefix identifies the prefix for the orderer-related ENV vars.
	Prefix = "ORDERER"
)

var (
	logger *logging.Logger

	configName string
)

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
	flogging.SetModuleLevel(pkgLogID, "error")

	configName = strings.ToLower(Prefix)
}

// TopLevel directly corresponds to the orderer config YAML.
// Note, for non 1-1 mappings, you may append
// something like `mapstructure:"weirdFoRMat"` to
// modify the default mapping, see the "Unmarshal"
// section of https://github.com/spf13/viper for more info
type TopLevel struct {
	General    General
	FileLedger FileLedger
	RAMLedger  RAMLedger
	Kafka      Kafka
	Genesis    Genesis
	SbftLocal  SbftLocal
}

// General contains config which should be common among all orderer types.
type General struct {
	LedgerType     string
	ListenAddress  string
	ListenPort     uint16
	TLS            TLS
	GenesisMethod  string
	GenesisProfile string
	GenesisFile    string
	Profile        Profile
	LogLevel       string
	LocalMSPDir    string
	LocalMSPID     string
	BCCSP          *bccsp.FactoryOpts
}

// TLS contains config for TLS connections.
type TLS struct {
	Enabled           bool
	PrivateKey        string
	Certificate       string
	RootCAs           []string
	ClientAuthEnabled bool
	ClientRootCAs     []string
}

// Profile contains configuration for Go pprof profiling.
type Profile struct {
	Enabled bool
	Address string
}

// FileLedger contains configuration for the file-based ledger.
type FileLedger struct {
	Location string
	Prefix   string
}

// RAMLedger contains configuration for the RAM ledger.
type RAMLedger struct {
	HistorySize uint
}

// Kafka contains configuration for the Kafka-based orderer.
type Kafka struct {
	Retry   Retry
	Verbose bool
	Version sarama.KafkaVersion // TODO Move this to global config
	TLS     TLS
}

// Retry contains config for the reconnection attempts to the Kafka brokers.
type Retry struct {
	Period time.Duration
	Stop   time.Duration
}

// Genesis is a deprecated structure which was used to put
// values into the genesis block, but this is now handled elsewhere.
// SBFT did not reference these values via the genesis block however
// so it is being left here for backwards compatibility purposes.
type Genesis struct {
	DeprecatedBatchTimeout time.Duration
	DeprecatedBatchSize    uint32
	SbftShared             SbftShared
}

// SbftLocal contains configuration for the SBFT peer/replica.
type SbftLocal struct {
	PeerCommAddr string
	CertFile     string
	KeyFile      string
	DataDir      string
}

// SbftShared contains config for the SBFT network.
type SbftShared struct {
	N                  uint64
	F                  uint64
	RequestTimeoutNsec uint64
	Peers              map[string]string // Address to Cert mapping
}

var defaults = TopLevel{
	General: General{
		LedgerType:     "file",
		ListenAddress:  "127.0.0.1",
		ListenPort:     7050,
		GenesisMethod:  "provisional",
		GenesisProfile: "SampleSingleMSPSolo",
		GenesisFile:    "genesisblock",
		Profile: Profile{
			Enabled: false,
			Address: "0.0.0.0:6060",
		},
		LogLevel:    "INFO",
		LocalMSPDir: "msp",
		LocalMSPID:  "DEFAULT",
		BCCSP:       &bccsp.DefaultOpts,
	},
	RAMLedger: RAMLedger{
		HistorySize: 10000,
	},
	FileLedger: FileLedger{
		Location: "/var/hyperledger/production/orderer",
		Prefix:   "hyperledger-fabric-ordererledger",
	},
	Kafka: Kafka{
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

// Load parses the orderer.yaml file and environment, producing a struct suitable for config use
func Load() *TopLevel {
	config := viper.New()
	cf.InitViper(config, configName)

	// for environment variables
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	err := config.ReadInConfig()
	if err != nil {
		logger.Panic("Error reading configuration:", err)
	}

	var uconf TopLevel
	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		logger.Panic("Error unmarshaling config into struct:", err)
	}

	uconf.completeInitialization(filepath.Dir(config.ConfigFileUsed()))

	return &uconf
}

func (c *TopLevel) completeInitialization(configDir string) {
	defer func() {
		// Translate any paths
		c.General.TLS.RootCAs = translateCAs(configDir, c.General.TLS.RootCAs)
		c.General.TLS.ClientRootCAs = translateCAs(configDir, c.General.TLS.ClientRootCAs)
		cf.TranslatePathInPlace(configDir, &c.General.TLS.PrivateKey)
		cf.TranslatePathInPlace(configDir, &c.General.TLS.Certificate)
		cf.TranslatePathInPlace(configDir, &c.General.GenesisFile)
		cf.TranslatePathInPlace(configDir, &c.General.LocalMSPDir)
	}()

	for {
		switch {
		case c.General.LedgerType == "":
			logger.Infof("General.LedgerType unset, setting to %s", defaults.General.LedgerType)
			c.General.LedgerType = defaults.General.LedgerType
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
		case c.General.GenesisProfile == "":
			c.General.GenesisProfile = defaults.General.GenesisProfile
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
			c.General.LocalMSPDir = defaults.General.LocalMSPDir
		case c.General.LocalMSPID == "":
			logger.Infof("General.LocalMSPID unset, setting to %s", defaults.General.LocalMSPID)
			c.General.LocalMSPID = defaults.General.LocalMSPID
		case c.FileLedger.Prefix == "":
			logger.Infof("FileLedger.Prefix unset, setting to %s", defaults.FileLedger.Prefix)
			c.FileLedger.Prefix = defaults.FileLedger.Prefix
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

func translateCAs(configDir string, certificateAuthorities []string) []string {
	results := make([]string, 0)
	for _, ca := range certificateAuthorities {
		result := cf.TranslatePath(configDir, ca)
		results = append(results, result)
	}
	return results
}
