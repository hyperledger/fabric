/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricconfig

import "time"

type Orderer struct {
	General              *General              `yaml:"General,omitempty"`
	FileLedger           *FileLedger           `yaml:"FileLedger,omitempty"`
	Kafka                *Kafka                `yaml:"Kafka,omitempty"`
	Operations           *OrdererOperations    `yaml:"Operations,omitempty"`
	ChannelParticipation *ChannelParticipation `yaml:"ChannelParticipation,omitempty"`
	Consensus            map[string]string     `yaml:"Consensus,omitempty"`
	Admin                *Admin                `yaml:"Admin,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Admin struct {
	TLS           *OrdererTLS `yaml:"TLS,omitempty"`
	ListenAddress string      `yaml:"ListenAddress,omitempty"`
}

type General struct {
	ListenAddress   string                 `yaml:"ListenAddress,omitempty"`
	ListenPort      uint16                 `yaml:"ListenPort,omitempty"`
	TLS             *OrdererTLS            `yaml:"TLS,omitempty"`
	Keepalive       *OrdererKeepalive      `yaml:"Keepalive,omitempty"`
	BootstrapMethod string                 `yaml:"BootstrapMethod,omitempty"`
	GenesisProfile  string                 `yaml:"GenesisProfile,omitempty"`
	GenesisFile     string                 `yaml:"GenesisFile,omitempty"` // will be replaced by the BootstrapFile
	BootstrapFile   string                 `yaml:"BootstrapFile,omitempty"`
	LocalMSPDir     string                 `yaml:"LocalMSPDir,omitempty"`
	LocalMSPID      string                 `yaml:"LocalMSPID,omitempty"`
	Profile         *OrdererProfile        `yaml:"Profile,omitempty"`
	BCCSP           *BCCSP                 `yaml:"BCCSP,omitempty"`
	Authentication  *OrdererAuthentication `yaml:"Authentication,omitempty"`
	Cluster         *Cluster               `yaml:"Cluster,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Cluster struct {
	ListenAddress                        string        `yaml:"ListenAddress,omitempty"`
	ListenPort                           uint16        `yaml:"ListenPort,omitempty"`
	ServerCertificate                    string        `yaml:"ServerCertificate,omitempty"`
	ServerPrivateKey                     string        `yaml:"ServerPrivateKey,omitempty"`
	ClientCertificate                    string        `yaml:"ClientCertificate,omitempty"`
	ClientPrivateKey                     string        `yaml:"ClientPrivateKey,omitempty"`
	RootCAs                              []string      `yaml:"RootCAs,omitempty"`
	DialTimeout                          time.Duration `yaml:"DialTimeout,omitempty"`
	RPCTimeout                           time.Duration `yaml:"RPCTimeout,omitempty"`
	ReplicationBufferSize                int           `yaml:"ReplicationBufferSize,omitempty"`
	ReplicationPullTimeout               time.Duration `yaml:"ReplicationPullTimeout,omitempty"`
	ReplicationRetryTimeout              time.Duration `yaml:"ReplicationRetryTimeout,omitempty"`
	ReplicationBackgroundRefreshInterval time.Duration `yaml:"ReplicationBackgroundRefreshInterval,omitempty"`
	ReplicationMaxRetries                int           `yaml:"ReplicationMaxRetries,omitempty"`
	SendBufferSize                       int           `yaml:"SendBufferSize,omitempty"`
	CertExpirationWarningThreshold       time.Duration `yaml:"CertExpirationWarningThreshold,omitempty"`
	TLSHandshakeTimeShift                time.Duration `yaml:"TLSHandshakeTimeShift,omitempty"`
}

type OrdererTLS struct {
	Enabled               bool          `yaml:"Enabled"`
	PrivateKey            string        `yaml:"PrivateKey,omitempty"`
	Certificate           string        `yaml:"Certificate,omitempty"`
	RootCAs               []string      `yaml:"RootCAs,omitempty"`
	ClientAuthRequired    bool          `yaml:"ClientAuthRequired"`
	ClientRootCAs         []string      `yaml:"ClientRootCAs,omitempty"`
	TLSHandshakeTimeShift time.Duration `yaml:"TLSHandshakeTimeShift,omitempty"`
}

type OrdererSASLPlain struct {
	Enabled  bool   `yaml:"Enabled"`
	User     string `yaml:"User,omitempty"`
	Password string `yaml:"Password,omitempty"`
}

type OrdererKeepalive struct {
	ServerMinInterval time.Duration `yaml:"ServerMinInterval,omitempty"`
	ServerInterval    time.Duration `yaml:"ServerInterval,omitempty"`
	ServerTimeout     time.Duration `yaml:"ServerTimeout,omitempty"`
}

type OrdererProfile struct {
	Enabled bool   `yaml:"Enabled"`
	Address string `yaml:"Address,omitempty"`
}

type OrdererAuthentication struct {
	TimeWindow time.Duration `yaml:"TimeWindow,omitempty"`
}

type OrdererTopic struct {
	ReplicationFactor int16
}

type FileLedger struct {
	Location string `yaml:"Location,omitempty"`
}

type Kafka struct {
	Retry     *Retry            `yaml:"Retry,omitempty"`
	Verbose   bool              `yaml:"Verbose"`
	TLS       *OrdererTLS       `yaml:"TLS,omitempty"`
	SASLPlain *OrdererSASLPlain `yaml:"SASLPlain,omitempty"`
	Topic     *OrdererTopic     `yaml:"Topic,omitempty"`
}

type Retry struct {
	ShortInterval   time.Duration    `yaml:"ShortInterval,omitempty"`
	ShortTotal      time.Duration    `yaml:"ShortTotal,omitempty"`
	LongInterval    time.Duration    `yaml:"LongInterval,omitempty"`
	LongTotal       time.Duration    `yaml:"LongTotal,omitempty"`
	NetworkTimeouts *NetworkTimeouts `yaml:"NetworkTimeouts,omitempty"`
	Metadata        *Backoff         `yaml:"Metadata,omitempty"`
	Producer        *Backoff         `yaml:"Producer,omitempty"`
	Consumer        *Backoff         `yaml:"Consumer,omitempty"`
}

type NetworkTimeouts struct {
	DialTimeout  time.Duration `yaml:"DialTimeout,omitempty"`
	ReadTimeout  time.Duration `yaml:"ReadTimeout,omitempty"`
	WriteTimeout time.Duration `yaml:"WriteTimeout,omitempty"`
}

type Backoff struct {
	RetryBackoff time.Duration `yaml:"RetryBackoff,omitempty"`
	RetryMax     int           `yaml:"RetryMax,omitempty"`
}

type OrdererOperations struct {
	ListenAddress string          `yaml:"ListenAddress,omitempty"`
	Metrics       *OrdererMetrics `yaml:"Metrics,omitempty"`
	TLS           *OrdererTLS     `yaml:"TLS"`
}

type OrdererMetrics struct {
	Provider string         `yaml:"Provider"`
	Statsd   *OrdererStatsd `yaml:"Statsd,omitempty"`
}

type OrdererStatsd struct {
	Network       string        `yaml:"Network,omitempty"`
	Address       string        `yaml:"Address,omitempty"`
	WriteInterval time.Duration `yaml:"WriteInterval,omitempty"`
	Prefix        string        `yaml:"Prefix,omitempty"`
}

type ChannelParticipation struct {
	Enabled            bool   `yaml:"Enabled"`
	MaxRequestBodySize string `yaml:"MaxRequestBodySize,omitempty"`
}
