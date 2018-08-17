// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package localconfig

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	bccsp "github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	coreconfig "github.com/hyperledger/fabric/core/config"

	"github.com/Shopify/sarama"
	"github.com/spf13/viper"
)

// Prefix for environment variables.
const Prefix = "ORDERER"

var logger = flogging.MustGetLogger("localconfig")

// TopLevel directly corresponds to the orderer config YAML.
// Note, for non 1-1 mappings, you may append
// something like `mapstructure:"weirdFoRMat"` to
// modify the default mapping, see the "Unmarshal"
// section of https://github.com/spf13/viper for more info.
type TopLevel struct {
	General    General
	FileLedger FileLedger
	RAMLedger  RAMLedger
	Kafka      Kafka
	Debug      Debug
}

// General contains config which should be common among all orderer types.
type General struct {
	LedgerType     string
	ListenAddress  string
	ListenPort     uint16
	TLS            TLS
	Keepalive      Keepalive
	GenesisMethod  string
	GenesisProfile string
	SystemChannel  string
	GenesisFile    string
	Profile        Profile
	LogLevel       string
	LogFormat      string
	LocalMSPDir    string
	LocalMSPID     string
	BCCSP          *bccsp.FactoryOpts
	Authentication Authentication
}

// Keepalive contains configuration for gRPC servers.
type Keepalive struct {
	ServerMinInterval time.Duration
	ServerInterval    time.Duration
	ServerTimeout     time.Duration
}

// TLS contains configuration for TLS connections.
type TLS struct {
	Enabled            bool
	PrivateKey         string
	Certificate        string
	RootCAs            []string
	ClientAuthRequired bool
	ClientRootCAs      []string
}

// SASLPlain contains configuration for SASL/PLAIN authentication
type SASLPlain struct {
	Enabled  bool
	User     string
	Password string
}

// Authentication contains configuration parameters related to authenticating
// client messages.
type Authentication struct {
	TimeWindow time.Duration
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
	Retry     Retry
	Verbose   bool
	Version   sarama.KafkaVersion // TODO Move this to global config
	TLS       TLS
	SASLPlain SASLPlain
	Topic     Topic
}

// Retry contains configuration related to retries and timeouts when the
// connection to the Kafka cluster cannot be established, or when Metadata
// requests needs to be repeated (because the cluster is in the middle of a
// leader election).
type Retry struct {
	ShortInterval   time.Duration
	ShortTotal      time.Duration
	LongInterval    time.Duration
	LongTotal       time.Duration
	NetworkTimeouts NetworkTimeouts
	Metadata        Metadata
	Producer        Producer
	Consumer        Consumer
}

// NetworkTimeouts contains the socket timeouts for network requests to the
// Kafka cluster.
type NetworkTimeouts struct {
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Metadata contains configuration for the metadata requests to the Kafka
// cluster.
type Metadata struct {
	RetryMax     int
	RetryBackoff time.Duration
}

// Producer contains configuration for the producer's retries when failing to
// post a message to a Kafka partition.
type Producer struct {
	RetryMax     int
	RetryBackoff time.Duration
}

// Consumer contains configuration for the consumer's retries when failing to
// read from a Kafa partition.
type Consumer struct {
	RetryBackoff time.Duration
}

// Topic contains the settings to use when creating Kafka topics
type Topic struct {
	ReplicationFactor int16
}

// Debug contains configuration for the orderer's debug parameters.
type Debug struct {
	BroadcastTraceDir string
	DeliverTraceDir   string
}

// Defaults carries the default orderer configuration values.
var Defaults = TopLevel{
	General: General{
		LedgerType:     "file",
		ListenAddress:  "127.0.0.1",
		ListenPort:     7050,
		GenesisMethod:  "provisional",
		GenesisProfile: "SampleSingleMSPSolo",
		SystemChannel:  "test-system-channel-name",
		GenesisFile:    "genesisblock",
		Profile: Profile{
			Enabled: false,
			Address: "0.0.0.0:6060",
		},
		LogLevel:    "INFO",
		LogFormat:   "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}",
		LocalMSPDir: "msp",
		LocalMSPID:  "SampleOrg",
		BCCSP:       bccsp.GetDefaultOpts(),
		Authentication: Authentication{
			TimeWindow: time.Duration(15 * time.Minute),
		},
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
			ShortInterval: 1 * time.Minute,
			ShortTotal:    10 * time.Minute,
			LongInterval:  10 * time.Minute,
			LongTotal:     12 * time.Hour,
			NetworkTimeouts: NetworkTimeouts{
				DialTimeout:  30 * time.Second,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			Metadata: Metadata{
				RetryBackoff: 250 * time.Millisecond,
				RetryMax:     3,
			},
			Producer: Producer{
				RetryBackoff: 100 * time.Millisecond,
				RetryMax:     3,
			},
			Consumer: Consumer{
				RetryBackoff: 2 * time.Second,
			},
		},
		Verbose: false,
		Version: sarama.V0_10_2_0,
		TLS: TLS{
			Enabled: false,
		},
		Topic: Topic{
			ReplicationFactor: 3,
		},
	},
	Debug: Debug{
		BroadcastTraceDir: "",
		DeliverTraceDir:   "",
	},
}

// Load parses the orderer YAML file and environment, producing
// a struct suitable for config use, returning error on failure.
func Load() (*TopLevel, error) {
	config := viper.New()
	coreconfig.InitViper(config, "orderer")
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	if err := config.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("Error reading configuration: %s", err)
	}

	var uconf TopLevel
	if err := viperutil.EnhancedExactUnmarshal(config, &uconf); err != nil {
		return nil, fmt.Errorf("Error unmarshaling config into struct: %s", err)
	}

	uconf.completeInitialization(filepath.Dir(config.ConfigFileUsed()))
	return &uconf, nil
}

func (c *TopLevel) completeInitialization(configDir string) {
	defer func() {
		// Translate any paths
		c.General.TLS.RootCAs = translateCAs(configDir, c.General.TLS.RootCAs)
		c.General.TLS.ClientRootCAs = translateCAs(configDir, c.General.TLS.ClientRootCAs)
		coreconfig.TranslatePathInPlace(configDir, &c.General.TLS.PrivateKey)
		coreconfig.TranslatePathInPlace(configDir, &c.General.TLS.Certificate)
		coreconfig.TranslatePathInPlace(configDir, &c.General.GenesisFile)
		coreconfig.TranslatePathInPlace(configDir, &c.General.LocalMSPDir)
	}()

	for {
		switch {
		case c.General.LedgerType == "":
			logger.Infof("General.LedgerType unset, setting to %s", Defaults.General.LedgerType)
			c.General.LedgerType = Defaults.General.LedgerType

		case c.General.ListenAddress == "":
			logger.Infof("General.ListenAddress unset, setting to %s", Defaults.General.ListenAddress)
			c.General.ListenAddress = Defaults.General.ListenAddress
		case c.General.ListenPort == 0:
			logger.Infof("General.ListenPort unset, setting to %v", Defaults.General.ListenPort)
			c.General.ListenPort = Defaults.General.ListenPort

		case c.General.LogLevel == "":
			logger.Infof("General.LogLevel unset, setting to %s", Defaults.General.LogLevel)
			c.General.LogLevel = Defaults.General.LogLevel
		case c.General.LogFormat == "":
			logger.Infof("General.LogFormat unset, setting to %s", Defaults.General.LogFormat)
			c.General.LogFormat = Defaults.General.LogFormat

		case c.General.GenesisMethod == "":
			c.General.GenesisMethod = Defaults.General.GenesisMethod
		case c.General.GenesisFile == "":
			c.General.GenesisFile = Defaults.General.GenesisFile
		case c.General.GenesisProfile == "":
			c.General.GenesisProfile = Defaults.General.GenesisProfile
		case c.General.SystemChannel == "":
			c.General.SystemChannel = Defaults.General.SystemChannel

		case c.Kafka.TLS.Enabled && c.Kafka.TLS.Certificate == "":
			logger.Panicf("General.Kafka.TLS.Certificate must be set if General.Kafka.TLS.Enabled is set to true.")
		case c.Kafka.TLS.Enabled && c.Kafka.TLS.PrivateKey == "":
			logger.Panicf("General.Kafka.TLS.PrivateKey must be set if General.Kafka.TLS.Enabled is set to true.")
		case c.Kafka.TLS.Enabled && c.Kafka.TLS.RootCAs == nil:
			logger.Panicf("General.Kafka.TLS.CertificatePool must be set if General.Kafka.TLS.Enabled is set to true.")

		case c.Kafka.SASLPlain.Enabled && c.Kafka.SASLPlain.User == "":
			logger.Panic("General.Kafka.SASLPlain.User must be set if General.Kafka.SASLPlain.Enabled is set to true.")
		case c.Kafka.SASLPlain.Enabled && c.Kafka.SASLPlain.Password == "":
			logger.Panic("General.Kafka.SASLPlain.Password must be set if General.Kafka.SASLPlain.Enabled is set to true.")

		case c.General.Profile.Enabled && c.General.Profile.Address == "":
			logger.Infof("Profiling enabled and General.Profile.Address unset, setting to %s", Defaults.General.Profile.Address)
			c.General.Profile.Address = Defaults.General.Profile.Address

		case c.General.LocalMSPDir == "":
			logger.Infof("General.LocalMSPDir unset, setting to %s", Defaults.General.LocalMSPDir)
			c.General.LocalMSPDir = Defaults.General.LocalMSPDir
		case c.General.LocalMSPID == "":
			logger.Infof("General.LocalMSPID unset, setting to %s", Defaults.General.LocalMSPID)
			c.General.LocalMSPID = Defaults.General.LocalMSPID

		case c.General.Authentication.TimeWindow == 0:
			logger.Infof("General.Authentication.TimeWindow unset, setting to %s", Defaults.General.Authentication.TimeWindow)
			c.General.Authentication.TimeWindow = Defaults.General.Authentication.TimeWindow

		case c.FileLedger.Prefix == "":
			logger.Infof("FileLedger.Prefix unset, setting to %s", Defaults.FileLedger.Prefix)
			c.FileLedger.Prefix = Defaults.FileLedger.Prefix

		case c.Kafka.Retry.ShortInterval == 0:
			logger.Infof("Kafka.Retry.ShortInterval unset, setting to %v", Defaults.Kafka.Retry.ShortInterval)
			c.Kafka.Retry.ShortInterval = Defaults.Kafka.Retry.ShortInterval
		case c.Kafka.Retry.ShortTotal == 0:
			logger.Infof("Kafka.Retry.ShortTotal unset, setting to %v", Defaults.Kafka.Retry.ShortTotal)
			c.Kafka.Retry.ShortTotal = Defaults.Kafka.Retry.ShortTotal
		case c.Kafka.Retry.LongInterval == 0:
			logger.Infof("Kafka.Retry.LongInterval unset, setting to %v", Defaults.Kafka.Retry.LongInterval)
			c.Kafka.Retry.LongInterval = Defaults.Kafka.Retry.LongInterval
		case c.Kafka.Retry.LongTotal == 0:
			logger.Infof("Kafka.Retry.LongTotal unset, setting to %v", Defaults.Kafka.Retry.LongTotal)
			c.Kafka.Retry.LongTotal = Defaults.Kafka.Retry.LongTotal

		case c.Kafka.Retry.NetworkTimeouts.DialTimeout == 0:
			logger.Infof("Kafka.Retry.NetworkTimeouts.DialTimeout unset, setting to %v", Defaults.Kafka.Retry.NetworkTimeouts.DialTimeout)
			c.Kafka.Retry.NetworkTimeouts.DialTimeout = Defaults.Kafka.Retry.NetworkTimeouts.DialTimeout
		case c.Kafka.Retry.NetworkTimeouts.ReadTimeout == 0:
			logger.Infof("Kafka.Retry.NetworkTimeouts.ReadTimeout unset, setting to %v", Defaults.Kafka.Retry.NetworkTimeouts.ReadTimeout)
			c.Kafka.Retry.NetworkTimeouts.ReadTimeout = Defaults.Kafka.Retry.NetworkTimeouts.ReadTimeout
		case c.Kafka.Retry.NetworkTimeouts.WriteTimeout == 0:
			logger.Infof("Kafka.Retry.NetworkTimeouts.WriteTimeout unset, setting to %v", Defaults.Kafka.Retry.NetworkTimeouts.WriteTimeout)
			c.Kafka.Retry.NetworkTimeouts.WriteTimeout = Defaults.Kafka.Retry.NetworkTimeouts.WriteTimeout

		case c.Kafka.Retry.Metadata.RetryBackoff == 0:
			logger.Infof("Kafka.Retry.Metadata.RetryBackoff unset, setting to %v", Defaults.Kafka.Retry.Metadata.RetryBackoff)
			c.Kafka.Retry.Metadata.RetryBackoff = Defaults.Kafka.Retry.Metadata.RetryBackoff
		case c.Kafka.Retry.Metadata.RetryMax == 0:
			logger.Infof("Kafka.Retry.Metadata.RetryMax unset, setting to %v", Defaults.Kafka.Retry.Metadata.RetryMax)
			c.Kafka.Retry.Metadata.RetryMax = Defaults.Kafka.Retry.Metadata.RetryMax

		case c.Kafka.Retry.Producer.RetryBackoff == 0:
			logger.Infof("Kafka.Retry.Producer.RetryBackoff unset, setting to %v", Defaults.Kafka.Retry.Producer.RetryBackoff)
			c.Kafka.Retry.Producer.RetryBackoff = Defaults.Kafka.Retry.Producer.RetryBackoff
		case c.Kafka.Retry.Producer.RetryMax == 0:
			logger.Infof("Kafka.Retry.Producer.RetryMax unset, setting to %v", Defaults.Kafka.Retry.Producer.RetryMax)
			c.Kafka.Retry.Producer.RetryMax = Defaults.Kafka.Retry.Producer.RetryMax

		case c.Kafka.Retry.Consumer.RetryBackoff == 0:
			logger.Infof("Kafka.Retry.Consumer.RetryBackoff unset, setting to %v", Defaults.Kafka.Retry.Consumer.RetryBackoff)
			c.Kafka.Retry.Consumer.RetryBackoff = Defaults.Kafka.Retry.Consumer.RetryBackoff

		case c.Kafka.Version == sarama.KafkaVersion{}:
			logger.Infof("Kafka.Version unset, setting to %v", Defaults.Kafka.Version)
			c.Kafka.Version = Defaults.Kafka.Version

		default:
			return
		}
	}
}

func translateCAs(configDir string, certificateAuthorities []string) []string {
	var results []string
	for _, ca := range certificateAuthorities {
		result := coreconfig.TranslatePath(configDir, ca)
		results = append(results, result)
	}
	return results
}
