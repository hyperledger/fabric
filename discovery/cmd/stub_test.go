/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/cmd/common"
	"github.com/hyperledger/fabric/cmd/common/comm"
	"github.com/hyperledger/fabric/cmd/common/signer"
	c "github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/discovery/client"
	"github.com/stretchr/testify/assert"
)

func TestClientStub(t *testing.T) {
	srv, err := c.NewGRPCServer("127.0.0.1:", c.ServerConfig{
		SecOpts: &c.SecureOptions{},
	})
	assert.NoError(t, err)
	go srv.Start()
	defer srv.Stop()

	_, portStr, _ := net.SplitHostPort(srv.Address())
	endpoint := fmt.Sprintf("localhost:%s", portStr)
	stub := &ClientStub{}

	req := discovery.NewRequest()

	_, err = stub.Send(endpoint, common.Config{
		SignerConfig: signer.Config{
			MSPID:        "Org1MSP",
			KeyPath:      filepath.Join("testdata", "8150cb2d09628ccc89727611ebb736189f6482747eff9b8aaaa27e9a382d2e93_sk"),
			IdentityPath: filepath.Join("testdata", "cert.pem"),
		},
		TLSConfig: comm.Config{},
	}, req)
	assert.Contains(t, err.Error(), "Unimplemented desc = unknown service discovery.Discovery")
}

func TestRawStub(t *testing.T) {
	srv, err := c.NewGRPCServer("127.0.0.1:", c.ServerConfig{
		SecOpts: &c.SecureOptions{},
	})
	assert.NoError(t, err)
	go srv.Start()
	defer srv.Stop()

	_, portStr, _ := net.SplitHostPort(srv.Address())
	endpoint := fmt.Sprintf("localhost:%s", portStr)
	stub := &RawStub{}

	req := discovery.NewRequest()

	_, err = stub.Send(endpoint, common.Config{
		SignerConfig: signer.Config{
			MSPID:        "Org1MSP",
			KeyPath:      filepath.Join("testdata", "8150cb2d09628ccc89727611ebb736189f6482747eff9b8aaaa27e9a382d2e93_sk"),
			IdentityPath: filepath.Join("testdata", "cert.pem"),
		},
		TLSConfig: comm.Config{},
	}, req)
	assert.Contains(t, err.Error(), "Unimplemented desc = unknown service discovery.Discovery")
}
