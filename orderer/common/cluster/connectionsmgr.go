/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"sync"

	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type ConnectionsCache map[string]*grpc.ClientConn

func (cbc ConnectionsCache) Lookup(key string) (*grpc.ClientConn, bool) {
	conn, ok := cbc[key]
	return conn, ok
}

func (cbc ConnectionsCache) Put(key string, conn *grpc.ClientConn) {
	cbc[key] = conn
}

func (cbc ConnectionsCache) Remove(key string) {
	delete(cbc, key)
}

func (cbc ConnectionsCache) Size() int {
	return len(cbc)
}

type ConnectionsMgr struct {
	lock        sync.RWMutex
	Connections ConnectionsCache
	dialer      comm.ClientConfig
}

func (c *ConnectionsMgr) Connect(endpoint string, serverRootCACert []byte) (*grpc.ClientConn, error) {
	if serverRootCACert == nil {
		return nil, errors.New("Server Root CA Cert is nil")
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	conn, alreadyConnected := c.Connections.Lookup(endpoint)
	if alreadyConnected {
		return conn, nil
	}

	c.dialer.SecOpts.ServerRootCAs = [][]byte{serverRootCACert}

	conn, err := c.dialer.Dial(endpoint)
	if err != nil {
		return nil, err
	}

	c.Connections.Put(endpoint, conn)

	return conn, nil
}

func (c *ConnectionsMgr) Disconnect(endpoint string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	conn, connected := c.Connections.Lookup(endpoint)
	if !connected {
		return
	}
	conn.Close()
	c.Connections.Remove(endpoint)
}

func NewConnectionMgr(dialer comm.ClientConfig) *ConnectionsMgr {
	connMapping := &ConnectionsMgr{
		Connections: make(ConnectionsCache),
		dialer:      dialer,
	}
	return connMapping
}
