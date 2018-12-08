/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package httpadmin_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/httpadmin"
	"github.com/hyperledger/fabric/common/flogging/httpadmin/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SpecHandler", func() {
	var (
		fakeLogging *fakes.Logging
		handler     *httpadmin.SpecHandler
	)

	BeforeEach(func() {
		fakeLogging = &fakes.Logging{}
		fakeLogging.SpecReturns("the-returned-specification")
		handler = &httpadmin.SpecHandler{
			Logging: fakeLogging,
		}
	})

	It("responds with the current logging spec", func() {
		req := httptest.NewRequest("GET", "/ignored", nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)

		Expect(fakeLogging.SpecCallCount()).To(Equal(1))
		Expect(resp.Code).To(Equal(http.StatusOK))
		Expect(resp.Body).To(MatchJSON(`{"spec": "the-returned-specification"}`))
		Expect(resp.Header().Get("Content-Type")).To(Equal("application/json"))
	})

	It("sets the current logging spec", func() {
		req := httptest.NewRequest("PUT", "/ignored", strings.NewReader(`{"spec": "updated-spec"}`))
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)

		Expect(resp.Code).To(Equal(http.StatusNoContent))
		Expect(fakeLogging.ActivateSpecCallCount()).To(Equal(1))
		Expect(fakeLogging.ActivateSpecArgsForCall(0)).To(Equal("updated-spec"))
	})

	Context("when the update spec payload cannot be decoded", func() {
		It("responds with an error payload", func() {
			req := httptest.NewRequest("PUT", "/ignored", strings.NewReader(`goo`))
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			Expect(fakeLogging.ActivateSpecCallCount()).To(Equal(0))
			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(resp.Body).To(MatchJSON(`{"error": "invalid character 'g' looking for beginning of value"}`))
		})
	})

	Context("when activating the spec fails", func() {
		BeforeEach(func() {
			fakeLogging.ActivateSpecReturns(errors.New("ewww; that's not right!"))
		})

		It("responds with an error payload", func() {
			req := httptest.NewRequest("PUT", "/ignored", strings.NewReader(`{}`))
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(resp.Body).To(MatchJSON(`{"error": "ewww; that's not right!"}`))
		})
	})

	Context("when an unsupported method is used", func() {
		It("responds with an error", func() {
			req := httptest.NewRequest("POST", "/ignored", strings.NewReader(`{}`))
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(resp.Body).To(MatchJSON(`{"error": "invalid request method: POST"}`))
		})

		It("doesn't use logging", func() {
			req := httptest.NewRequest("POST", "/ignored", strings.NewReader(`{}`))
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			Expect(fakeLogging.ActivateSpecCallCount()).To(Equal(0))
			Expect(fakeLogging.SpecCallCount()).To(Equal(0))
		})
	})

	Describe("NewSpecHandler", func() {
		It("constructs a handler that modifies the global spec", func() {
			specHandler := httpadmin.NewSpecHandler()
			Expect(specHandler.Logging).To(Equal(flogging.Global))
			Expect(specHandler.Logger).NotTo(BeNil())
		})
	})
})
