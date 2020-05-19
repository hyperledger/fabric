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
	"path"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/types"
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

	t.Run("on /channels/ch-id", func(t *testing.T) {
		for _, method := range invalidMethods {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(method, path.Join(channelparticipation.URLBaseV1Channels, "ch-id"), nil)
			h.ServeHTTP(resp, req)
			checkErrorResponse(t, http.StatusMethodNotAllowed, fmt.Sprintf("invalid request method: %s", method), resp)
			assert.Equal(t, "GET, POST, DELETE", resp.Header().Get("Allow"), "%s", method)
		}
	})

	t.Run("on /channels", func(t *testing.T) {
		invalidMethodsExt := append(invalidMethods, http.MethodDelete, http.MethodPost)
		for _, method := range invalidMethodsExt {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(method, channelparticipation.URLBaseV1Channels, nil)
			h.ServeHTTP(resp, req)
			checkErrorResponse(t, http.StatusMethodNotAllowed, fmt.Sprintf("invalid request method: %s", method), resp)
			assert.Equal(t, "GET", resp.Header().Get("Allow"), "%s", method)
		}
	})
}

func TestHTTPHandler_ServeHTTP_ListErrors(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
	h := channelparticipation.NewHTTPHandler(config, &mocks.ChannelManagement{})
	require.NotNilf(t, h, "cannot create handler")

	t.Run("bad base", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/oops", nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("missing channels collection", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1, nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("bad resource", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1+"oops", nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("bad channel ID", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/no/slash", nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("illegal character in channel ID", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/Oops", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid channel ID: 'Oops' contains illegal characters", resp)
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
		list := types.ChannelList{
			Channels: []types.ChannelInfoShort{
				{Name: "app-channel1", URL: ""},
				{Name: "app-channel2", URL: ""},
			},
			SystemChannel: &types.ChannelInfoShort{Name: "system-channel", URL: ""},
		}
		fakeManager.ChannelListReturns(list)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		assert.Equal(t, 2, len(listAll.Channels))
		assert.Equal(t, list.SystemChannel, listAll.SystemChannel)
		m := make(map[string]bool)
		for _, item := range listAll.Channels {
			m[item.Name] = true
			assert.Equal(t, channelparticipation.URLBaseV1Channels+"/"+item.Name, item.URL)
		}
		assert.True(t, m["app-channel1"])
		assert.True(t, m["app-channel2"])
	})

	t.Run("no channels, empty channels", func(t *testing.T) {
		list := types.ChannelList{
			SystemChannel: nil,
			Channels:      []types.ChannelInfoShort{},
		}
		fakeManager.ChannelListReturns(list)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		assert.Equal(t, 0, len(listAll.Channels))
		assert.NotNil(t, listAll.Channels)
		assert.Nil(t, listAll.SystemChannel)
	})

	t.Run("no channels", func(t *testing.T) {
		list := types.ChannelList{
			SystemChannel: nil,
			Channels:      nil,
		}
		fakeManager.ChannelListReturns(list)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		assert.Equal(t, 0, len(listAll.Channels))
		assert.Nil(t, listAll.Channels)
		assert.Nil(t, listAll.SystemChannel)
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
		}

		fakeManager.ChannelInfoReturns(info, nil)
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
		fakeManager.ChannelInfoReturns(types.ChannelInfo{}, errors.New("not found"))
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/app-channel", nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusNotFound, "not found", resp)
	})
}

func TestHTTPHandler_ServeHTTP_Join(t *testing.T) {
	t.Run("not implemented yet", func(t *testing.T) {
		config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
		fakeManager := &mocks.ChannelManagement{}
		h := channelparticipation.NewHTTPHandler(config, fakeManager)
		require.NotNilf(t, h, "cannot create handler")

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path.Join(channelparticipation.URLBaseV1Channels, "ch-id"), nil)
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusNotImplemented, "not implemented yet: POST /participation/v1/channels/ch-id", resp)
	})

	t.Run("content type mismatch", func(t *testing.T) {
		config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
		fakeManager := &mocks.ChannelManagement{}
		h := channelparticipation.NewHTTPHandler(config, fakeManager)
		require.NotNilf(t, h, "cannot create handler")

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path.Join(channelparticipation.URLBaseV1Channels, "ch-id"), nil)
		req.Header.Set("Content-Type", "text/plain")
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "unsupported Content-Type: [text/plain]", resp)
	})
}

func TestHTTPHandler_ServeHTTP_Remove(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true, RemoveStorage: false}
	fakeManager := &mocks.ChannelManagement{}
	h := channelparticipation.NewHTTPHandler(config, fakeManager)
	require.NotNilf(t, h, "cannot create handler")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, path.Join(channelparticipation.URLBaseV1Channels, "ch-id"), nil)
	h.ServeHTTP(resp, req)
	checkErrorResponse(t, http.StatusNotImplemented, "not implemented yet: DELETE /participation/v1/channels/ch-id", resp)
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
	assert.NoError(t, err, "body: %s", string(resp.Body.Bytes()))
	assert.Equal(t, expectedErrMsg, respErr.Error)
}
