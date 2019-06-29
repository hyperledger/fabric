/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func TestServerKeepaliveOptions(t *testing.T) {
	t.Parallel()

	kap := keepalive.ServerParameters{
		Time:    DefaultKeepaliveOptions.ServerInterval,
		Timeout: DefaultKeepaliveOptions.ServerTimeout,
	}
	kep := keepalive.EnforcementPolicy{
		MinTime:             DefaultKeepaliveOptions.ServerMinInterval,
		PermitWithoutStream: true,
	}
	expectedOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(kap),
		grpc.KeepaliveEnforcementPolicy(kep),
	}
	opts := ServerKeepaliveOptions(DefaultKeepaliveOptions)
	assert.ObjectsAreEqual(expectedOpts, opts)

}

func TestClientKeepaliveOptions(t *testing.T) {
	t.Parallel()

	kap := keepalive.ClientParameters{
		Time:                DefaultKeepaliveOptions.ClientInterval,
		Timeout:             DefaultKeepaliveOptions.ClientTimeout,
		PermitWithoutStream: true,
	}
	expectedOpts := []grpc.DialOption{grpc.WithKeepaliveParams(kap)}
	opts := ClientKeepaliveOptions(DefaultKeepaliveOptions)
	assert.ObjectsAreEqual(expectedOpts, opts)

}

func TestClientConfigClone(t *testing.T) {
	origin := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval: time.Second,
		},
		SecOpts: SecureOptions{
			Key: []byte{1, 2, 3},
		},
		Timeout:      time.Second,
		AsyncConnect: true,
	}

	clone := origin.Clone()

	// Same content, different inner fields references.
	assert.Equal(t, origin, clone)

	// We change the contents of the fields and ensure it doesn't
	// propagate across instances.
	origin.AsyncConnect = false
	origin.KaOpts.ServerInterval = time.Second
	origin.KaOpts.ClientInterval = time.Hour
	origin.SecOpts.Certificate = []byte{1, 2, 3}
	origin.SecOpts.Key = []byte{5, 4, 6}
	origin.Timeout = time.Second * 2

	clone.SecOpts.UseTLS = true
	clone.KaOpts.ServerMinInterval = time.Hour

	expectedOriginState := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval: time.Hour,
			ServerInterval: time.Second,
		},
		SecOpts: SecureOptions{
			Key:         []byte{5, 4, 6},
			Certificate: []byte{1, 2, 3},
		},
		Timeout: time.Second * 2,
	}

	expectedCloneState := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval:    time.Second,
			ServerMinInterval: time.Hour,
		},
		SecOpts: SecureOptions{
			Key:    []byte{1, 2, 3},
			UseTLS: true,
		},
		Timeout:      time.Second,
		AsyncConnect: true,
	}

	assert.Equal(t, expectedOriginState, origin)
	assert.Equal(t, expectedCloneState, clone)
}
