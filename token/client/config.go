/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client

import "github.com/pkg/errors"

// ConnectionConfig contains data required to establish grpc connection to a peer or orderer
type ConnectionConfig struct {
	Address            string
	TlsRootCertFile    string
	ServerNameOverride string
}

// ClientConfig will be updated after the CR for token client config is merged, where the config data
// will be populated based on a config file.
type ClientConfig struct {
	ChannelId     string
	MspDir        string
	MspId         string
	TlsEnabled    bool
	OrdererCfg    ConnectionConfig
	CommitPeerCfg ConnectionConfig
	ProverPeerCfg ConnectionConfig
}

func ValidateClientConfig(config *ClientConfig) error {
	if config == nil {
		return errors.New("client config is nil")
	}
	if config.ChannelId == "" {
		return errors.New("missing channelId")
	}

	if config.OrdererCfg.Address == "" {
		return errors.New("missing orderer address")
	}

	if config.TlsEnabled && config.OrdererCfg.TlsRootCertFile == "" {
		return errors.New("missing orderer TlsRootCertFile")
	}

	if config.OrdererCfg.Address == "" {
		return errors.New("missing commit peer address")
	}

	if config.TlsEnabled && config.OrdererCfg.TlsRootCertFile == "" {
		return errors.New("missing commit peer TlsRootCertFile")
	}

	// TODO: add prover peer validation in a different CR
	return nil
}
