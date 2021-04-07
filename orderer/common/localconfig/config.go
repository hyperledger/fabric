// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package localconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	bccsp "github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	coreconfig "github.com/hyperledger/fabric/core/config"
)

var logger = flogging.MustGetLogger("localconfig")

// TopLevel directly corresponds to the orderer config YAML.
type TopLevel struct {
	General              General
	FileLedger           FileLedger
	Kafka                Kafka
	Debug                Debug
	Consensus            interface{}
	Operations           Operations
	Metrics              Metrics
	ChannelParticipation ChannelParticipation
	Admin                Admin
}

// General contains config which should be common among all orderer types.
type General struct {
	ListenAddress     string
	ListenPort        uint16
	TLS               TLS
	Cluster           Cluster
	Keepalive         Keepalive
	ConnectionTimeout time.Duration
	GenesisMethod     string // For compatibility only, will be replaced by BootstrapMethod
	GenesisFile       string // For compatibility only, will be replaced by BootstrapFile
	BootstrapMethod   string
	BootstrapFile     string
	Profile           Profile
	LocalMSPDir       string
	LocalMSPID        string
	BCCSP             *bccsp.FactoryOpts
	Authentication    Authentication
}

type Cluster struct {
	ListenAddress                        string
	ListenPort                           uint16
	ServerCertificate                    string
	ServerPrivateKey                     string
	ClientCertificate                    string
	ClientPrivateKey                     string
	RootCAs                              []string
	DialTimeout                          time.Duration
	RPCTimeout                           time.Duration
	ReplicationBufferSize                int
	ReplicationPullTimeout               time.Duration
	ReplicationRetryTimeout              time.Duration
	ReplicationBackgroundRefreshInterval time.Duration
	ReplicationMaxRetries                int
	SendBufferSize                       int
	CertExpirationWarningThreshold       time.Duration
	TLSHandshakeTimeShift                time.Duration
}

// Keepalive contains configuration for gRPC servers.
type Keepalive struct {
	ServerMinInterval time.Duration
	ServerInterval    time.Duration
	ServerTimeout     time.Duration
}

// TLS contains configuration for TLS connections.
type TLS struct {
	Enabled               bool
	PrivateKey            string
	Certificate           string
	RootCAs               []string
	ClientAuthRequired    bool
	ClientRootCAs         []string
	TLSHandshakeTimeShift time.Duration
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
	TimeWindow         time.Duration
	NoExpirationChecks bool
}

// Profile contains configuration for Go pprof profiling.
type Profile struct {
	Enabled bool
	Address string
}

// FileLedger contains configuration for the file-based ledger.
type FileLedger struct {
	Location string
	Prefix   string // For compatibility only. This setting is no longer supported.
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

// Operations configures the operations endpoint for the orderer.
type Operations struct {
	ListenAddress string
	TLS           TLS
}

// Metrics configures the metrics provider for the orderer.
type Metrics struct {
	Provider string
	Statsd   Statsd
}

// Statsd provides the configuration required to emit statsd metrics from the orderer.
type Statsd struct {
	Network       string
	Address       string
	WriteInterval time.Duration
	Prefix        string
}

// Admin configures the admin endpoint for the orderer.
type Admin struct {
	ListenAddress string
	TLS           TLS
}

// ChannelParticipation provides the channel participation API configuration for the orderer.
// Channel participation uses the same ListenAddress and TLS settings of the Operations service.
type ChannelParticipation struct {
	Enabled            bool
	MaxRequestBodySize uint32
}

// Defaults carries the default orderer configuration values.
var Defaults = TopLevel{
	General: General{
		ListenAddress:   "127.0.0.1",
		ListenPort:      7050,
		BootstrapMethod: "file",
		BootstrapFile:   "genesisblock",
		Profile: Profile{
			Enabled: false,
			Address: "0.0.0.0:6060",
		},
		Cluster: Cluster{
			ReplicationMaxRetries:                12,
			RPCTimeout:                           time.Second * 7,
			DialTimeout:                          time.Second * 5,
			ReplicationBufferSize:                20971520,
			SendBufferSize:                       10,
			ReplicationBackgroundRefreshInterval: time.Minute * 5,
			ReplicationRetryTimeout:              time.Second * 5,
			ReplicationPullTimeout:               time.Second * 5,
			CertExpirationWarningThreshold:       time.Hour * 24 * 7,
		},
		LocalMSPDir: "msp",
		LocalMSPID:  "SampleOrg",
		BCCSP:       bccsp.GetDefaultOpts(),
		Authentication: Authentication{
			TimeWindow: time.Duration(15 * time.Minute),
		},
	},
	FileLedger: FileLedger{
		Location: "/var/hyperledger/production/orderer",
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
	Operations: Operations{
		ListenAddress: "127.0.0.1:0",
	},
	Metrics: Metrics{
		Provider: "disabled",
	},
	ChannelParticipation: ChannelParticipation{
		Enabled:            false,
		MaxRequestBodySize: 1024 * 1024,
	},
	Admin: Admin{
		ListenAddress: "127.0.0.1:0",
	},
}

// Load parses the orderer YAML file and environment, producing
// a struct suitable for config use, returning error on failure.
func Load() (*TopLevel, error) {
	return cache.load()
}

// configCache stores marshalled bytes of config structures that produced from
// EnhancedExactUnmarshal. Cache key is the path of the configuration file that was used.
type configCache struct {
	mutex sync.Mutex
	cache map[string][]byte
}

var cache = &configCache{}

// Load will load the configuration and cache it on the first call; subsequent
// calls will return a clone of the configuration that was previously loaded.
func (c *configCache) load() (*TopLevel, error) {
	var uconf TopLevel

	config := viperutil.New()
	config.SetConfigName("orderer")

	if err := config.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("Error reading configuration: %s", err)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	serializedConf, ok := c.cache[config.ConfigFileUsed()]
	if !ok {
		err := config.EnhancedExactUnmarshal(&uconf)
		if err != nil {
			return nil, fmt.Errorf("Error unmarshalling config into struct: %s", err)
		}

		serializedConf, err = json.Marshal(uconf)
		if err != nil {
			return nil, err
		}

		if c.cache == nil {
			c.cache = map[string][]byte{}
		}
		c.cache[config.ConfigFileUsed()] = serializedConf
	}

	err := json.Unmarshal(serializedConf, &uconf)
	if err != nil {
		return nil, err
	}
	uconf.completeInitialization(filepath.Dir(config.ConfigFileUsed()))

	return &uconf, nil
}

func (c *TopLevel) completeInitialization(configDir string) {
	defer func() {
		// Translate any paths for cluster TLS configuration if applicable
		if c.General.Cluster.ClientPrivateKey != "" {
			coreconfig.TranslatePathInPlace(configDir, &c.General.Cluster.ClientPrivateKey)
		}
		if c.General.Cluster.ClientCertificate != "" {
			coreconfig.TranslatePathInPlace(configDir, &c.General.Cluster.ClientCertificate)
		}
		c.General.Cluster.RootCAs = translateCAs(configDir, c.General.Cluster.RootCAs)
		// Translate any paths for general TLS configuration
		c.General.TLS.RootCAs = translateCAs(configDir, c.General.TLS.RootCAs)
		c.General.TLS.ClientRootCAs = translateCAs(configDir, c.General.TLS.ClientRootCAs)
		coreconfig.TranslatePathInPlace(configDir, &c.General.TLS.PrivateKey)
		coreconfig.TranslatePathInPlace(configDir, &c.General.TLS.Certificate)
		coreconfig.TranslatePathInPlace(configDir, &c.General.BootstrapFile)
		coreconfig.TranslatePathInPlace(configDir, &c.General.LocalMSPDir)
		// Translate file ledger location
		coreconfig.TranslatePathInPlace(configDir, &c.FileLedger.Location)
	}()

	for {
		switch {
		case c.General.ListenAddress == "":
			logger.Infof("General.ListenAddress unset, setting to %s", Defaults.General.ListenAddress)
			c.General.ListenAddress = Defaults.General.ListenAddress
		case c.General.ListenPort == 0:
			logger.Infof("General.ListenPort unset, setting to %v", Defaults.General.ListenPort)
			c.General.ListenPort = Defaults.General.ListenPort
		case c.General.BootstrapMethod == "":
			if c.General.GenesisMethod != "" {
				// This is to keep the compatibility with old config file that uses genesismethod
				logger.Warn("General.GenesisMethod should be replaced by General.BootstrapMethod")
				c.General.BootstrapMethod = c.General.GenesisMethod
			} else {
				c.General.BootstrapMethod = Defaults.General.BootstrapMethod
			}
		case c.General.BootstrapFile == "":
			if c.General.GenesisFile != "" {
				// This is to keep the compatibility with old config file that uses genesisfile
				logger.Warn("General.GenesisFile should be replaced by General.BootstrapFile")
				c.General.BootstrapFile = c.General.GenesisFile
			} else {
				c.General.BootstrapFile = Defaults.General.BootstrapFile
			}
		case c.General.Cluster.RPCTimeout == 0:
			c.General.Cluster.RPCTimeout = Defaults.General.Cluster.RPCTimeout
		case c.General.Cluster.DialTimeout == 0:
			c.General.Cluster.DialTimeout = Defaults.General.Cluster.DialTimeout
		case c.General.Cluster.ReplicationMaxRetries == 0:
			c.General.Cluster.ReplicationMaxRetries = Defaults.General.Cluster.ReplicationMaxRetries
		case c.General.Cluster.SendBufferSize == 0:
			c.General.Cluster.SendBufferSize = Defaults.General.Cluster.SendBufferSize
		case c.General.Cluster.ReplicationBufferSize == 0:
			c.General.Cluster.ReplicationBufferSize = Defaults.General.Cluster.ReplicationBufferSize
		case c.General.Cluster.ReplicationPullTimeout == 0:
			c.General.Cluster.ReplicationPullTimeout = Defaults.General.Cluster.ReplicationPullTimeout
		case c.General.Cluster.ReplicationRetryTimeout == 0:
			c.General.Cluster.ReplicationRetryTimeout = Defaults.General.Cluster.ReplicationRetryTimeout
		case c.General.Cluster.ReplicationBackgroundRefreshInterval == 0:
			c.General.Cluster.ReplicationBackgroundRefreshInterval = Defaults.General.Cluster.ReplicationBackgroundRefreshInterval
		case c.General.Cluster.CertExpirationWarningThreshold == 0:
			c.General.Cluster.CertExpirationWarningThreshold = Defaults.General.Cluster.CertExpirationWarningThreshold
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

		case c.Admin.TLS.Enabled && !c.Admin.TLS.ClientAuthRequired:
			logger.Panic("Admin.TLS.ClientAuthRequired must be set to true if Admin.TLS.Enabled is set to true")

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
