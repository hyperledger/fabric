/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration_test

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("CORS", func() {
	var (
		sess *gexec.Session
		req  *http.Request

		// runServer starts the server on an ephemeral port, then creates a CORS request
		// targeting that same server (but does not send it), it must be invoked inside
		// the BeforeEach of each test
		runServer func(args ...string)
	)

	BeforeEach(func() {
		runServer = func(args ...string) {
			cmd := exec.Command(configtxlatorPath, args...)
			var err error
			errBuffer := gbytes.NewBuffer()
			sess, err = gexec.Start(cmd, GinkgoWriter, io.MultiWriter(errBuffer, GinkgoWriter))
			Expect(err).NotTo(HaveOccurred())
			Consistently(sess.Exited).ShouldNot(BeClosed())
			Eventually(errBuffer).Should(gbytes.Say("Serving HTTP requests on 127.0.0.1:"))
			address := regexp.MustCompile("127.0.0.1:[0-9]+").FindString(string(errBuffer.Contents()))
			Expect(address).NotTo(BeEmpty())

			req, err = http.NewRequest("OPTIONS", fmt.Sprintf("http://%s/protolator/encode/common.Block", address), nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Origin", "http://foo.com")
			req.Header.Add("Access-Control-Request-Method", "POST")
			req.Header.Add("Access-Control-Request-Headers", "Content-Type")
		}
	})

	AfterEach(func() {
		sess.Signal(syscall.SIGKILL)
		Eventually(sess.Exited).Should(BeClosed())
		Expect(sess.ExitCode()).To(Equal(137))
	})

	Context("when CORS options are not provided", func() {
		BeforeEach(func() {
			runServer("start", "--hostname", "127.0.0.1", "--port", "0")
		})

		It("rejects CORS OPTIONS requests", func() {
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
		})
	})

	Context("when the CORS wildcard is provided", func() {
		BeforeEach(func() {
			runServer("start", "--hostname", "127.0.0.1", "--port", "0", "--CORS", "*")
		})

		It("it allows CORS requests from any domain", func() {
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Header.Get("Access-Control-Allow-Origin")).To(Equal("*"))
			Expect(resp.Header.Get("Access-Control-Allow-Headers")).To(Equal("Content-Type"))
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Context("when multiple CORS options are provided", func() {
		BeforeEach(func() {
			runServer("start", "--hostname", "127.0.0.1", "--port", "0", "--CORS", "http://foo.com", "--CORS", "http://bar.com")
		})

		It("it allows CORS requests from any of them", func() {
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Header.Get("Access-Control-Allow-Origin")).To(Equal("http://foo.com"))
			Expect(resp.Header.Get("Access-Control-Allow-Headers")).To(Equal("Content-Type"))
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
