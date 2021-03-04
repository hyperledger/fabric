/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const defaultTimeout = time.Second * 5

// Client deals with TLS connections
// to the discovery server
type Client struct {
	config      comm.ClientConfig
	TLSCertHash []byte
}

// NewClient creates a new comm client out of the given configuration
func NewClient(conf Config) (*Client, error) {
	if conf.Timeout == time.Duration(0) {
		conf.Timeout = defaultTimeout
	}
	sop, err := conf.ToSecureOptions(newSelfSignedTLSCert)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cc := comm.ClientConfig{
		SecOpts:     sop,
		DialTimeout: conf.Timeout,
	}
	return &Client{config: cc, TLSCertHash: util.ComputeSHA256(sop.Certificate)}, nil
}

// NewDialer creates a new dialer from the given endpoint
func (c *Client) NewDialer(endpoint string) func() (*grpc.ClientConn, error) {
	return func() (*grpc.ClientConn, error) {
		conn, err := c.config.Dial(endpoint)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return conn, nil
	}
}

func newSelfSignedTLSCert() (*tlsgen.CertKeyPair, error) {
	ca, err := tlsgen.NewCA()
	if err != nil {
		return nil, err
	}
	return ca.NewClientCertKeyPair()
}
