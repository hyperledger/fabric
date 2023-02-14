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

func (c *ConnectionsMgr) Connect(endpoint string, serverRootCACert [][]byte) (*grpc.ClientConn, error) {
	if serverRootCACert == nil {
		return nil, errors.New("server root CA cert is nil")
	}

	c.lock.Lock()
	conn, alreadyConnected := c.Connections.Lookup(endpoint)
	if alreadyConnected {
		c.lock.Unlock()
		return conn, nil
	}
	dialer := c.dialer
	c.lock.Unlock()

	dialer.SecOpts.ServerRootCAs = serverRootCACert
	newConn, err := dialer.Dial(endpoint)
	if err != nil {
		return nil, err
	}

	c.lock.Lock()
	// check again if someother goroutine successful meanwhile
	conn, alreadyConnected = c.Connections.Lookup(endpoint)
	if alreadyConnected {
		c.lock.Unlock()
		newConn.Close()
		return conn, nil
	}
	c.Connections.Put(endpoint, newConn)
	c.lock.Unlock()

	return newConn, nil
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
