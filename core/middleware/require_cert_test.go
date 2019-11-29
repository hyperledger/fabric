/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"

	"github.com/hyperledger/fabric/core/middleware"
	"github.com/hyperledger/fabric/core/middleware/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RequireCert", func() {
	var (
		requireCert middleware.Middleware
		handler     *fakes.HTTPHandler
		chain       http.Handler

		req  *http.Request
		resp *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		handler = &fakes.HTTPHandler{}
		requireCert = middleware.RequireCert()
		chain = requireCert(handler)

		req = httptest.NewRequest("GET", "https:///", nil)
		req.TLS.VerifiedChains = [][]*x509.Certificate{{
			&x509.Certificate{},
		}}
		resp = httptest.NewRecorder()
	})

	It("delegates to the next handler when the first verified chain is not empty", func() {
		chain.ServeHTTP(resp, req)
		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(handler.ServeHTTPCallCount()).To(Equal(1))
	})

	Context("when the TLS connection state is nil", func() {
		BeforeEach(func() {
			req.TLS = nil
		})

		It("responds with http.StatusUnauthorized", func() {
			chain.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		})

		It("does not call the next handler", func() {
			chain.ServeHTTP(resp, req)
			Expect(handler.ServeHTTPCallCount()).To(Equal(0))
		})
	})

	Context("when verified chains is nil", func() {
		BeforeEach(func() {
			req.TLS.VerifiedChains = nil
		})

		It("responds with http.StatusUnauthorized", func() {
			chain.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		})

		It("does not call the next handler", func() {
			chain.ServeHTTP(resp, req)
			Expect(handler.ServeHTTPCallCount()).To(Equal(0))
		})
	})

	Context("when verified chains is empty", func() {
		BeforeEach(func() {
			req.TLS.VerifiedChains = [][]*x509.Certificate{}
		})

		It("responds with http.StatusUnauthorized", func() {
			chain.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		})

		It("does not call the next handler", func() {
			chain.ServeHTTP(resp, req)
			Expect(handler.ServeHTTPCallCount()).To(Equal(0))
		})
	})

	Context("when the first verified chain is empty", func() {
		BeforeEach(func() {
			req.TLS.VerifiedChains = [][]*x509.Certificate{{}}
		})

		It("responds with http.StatusUnauthorized", func() {
			chain.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusUnauthorized))
		})

		It("does not call the next handler", func() {
			chain.ServeHTTP(resp, req)
			Expect(handler.ServeHTTPCallCount()).To(Equal(0))
		})
	})
})
