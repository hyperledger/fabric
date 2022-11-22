/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricconfig

import "time"

type ConfigTx struct {
	Organizations []*Organization     `yaml:"Organizations,omitempty"`
	Capabilities  *Capabilities       `yaml:"Capabilities,omitempty"`
	Application   *Application        `yaml:"Application,omitempty"`
	Orderer       *ConfigTxOrderer    `yaml:"Orderer,omitempty"`
	Channel       *Channel            `yaml:"Channel,omitempty"`
	Profiles      map[string]*Channel `yaml:"Profiles,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Organization struct {
	Name             string             `yaml:"Name,omitempty"`
	SkipAsForeign    bool               `yaml:"SkipAsForeign,omitempty"`
	ID               string             `yaml:"ID,omitempty"`
	MSPDir           string             `yaml:"MSPDir,omitempty"`
	Policies         map[string]*Policy `yaml:"Policies,omitempty"`
	OrdererEndpoints []string           `yaml:"OrdererEndpoints,omitempty"`
	AnchorPeers      []*AnchorPeer      `yaml:"AnchorPeers,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Policy struct {
	Type string `yaml:"Type,omitempty"`
	Rule string `yaml:"Rule,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Capabilities struct {
	Channel     map[string]bool `yaml:"Channel,omitempty"`
	Orderer     map[string]bool `yaml:"Orderer,omitempty"`
	Application map[string]bool `yaml:"Application,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type AnchorPeer struct {
	Host string `yaml:"Host,omitempty"`
	Port int    `yaml:"Port,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Application struct {
	ACLs          map[string]string  `yaml:"ACLs,omitempty"`
	Organizations []*Organization    `yaml:"Organizations,omitempty"`
	Policies      map[string]*Policy `yaml:"Policies,omitempty"`
	Capabilities  map[string]bool    `yaml:"Capabilities,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type ConsenterMapping struct {
	ID       uint64
	Host     string
	Port     int
	MSPID    string
	Identity string
}

type ConfigTxOrderer struct {
	OrdererType      string             `yaml:"OrdererType,omitempty"`
	BatchTimeout     time.Duration      `yaml:"BatchTimeout,omitempty"`
	BatchSize        *BatchSize         `yaml:"BatchSize,omitempty"`
	Kafka            *ConfigTxKafka     `yaml:"Kafka,omitempty"`
	EtcdRaft         *ConfigTxEtcdRaft  `yaml:"EtcdRaft,omitempty"`
	ConsenterMapping []ConsenterMapping `yaml:"ConsenterMapping,omitempty"`
	Organizations    []*Organization    `yaml:"Organizations,omitempty"`
	Policies         map[string]*Policy `yaml:"Policies,omitempty"`
	Capabilities     map[string]bool    `yaml:"Capabilities,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type BatchSize struct {
	MaxMessageCount   string `yaml:"MaxMessageCount,omitempty"`
	AbsoluteMaxBytes  string `yaml:"AbsoluteMaxBytes,omitempty"`
	PreferredMaxBytes string `yaml:"PreferredMaxBytes,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type ConfigTxKafka struct {
	Brokers []string `yaml:"Brokers,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type ConfigTxEtcdRaft struct {
	Consenters []*Consenter     `yaml:"Consenters,omitempty"`
	Options    *EtcdRaftOptions `yaml:"EtcdRaftOptions,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Consenter struct {
	Host          string `yaml:"Host,omitempty"`
	Port          int    `yaml:"Port,omitempty"`
	ClientTLSCert string `yaml:"ClientTLSCert,omitempty"`
	ServerTLSCert string `yaml:"ServerTLSCert,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type EtcdRaftOptions struct {
	TickInterval         string `yaml:"TickInterval,omitempty"`
	ElectionTick         string `yaml:"ElectionTick,omitempty"`
	HeartbeatTick        string `yaml:"HeartbeatTick,omitempty"`
	MaxInflightBlocks    string `yaml:"MaxInflightBlocks,omitempty"`
	SnapshotIntervalSize string `yaml:"SnapshotIntervalSize,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Channel struct {
	Orderer      *ConfigTxOrderer       `yaml:"Orderer,omitempty"`
	Application  *Application           `yaml:"Application,omitempty"`
	Policies     map[string]*Policy     `yaml:"Policies,omitempty"`
	Capabilities map[string]bool        `yaml:"Capabilities,omitempty"`
	Consortiums  map[string]*Consortium `yaml:"Consortiums,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}

type Consortium struct {
	Organizations []*Organization `yaml:"Organizations,omitempty"`

	ExtraProperties map[string]interface{} `yaml:",inline,omitempty"`
}
