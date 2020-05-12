/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPHandler(t *testing.T) {
	config := localconfig.ChannelParticipation{
		Enabled:       false,
		RemoveStorage: false,
	}
	h := channelparticipation.NewHTTPHandler(config, &mocks.ChannelManagement{})
	assert.NotNilf(t, h, "cannot create handler")
}

func TestHTTPHandler_ServeHTTP_Disabled(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: false, RemoveStorage: false}
	h := channelparticipation.NewHTTPHandler(config, &mocks.ChannelManagement{})
	require.NotNilf(t, h, "cannot create handler")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", channelparticipation.URLBaseV1, nil)
	h.ServeHTTP(resp, req)
	checkErrorResponse(t, http.StatusServiceUnavailable, "channel participation API is disabled", resp)
}

func TestHTTPHandler_ServeHTTP_InvalidMethods(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
	h := channelparticipation.NewHTTPHandler(config, nil)
	require.NotNilf(t, h, "cannot create handler")

	invalidMethods := []string{http.MethodConnect, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPut, http.MethodTrace}
	for _, method := range invalidMethods {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(method, channelparticipation.URLBaseV1, nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, fmt.Sprintf("invalid request method: %s", method), resp)
	}
}

func checkErrorResponse(t *testing.T, expectedCode int, expectedErrMsg string, resp *httptest.ResponseRecorder) {
	assert.Equal(t, expectedCode, resp.Code)

	header := resp.Header()
	headerArray, headerOK := header["Content-Type"]
	assert.True(t, headerOK)
	require.Len(t, headerArray, 1)
	assert.Equal(t, "application/json", headerArray[0])

	decoder := json.NewDecoder(resp.Body)
	respErr := &channelparticipation.ErrorResponse{}
	err := decoder.Decode(respErr)
	assert.NoError(t, err)
	assert.Equal(t, expectedErrMsg, respErr.Error)
}
