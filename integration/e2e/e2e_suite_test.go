/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EndToEnd Suite")
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
	buildServer = nwo.NewBuildServer()
	buildServer.Serve()

	components = buildServer.Components()
	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	buildServer.Shutdown()
})

func StartPort() int {
	return integration.E2EBasePort.StartPortForNode()
}

type MetricsReader struct {
	buffer    *gbytes.Buffer
	errCh     chan error
	listener  net.Listener
	doneCh    chan struct{}
	closeOnce sync.Once
	err       error
}

func NewMetricsReader() *MetricsReader {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())

	return &MetricsReader{
		buffer:   gbytes.NewBuffer(),
		listener: listener,
		errCh:    make(chan error, 1),
		doneCh:   make(chan struct{}),
	}
}

func (mr *MetricsReader) Buffer() *gbytes.Buffer {
	return mr.buffer
}

func (mr *MetricsReader) Address() string {
	return mr.listener.Addr().String()
}

func (mr *MetricsReader) String() string {
	return string(mr.buffer.Contents())
}

func (mr *MetricsReader) Start() {
	for {
		conn, err := mr.listener.Accept()
		if err != nil {
			mr.errCh <- err
			return
		}
		go mr.handleConnection(conn)
	}
}

func (mr *MetricsReader) handleConnection(c net.Conn) {
	defer GinkgoRecover()
	defer c.Close()

	br := bufio.NewReader(c)
	for {
		select {
		case <-mr.doneCh:
			c.Close()
		default:
			data, err := br.ReadBytes('\n')
			if err == io.EOF {
				return
			}
			Expect(err).NotTo(HaveOccurred())

			_, err = mr.buffer.Write(data)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func (mr *MetricsReader) Close() error {
	mr.closeOnce.Do(func() {
		close(mr.doneCh)
		err := mr.listener.Close()
		mr.err = <-mr.errCh
		if mr.err == nil && err != nil && err != io.EOF {
			mr.err = err
		}
	})
	return mr.err
}
