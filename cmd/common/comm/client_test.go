/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/stretchr/testify/assert"
)

func TestTLSClient(t *testing.T) {
	srv, err := comm.NewGRPCServer("127.0.0.1:", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			UseTLS:      true,
			Key:         loadFileOrDie(filepath.Join("testdata", "server", "key.pem")),
			Certificate: loadFileOrDie(filepath.Join("testdata", "server", "cert.pem")),
		},
	})
	assert.NoError(t, err)
	go srv.Start()
	defer srv.Stop()
	conf := Config{
		Timeout:        time.Millisecond * 100,
		PeerCACertPath: filepath.Join("testdata", "server", "ca.pem"),
	}
	cl, err := NewClient(conf)
	assert.NoError(t, err)
	_, port, _ := net.SplitHostPort(srv.Address())
	dial := cl.NewDialer(fmt.Sprintf("localhost:%s", port))
	conn, err := dial()
	assert.NoError(t, err)
	conn.Close()

	dial = cl.NewDialer(fmt.Sprintf("non_existent_host.xyz.blabla:%s", port))
	_, err = dial()
	assert.Error(t, err)

}

func TestNonTLSClient(t *testing.T) {
	srv, err := comm.NewGRPCServer("127.0.0.1:", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{},
	})
	assert.NoError(t, err)
	go srv.Start()
	defer srv.Stop()
	conf := Config{}
	cl, err := NewClient(conf)
	assert.NoError(t, err)
	_, port, _ := net.SplitHostPort(srv.Address())
	dial := cl.NewDialer(fmt.Sprintf("localhost:%s", port))
	conn, err := dial()
	assert.NoError(t, err)
	conn.Close()
}

func TestClientBadConfig(t *testing.T) {
	conf := Config{
		PeerCACertPath: filepath.Join("testdata", "server", "non_existent_file"),
	}
	cl, err := NewClient(conf)
	assert.Nil(t, cl)
	assert.Contains(t, err.Error(), "open testdata/server/non_existent_file: no such file or directory")

	conf = Config{
		PeerCACertPath: filepath.Join("testdata", "server", "ca.pem"),
		KeyPath:        "non_existent_file",
		CertPath:       "non_existent_file",
	}
	cl, err = NewClient(conf)
	assert.Nil(t, cl)
	assert.Contains(t, err.Error(), "open non_existent_file: no such file or directory")

	conf = Config{
		PeerCACertPath: filepath.Join("testdata", "server", "ca.pem"),
		KeyPath:        filepath.Join("testdata", "client", "key.pem"),
		CertPath:       "non_existent_file",
	}
	cl, err = NewClient(conf)
	assert.Nil(t, cl)
	assert.Contains(t, err.Error(), "open non_existent_file: no such file or directory")
}

func loadFileOrDie(path string) []byte {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println("Failed opening file", path, ":", err)
		os.Exit(1)
	}
	return b
}
