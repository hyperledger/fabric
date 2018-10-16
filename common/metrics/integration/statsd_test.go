/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration_test

import (
	"io"
	"net"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics/goruntime"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Statsd", func() {
	var (
		provider  *statsd.Provider
		collector *goruntime.Collector
	)

	BeforeEach(func() {
		provider = &statsd.Provider{
			Statsd: kitstatsd.New("", log.NewNopLogger()),
		}
		collector = goruntime.NewCollector(provider)
	})

	It("sends collected metrics to a UDP sink", func() {
		udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		sock, err := net.ListenUDP("udp", udpAddr)
		Expect(err).NotTo(HaveOccurred())
		err = sock.SetReadBuffer(1024 * 1024)
		Expect(err).NotTo(HaveOccurred())

		done := make(chan struct{})
		errCh := make(chan error, 1)
		datagramBuffer := gbytes.NewBuffer()
		go receiveDatagrams(sock, datagramBuffer, done, errCh)

		collectTicker := time.NewTicker(50 * time.Millisecond)
		defer collectTicker.Stop()
		go collector.CollectAndPublish(collectTicker.C)

		statsdTicker := time.NewTicker(100 * time.Millisecond)
		defer statsdTicker.Stop()
		go provider.Statsd.SendLoop(statsdTicker.C, "udp", sock.LocalAddr().String())

		Eventually(datagramBuffer, 5*time.Second).Should(gbytes.Say("runtime.go.goroutine.count"))
		close(done)
		Eventually(errCh).Should(Receive(BeNil()))

		for _, stat := range strings.Split(string(datagramBuffer.Contents()), "\n") {
			if stat != "" {
				Expect(stat).To(MatchRegexp(`runtime\.go\..*:\d{1,}\.\d{1,}\|g(@.*)?`))
			}
		}
	})
})

func receiveDatagrams(sock *net.UDPConn, w io.Writer, done <-chan struct{}, errCh chan<- error) {
	defer sock.Close()

	buf := make([]byte, 1024*1024)
	for {
		select {
		case <-done:
			errCh <- nil
			return

		default:
			n, _, err := sock.ReadFrom(buf)
			if err != nil {
				errCh <- err
				return
			}
			_, err = w.Write(buf[0:n])
			if err != nil {
				errCh <- err
				return
			}
		}
	}
}
