/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/comm/testpb"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name         string
		config       comm.ServerConfig
		createServer bool
		service      string
		expectErr    bool
	}{
		{
			name: "Serving",
			config: comm.ServerConfig{
				HealthCheckEnabled: true,
			},
			createServer: true,
			service:      "EmptyService",
		},
		{
			name: "Serving Empty Name",
			config: comm.ServerConfig{
				HealthCheckEnabled: true,
			},
			createServer: true,
			service:      "",
		},
		{
			name: "UnknownService",
			config: comm.ServerConfig{
				HealthCheckEnabled: true,
			},
			createServer: true,
			service:      "UnknownService",
			expectErr:    true,
		},
		{
			name: "No gRPC Server",
			config: comm.ServerConfig{
				HealthCheckEnabled: false,
			},
			service:   "EmptyService",
			expectErr: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("failed to start listener: [%s]", err)
			}
			defer lis.Close()
			if test.createServer {
				srv, err := comm.NewGRPCServerFromListener(lis, test.config)
				if err != nil {
					t.Fatalf("failed to create server: [%s]", err)
				}
				testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})
				go srv.Start()
				defer srv.Stop()
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			//create HealthCheckClient
			config := comm.ClientConfig{
				Timeout: 1 * time.Second,
			}
			hcc, err := comm.NewHealthCheckClient(
				config,
				lis.Addr().String(),
				test.service,
			)
			if err != nil {
				t.Fatalf("failed to create HealthCheckClient: [%s]", err)
			}
			err = hcc.HealthCheck(ctx)
			if test.expectErr {
				assert.Contains(
					t,
					err.Error(),
					fmt.Sprintf(
						"failed to connect to service '%s' at '%s'",
						hcc.Service,
						hcc.Address),
				)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
