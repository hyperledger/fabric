// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package localconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	bccsp "github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	coreconfig "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/internal/pkg/comm"
)

var logger = flogging.MustGetLogger("localconfig")

// TopLevel directly corresponds to the orderer config YAML.
type TopLevel struct {
	General              General
	FileLedger           FileLedger
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
	GenesisMethod     string // Deprecated: For compatibility only, will be replaced by BootstrapMethod
	GenesisFile       string // Deprecated: For compatibility only, will be replaced by BootstrapFile
	BootstrapMethod   string // Deprecated: System channel is no longer supported.
	BootstrapFile     string // Deprecated: System channel is no longer supported.
	Profile           Profile
	LocalMSPDir       string
	LocalMSPID        string
	BCCSP             *bccsp.FactoryOpts
	Authentication    Authentication
	MaxRecvMsgSize    int32
	MaxSendMsgSize    int32
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
	Enabled            bool // Deprecated: always overridden to 'true'
	MaxRequestBodySize uint32
}

// Defaults carries the default orderer configuration values.
var Defaults = TopLevel{
	General: General{
		ListenAddress:   "127.0.0.1",
		ListenPort:      7050,
		BootstrapMethod: "none",
		Profile: Profile{
			Enabled: false,
			Address: "0.0.0.0:6060",
		},
		Cluster: Cluster{
			ReplicationMaxRetries:                12,
			RPCTimeout:                           time.Second * 7,
			DialTimeout:                          time.Second * 5,
			ReplicationBufferSize:                20971520,
			SendBufferSize:                       100,
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
		MaxRecvMsgSize: comm.DefaultMaxRecvMsgSize,
		MaxSendMsgSize: comm.DefaultMaxSendMsgSize,
	},
	FileLedger: FileLedger{
		Location: "/var/hyperledger/production/orderer",
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
		Enabled:            true,
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

		case !c.ChannelParticipation.Enabled:
			logger.Info("General.ChannelParticipation.Enabled was set to false, setting to true")
			c.ChannelParticipation.Enabled = true

		case c.Admin.TLS.Enabled && !c.Admin.TLS.ClientAuthRequired:
			logger.Panic("Admin.TLS.ClientAuthRequired must be set to true if Admin.TLS.Enabled is set to true")

		case c.General.MaxRecvMsgSize == 0:
			logger.Infof("General.MaxRecvMsgSize is unset, setting to %v", Defaults.General.MaxRecvMsgSize)
			c.General.MaxRecvMsgSize = Defaults.General.MaxRecvMsgSize
		case c.General.MaxSendMsgSize == 0:
			logger.Infof("General.MaxSendMsgSize is unset, setting to %v", Defaults.General.MaxSendMsgSize)
			c.General.MaxSendMsgSize = Defaults.General.MaxSendMsgSize
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

// Consensus indicates the orderer type.
type Consensus struct {
	Type string `yaml:"type,omitempty"`
}
