/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderer

import (
	"crypto/x509"
)

const (

	// ConsensusStateNormal indicates normal orderer operation.
	ConsensusStateNormal ConsensusState = "STATE_NORMAL"

	// ConsensusStateMaintenance indicates the orderer is in consensus type migration.
	ConsensusStateMaintenance ConsensusState = "STATE_MAINTENANCE"

	// ConsensusTypeSolo identifies the solo consensus implementation.
	ConsensusTypeSolo = "solo"

	// ConsensusTypeKafka identifies the Kafka-based consensus implementation.
	ConsensusTypeKafka = "kafka"

	// ConsensusTypeEtcdRaft identifies the Raft-based consensus implementation.
	ConsensusTypeEtcdRaft = "etcdraft"

	// KafkaBrokersKey is the common.ConfigValue type key name for the KafkaBrokers message.
	KafkaBrokersKey = "KafkaBrokers"

	// ConsensusTypeKey is the common.ConfigValue type key name for the ConsensusType message.
	ConsensusTypeKey = "ConsensusType"

	// BatchSizeKey is the common.ConfigValue type key name for the BatchSize message.
	BatchSizeKey = "BatchSize"

	// BatchTimeoutKey is the common.ConfigValue type key name for the BatchTimeout message.
	BatchTimeoutKey = "BatchTimeout"

	// ChannelRestrictionsKey is the key name for the ChannelRestrictions message.
	ChannelRestrictionsKey = "ChannelRestrictions"
)

// ConsensusState defines the orderer mode of operation.
// Options: `ConsensusStateNormal` and `ConsensusStateMaintenance`
type ConsensusState string

// BatchSize is the configuration affecting the size of batches.
type BatchSize struct {
	// MaxMessageCount is the max message count.
	MaxMessageCount uint32
	// AbsoluteMaxBytes is the max block size (not including headers).
	AbsoluteMaxBytes uint32
	// PreferredMaxBytes is the preferred size of blocks.
	PreferredMaxBytes uint32
}

// Kafka is a list of Kafka broker endpoints.
type Kafka struct {
	// Brokers contains the addresses of *at least two* kafka brokers
	// Must be in `IP:port` notation
	Brokers []string
}

// EtcdRaft is serialized and set as the value of ConsensusType.Metadata in
// a channel configuration when the ConsensusType.Type is set to "etcdraft".
type EtcdRaft struct {
	Consenters []Consenter
	Options    EtcdRaftOptions
}

// EtcdRaftOptions to be specified for all the etcd/raft nodes.
// These can be modified on a per-channel basis.
type EtcdRaftOptions struct {
	TickInterval      string
	ElectionTick      uint32
	HeartbeatTick     uint32
	MaxInflightBlocks uint32
	// Take snapshot when cumulative data exceeds certain size in bytes.
	SnapshotIntervalSize uint32
}

// Consenter represents a consenting node (i.e. replica).
type Consenter struct {
	Address       EtcdAddress
	ClientTLSCert *x509.Certificate
	ServerTLSCert *x509.Certificate
}

// EtcdAddress contains the hostname and port for an endpoint.
type EtcdAddress struct {
	Host string
	Port int
}
