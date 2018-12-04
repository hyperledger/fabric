/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/pkg/errors"
)

const DefaultConnectionTimeout = 10 * time.Second

// CreateGRPCClient returns a comm.GRPCClient based on toke client config
func CreateGRPCClient(config *ConnectionConfig) (*comm.GRPCClient, error) {
	timeout := config.ConnectionTimeout
	if timeout <= 0 {
		timeout = DefaultConnectionTimeout
	}
	clientConfig := comm.ClientConfig{Timeout: timeout}

	if config.TLSEnabled {
		if config.TLSRootCertFile == "" {
			return nil, errors.New("missing TLSRootCertFile in client config")
		}
		caPEM, err := ioutil.ReadFile(config.TLSRootCertFile)
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("unable to load TLS cert from %s", config.TLSRootCertFile))
		}
		secOpts := &comm.SecureOptions{
			UseTLS:            true,
			ServerRootCAs:     [][]byte{caPEM},
			RequireClientCert: false,
		}
		clientConfig.SecOpts = secOpts
	}

	return comm.NewGRPCClient(clientConfig)
}
