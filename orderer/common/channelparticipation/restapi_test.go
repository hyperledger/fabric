/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation_test

import (
	"encoding/json"
	"fmt"
	"github.com/hyperledger/fabric/orderer/common/types"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
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

func TestHTTPHandler_ServeHTTP_ListErrors(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
	h := channelparticipation.NewHTTPHandler(config, &mocks.ChannelManagement{})
	require.NotNilf(t, h, "cannot create handler")

	t.Run("bad base", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/oops", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid path: /oops", resp)
	})

	t.Run("missing channels collection", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1, nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid path: /participation/v1/", resp)
	})

	t.Run("bad resource", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1+"oops", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid path: /participation/v1/oops", resp)
	})

	t.Run("bad channel ID", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/Oops", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid channel ID: 'Oops' contains illegal characters", resp)

		resp = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/no/slash", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid channel ID: 'no/slash' contains illegal characters", resp)
	})

	t.Run("bad Accept header", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/ok", nil)
		req.Header.Set("Accept", "text/html")
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusNotAcceptable, "response Content-Type is application/json only", resp)
	})
}

func TestHTTPHandler_ServeHTTP_ListAll(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
	fakeManager := &mocks.ChannelManagement{}
	h := channelparticipation.NewHTTPHandler(config, fakeManager)
	require.NotNilf(t, h, "cannot create handler")

	t.Run("two channels", func(t *testing.T) {
		list := []types.ChannelInfoShort{
			{Name: "app-channel", URL: ""},
			{Name: "system-channel", URL: ""}}
		fakeManager.ListAllChannelsReturns(list, "system-channel")
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		assert.Equal(t, 2, listAll.Size)
		assert.Equal(t, 2, len(listAll.Channels))
		assert.Equal(t, "system-channel", listAll.SystemChannel)
		m := make(map[string]bool)
		for _, item := range listAll.Channels {
			m[item.Name] = true
			assert.Equal(t, channelparticipation.URLBaseV1Channels+"/"+item.Name, item.URL)
		}
		assert.True(t, m["system-channel"])
		assert.True(t, m["app-channel"])
	})

	t.Run("no channels", func(t *testing.T) {
		list := []types.ChannelInfoShort{}
		fakeManager.ListAllChannelsReturns(list, "")
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		assert.Equal(t, 0, listAll.Size)
		assert.Equal(t, 0, len(listAll.Channels))
		assert.Equal(t, "", listAll.SystemChannel)
	})
}

func TestHTTPHandler_ServeHTTP_ListSingle(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
	fakeManager := &mocks.ChannelManagement{}
	h := channelparticipation.NewHTTPHandler(config, fakeManager)
	require.NotNilf(t, h, "cannot create handler")

	t.Run("channel exists", func(t *testing.T) {
		info := types.ChannelInfo{
			Name:            "app-channel",
			URL:             channelparticipation.URLBaseV1Channels + "/app-channel",
			ClusterRelation: "member",
			Status:          "active",
			Height:          3,
			BlockHash:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		}

		fakeManager.ListChannelReturns(&info, nil)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/app-channel", nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))

		infoResp := types.ChannelInfo{}
		err := json.Unmarshal(resp.Body.Bytes(), &infoResp)
		require.NoError(t, err, "cannot be unmarshaled")
		assert.Equal(t, info, infoResp)

	})

	t.Run("channel does not exists", func(t *testing.T) {
		fakeManager.ListChannelReturns(nil, errors.New("not found"))
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/app-channel", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusNotFound, "not found", resp)
	})
}

func checkErrorResponse(t *testing.T, expectedCode int, expectedErrMsg string, resp *httptest.ResponseRecorder) {
	assert.Equal(t, expectedCode, resp.Code)

	header := resp.Header()
	headerArray, headerOK := header["Content-Type"]
	assert.True(t, headerOK)
	require.Len(t, headerArray, 1)
	assert.Equal(t, "application/json", headerArray[0])

	decoder := json.NewDecoder(resp.Body)
	respErr := &types.ErrorResponse{}
	err := decoder.Decode(respErr)
	assert.NoError(t, err)
	assert.Equal(t, expectedErrMsg, respErr.Error)
}
