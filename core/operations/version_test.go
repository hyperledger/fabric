/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Version", func() {
	It("returns 200 if the method is GET", func() {
		resp := httptest.NewRecorder()

		versionInfoHandler := &VersionInfoHandler{Version: "latest"}
		versionInfoHandler.ServeHTTP(resp, &http.Request{Method: http.MethodGet})
		Expect(resp.Result().StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Result().Header.Get("Content-Type")).To(Equal("application/json"))
		Expect(resp.Body).To(MatchJSON(`{"Version": "latest"}`))
	})

	It("returns 400 when an unsupported method is used", func() {
		resp := httptest.NewRecorder()

		versionInfoHandler := &VersionInfoHandler{}
		versionInfoHandler.ServeHTTP(resp, &http.Request{Method: http.MethodPut})
		Expect(resp.Result().StatusCode).To(Equal(http.StatusBadRequest))
		Expect(resp.Body).To(MatchJSON(`{"Error": "invalid request method: PUT"}`))
	})

	It("returns 500 when the payload is invalid JSON", func() {
		resp := httptest.NewRecorder()

		versionInfoHandler := &VersionInfoHandler{}
		versionInfoHandler.sendResponse(resp, 200, make(chan int))
		Expect(resp.Result().StatusCode).To(Equal(http.StatusInternalServerError))
	})
})
