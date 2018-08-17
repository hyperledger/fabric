/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricconfig

import "time"

type Orderer struct {
	General    *General    `yaml:"General,omitempty"`
	FileLedger *FileLedger `yaml:"FileLedger,omitempty"`
	RAMLedger  *RAMLedger  `yaml:"RAMLedger,omitempty"`
	Kafka      *Kafka      `yaml:"Kafka,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type General struct {
	LedgerType     string                 `yaml:"LedgerType,omitempty"`
	ListenAddress  string                 `yaml:"ListenAddress,omitempty"`
	ListenPort     int                    `yaml:"ListenPort,omitempty"`
	TLS            *OrdererTLS            `yaml:"TLS,omitempty"`
	Keepalive      *OrdererKeepalive      `yaml:"Keepalive,omitempty"`
	LogLevel       string                 `yaml:"LogLevel,omitempty"`
	LogFormat      string                 `yaml:"LogFormat,omitempty"`
	GenesisMethod  string                 `yaml:"GenesisMethod,omitempty"`
	GenesisProfile string                 `yaml:"GenesisProfile,omitempty"`
	GenesisFile    string                 `yaml:"GenesisFile,omitempty"`
	LocalMSPDir    string                 `yaml:"LocalMSPDir,omitempty"`
	LocalMSPID     string                 `yaml:"LocalMSPID,omitempty"`
	Profile        *OrdererProfile        `yaml:"Profile,omitempty"`
	BCCSP          *BCCSP                 `yaml:"BCCSP,omitempty"`
	Authentication *OrdererAuthentication `yaml:"Authentication,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type OrdererTLS struct {
	Enabled            bool     `yaml:"Enabled"`
	PrivateKey         string   `yaml:"PrivateKey,omitempty"`
	Certificate        string   `yaml:"Certificate,omitempty"`
	RootCAs            []string `yaml:"RootCAs,omitempty"`
	ClientAuthRequired bool     `yaml:"ClientAuthRequired"`
	ClientRootCAs      []string `yaml:"ClientRootCAs,omitempty"`
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
	Prefix   string `yaml:"Prefix,omitempty"`
}

type RAMLedger struct {
	HistorySize int `yaml:"HistorySize,omitempty"`
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
