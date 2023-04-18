/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPHandler(t *testing.T) {
	config := localconfig.ChannelParticipation{
		Enabled: false,
	}
	h := channelparticipation.NewHTTPHandler(config, &mocks.ChannelManagement{})
	require.NotNilf(t, h, "cannot create handler")
}

func TestHTTPHandler_ServeHTTP_Disabled(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: false}
	_, h := setup(config, t)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", channelparticipation.URLBaseV1, nil)
	h.ServeHTTP(resp, req)
	checkErrorResponse(t, http.StatusServiceUnavailable, "channel participation API is disabled", resp)
}

func TestHTTPHandler_ServeHTTP_InvalidMethods(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true}
	_, h := setup(config, t)
	invalidMethods := []string{http.MethodConnect, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPut, http.MethodTrace}

	t.Run("on /channels/ch-id", func(t *testing.T) {
		invalidMethodsExt := append(invalidMethods, http.MethodPost)
		for _, method := range invalidMethodsExt {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(method, path.Join(channelparticipation.URLBaseV1Channels, "ch-id"), nil)
			h.ServeHTTP(resp, req)
			checkErrorResponse(t, http.StatusMethodNotAllowed, fmt.Sprintf("invalid request method: %s", method), resp)
			require.Equal(t, "GET, DELETE", resp.Result().Header.Get("Allow"), "%s", method)
		}
	})

	t.Run("on /channels", func(t *testing.T) {
		invalidMethodsExt := append(invalidMethods, http.MethodDelete)
		for _, method := range invalidMethodsExt {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(method, channelparticipation.URLBaseV1Channels, nil)
			h.ServeHTTP(resp, req)
			checkErrorResponse(t, http.StatusMethodNotAllowed, fmt.Sprintf("invalid request method: %s", method), resp)
			require.Equal(t, "GET, POST", resp.Result().Header.Get("Allow"), "%s", method)
		}
	})
}

func TestHTTPHandler_ServeHTTP_ListErrors(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true}
	_, h := setup(config, t)

	t.Run("bad base", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/oops", nil)
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusNotFound, resp.Result().StatusCode)
	})

	t.Run("bad resource", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1+"oops", nil)
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusNotFound, resp.Result().StatusCode)
	})

	t.Run("bad channel ID", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/no/slash", nil)
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusNotFound, resp.Result().StatusCode)
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
	config := localconfig.ChannelParticipation{Enabled: true}
	fakeManager, h := setup(config, t)

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
		require.Equal(t, http.StatusOK, resp.Result().StatusCode)
		require.Equal(t, "application/json", resp.Result().Header.Get("Content-Type"))
		require.Equal(t, "no-store", resp.Result().Header.Get("Cache-Control"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		require.Equal(t, 2, len(listAll.Channels))
		require.Equal(t, list.SystemChannel, listAll.SystemChannel)
		m := make(map[string]bool)
		for _, item := range listAll.Channels {
			m[item.Name] = true
			require.Equal(t, channelparticipation.URLBaseV1Channels+"/"+item.Name, item.URL)
		}
		require.True(t, m["app-channel1"])
		require.True(t, m["app-channel2"])
	})

	t.Run("no channels, empty channels", func(t *testing.T) {
		list := types.ChannelList{
			Channels: []types.ChannelInfoShort{},
		}
		fakeManager.ChannelListReturns(list)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Result().StatusCode)
		require.Equal(t, "application/json", resp.Result().Header.Get("Content-Type"))
		require.Equal(t, "no-store", resp.Result().Header.Get("Cache-Control"))

		listAll := &types.ChannelList{}
		err := json.Unmarshal(resp.Body.Bytes(), listAll)
		require.NoError(t, err, "cannot be unmarshaled")
		require.Equal(t, 0, len(listAll.Channels))
		require.NotNil(t, listAll.Channels)
		require.Nil(t, listAll.SystemChannel)
	})

	t.Run("no channels, Accept ok", func(t *testing.T) {
		list := types.ChannelList{}
		fakeManager.ChannelListReturns(list)

		for _, accept := range []string{"application/json", "application/*", "*/*"} {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels, nil)
			req.Header.Set("Accept", accept)
			h.ServeHTTP(resp, req)
			require.Equal(t, http.StatusOK, resp.Result().StatusCode, "Accept: %s", accept)
			require.Equal(t, "application/json", resp.Result().Header.Get("Content-Type"))
			require.Equal(t, "no-store", resp.Result().Header.Get("Cache-Control"))

			listAll := &types.ChannelList{}
			err := json.Unmarshal(resp.Body.Bytes(), listAll)
			require.NoError(t, err, "cannot be unmarshaled")
			require.Equal(t, 0, len(listAll.Channels))
			require.Nil(t, listAll.Channels)
			require.Nil(t, listAll.SystemChannel)
		}
	})

	t.Run("redirect from base V1 URL", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1, nil)
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusFound, resp.Result().StatusCode)
		require.Equal(t, channelparticipation.URLBaseV1Channels, resp.Result().Header.Get("Location"))
	})
}

func TestHTTPHandler_ServeHTTP_ListSingle(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true}
	fakeManager, h := setup(config, t)
	require.NotNilf(t, h, "cannot create handler")

	t.Run("channel exists", func(t *testing.T) {
		fakeManager.ChannelInfoReturns(types.ChannelInfo{
			Name:              "app-channel",
			ConsensusRelation: "consenter",
			Status:            "active",
			Height:            3,
		}, nil)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, channelparticipation.URLBaseV1Channels+"/app-channel", nil)
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Result().StatusCode)
		require.Equal(t, "application/json", resp.Result().Header.Get("Content-Type"))
		require.Equal(t, "no-store", resp.Result().Header.Get("Cache-Control"))

		infoResp := types.ChannelInfo{}
		err := json.Unmarshal(resp.Body.Bytes(), &infoResp)
		require.NoError(t, err, "cannot be unmarshaled")
		require.Equal(t, types.ChannelInfo{
			Name:              "app-channel",
			URL:               channelparticipation.URLBaseV1Channels + "/app-channel",
			ConsensusRelation: "consenter",
			Status:            "active",
			Height:            3,
		}, infoResp)
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
	config := localconfig.ChannelParticipation{
		Enabled:            true,
		MaxRequestBodySize: 1024 * 1024,
	}

	t.Run("created ok", func(t *testing.T) {
		fakeManager, h := setup(config, t)
		fakeManager.JoinChannelReturns(types.ChannelInfo{
			Name:              "app-channel",
			ConsensusRelation: "consenter",
			Status:            "active",
			Height:            1,
		}, nil)

		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, validBlockBytes("ch-id"))
		h.ServeHTTP(resp, req)
		require.Equal(t, http.StatusCreated, resp.Result().StatusCode)
		require.Equal(t, "application/json", resp.Result().Header.Get("Content-Type"))

		infoResp := types.ChannelInfo{}
		err := json.Unmarshal(resp.Body.Bytes(), &infoResp)
		require.NoError(t, err, "cannot be unmarshaled")
		require.Equal(t, types.ChannelInfo{
			Name:              "app-channel",
			URL:               channelparticipation.URLBaseV1Channels + "/app-channel",
			ConsensusRelation: "consenter",
			Status:            "active",
			Height:            1,
		}, infoResp)
	})

	t.Run("Error: system channel not supported", func(t *testing.T) {
		_, h := setup(config, t)

		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, sysChanBlockBytes("ch-id"))
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid join block: invalid config: contains consortiums: system channel not supported", resp)
	})

	t.Run("Error: Channel Exists", func(t *testing.T) {
		fakeManager, h := setup(config, t)
		fakeManager.JoinChannelReturns(types.ChannelInfo{}, types.ErrChannelAlreadyExists)
		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, validBlockBytes("ch-id"))
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusMethodNotAllowed, "cannot join: channel already exists", resp)
		require.Equal(t, "GET, DELETE", resp.Result().Header.Get("Allow"))
	})

	t.Run("Error: App Channels Exist", func(t *testing.T) {
		fakeManager, h := setup(config, t)
		fakeManager.JoinChannelReturns(types.ChannelInfo{}, types.ErrAppChannelsAlreadyExists)
		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, validBlockBytes("ch-id"))
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusForbidden, "cannot join: application channels already exist", resp)
	})

	t.Run("Error: Channel Pending Removal", func(t *testing.T) {
		fakeManager, h := setup(config, t)
		fakeManager.JoinChannelReturns(types.ChannelInfo{}, types.ErrChannelPendingRemoval)
		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, validBlockBytes("ch-id"))
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusConflict, "cannot join: channel pending removal", resp)
	})

	t.Run("bad body - not a block", func(t *testing.T) {
		_, h := setup(config, t)
		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, []byte{1, 2, 3, 4})
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "cannot unmarshal file part config-block into a block", resp)
	})

	t.Run("bad body - invalid join block", func(t *testing.T) {
		_, h := setup(config, t)
		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, []byte{})
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "invalid join block: block is not a config block", resp)
	})

	t.Run("content type mismatch", func(t *testing.T) {
		_, h := setup(config, t)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, channelparticipation.URLBaseV1Channels, nil)
		req.Header.Set("Content-Type", "text/plain")
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "unsupported Content-Type: [text/plain]", resp)
	})

	t.Run("form-data: bad form - no boundary", func(t *testing.T) {
		_, h := setup(config, t)
		resp := httptest.NewRecorder()

		joinBody := new(bytes.Buffer)
		writer := multipart.NewWriter(joinBody)
		part, err := writer.CreateFormFile(channelparticipation.FormDataConfigBlockKey, "join-config.block")
		require.NoError(t, err)
		part.Write([]byte{})
		err = writer.Close()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, channelparticipation.URLBaseV1Channels, joinBody)
		req.Header.Set("Content-Type", "multipart/form-data") // missing boundary

		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "cannot read form from request body: multipart: boundary is empty", resp)
	})

	t.Run("form-data: bad form - no key", func(t *testing.T) {
		_, h := setup(config, t)
		resp := httptest.NewRecorder()

		joinBody := new(bytes.Buffer)
		writer := multipart.NewWriter(joinBody)
		part, err := writer.CreateFormFile("bad-key", "join-config.block")
		require.NoError(t, err)
		part.Write([]byte{})
		err = writer.Close()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, channelparticipation.URLBaseV1Channels, joinBody)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "form does not contains part key: config-block", resp)
	})

	t.Run("form-data: bad form - too many parts", func(t *testing.T) {
		_, h := setup(config, t)
		resp := httptest.NewRecorder()

		joinBody := new(bytes.Buffer)
		writer := multipart.NewWriter(joinBody)
		part, err := writer.CreateFormFile(channelparticipation.FormDataConfigBlockKey, "join-config.block")
		require.NoError(t, err)
		part.Write([]byte{})
		part, err = writer.CreateFormField("not-wanted")
		require.NoError(t, err)
		part.Write([]byte("something"))
		err = writer.Close()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, channelparticipation.URLBaseV1Channels, joinBody)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "form contains too many parts", resp)
	})

	t.Run("body larger that MaxRequestBodySize", func(t *testing.T) {
		config := localconfig.ChannelParticipation{
			Enabled:            true,
			MaxRequestBodySize: 1,
		}
		_, h := setup(config, t)
		resp := httptest.NewRecorder()
		req := genJoinRequestFormData(t, []byte{1, 2, 3, 4})
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusBadRequest, "cannot read form from request body: multipart: NextPart: http: request body too large", resp)
	})
}

func TestHTTPHandler_ServeHTTP_Remove(t *testing.T) {
	config := localconfig.ChannelParticipation{Enabled: true}
	fakeManager, h := setup(config, t)

	type testDef struct {
		name         string
		channel      string
		fakeReturns  error
		expectedCode int
		expectedErr  error
	}

	testCases := []testDef{
		{
			name:         "success",
			channel:      "my-channel",
			fakeReturns:  nil,
			expectedCode: http.StatusNoContent,
			expectedErr:  nil,
		},
		{
			name:         "bad channel ID",
			channel:      "My-Channel",
			fakeReturns:  nil,
			expectedCode: http.StatusBadRequest,
			expectedErr:  errors.New("invalid channel ID: 'My-Channel' contains illegal characters"),
		},
		{
			name:         "channel does not exist",
			channel:      "my-channel",
			fakeReturns:  types.ErrChannelNotExist,
			expectedCode: http.StatusNotFound,
			expectedErr:  errors.Wrap(types.ErrChannelNotExist, "cannot remove"),
		},
		{
			name:         "some other error",
			channel:      "my-channel",
			fakeReturns:  os.ErrInvalid,
			expectedCode: http.StatusBadRequest,
			expectedErr:  errors.Wrap(os.ErrInvalid, "cannot remove"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fakeManager.RemoveChannelReturns(testCase.fakeReturns)
			resp := httptest.NewRecorder()
			target := path.Join(channelparticipation.URLBaseV1Channels, testCase.channel)
			req := httptest.NewRequest(http.MethodDelete, target, nil)
			h.ServeHTTP(resp, req)

			if testCase.expectedErr == nil {
				require.Equal(t, testCase.expectedCode, resp.Result().StatusCode)
				require.Equal(t, 0, resp.Body.Len(), "empty body")
			} else {
				checkErrorResponse(t, testCase.expectedCode, testCase.expectedErr.Error(), resp)
			}
		})
	}

	t.Run("Error: System Channel Exists", func(t *testing.T) {
		fakeManager, h := setup(config, t)
		fakeManager.RemoveChannelReturns(types.ErrSystemChannelExists)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, path.Join(channelparticipation.URLBaseV1Channels, "my-channel"), nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusMethodNotAllowed, "cannot remove: system channel exists", resp)
		require.Equal(t, "GET", resp.Result().Header.Get("Allow"))
	})

	t.Run("Error: Channel Pending Removal", func(t *testing.T) {
		fakeManager, h := setup(config, t)
		fakeManager.RemoveChannelReturns(types.ErrChannelPendingRemoval)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, path.Join(channelparticipation.URLBaseV1Channels, "my-channel"), nil)
		h.ServeHTTP(resp, req)
		checkErrorResponse(t, http.StatusConflict, "cannot remove: channel pending removal", resp)
	})
}

func setup(config localconfig.ChannelParticipation, t *testing.T) (*mocks.ChannelManagement, *channelparticipation.HTTPHandler) {
	fakeManager := &mocks.ChannelManagement{}
	h := channelparticipation.NewHTTPHandler(config, fakeManager)
	require.NotNilf(t, h, "cannot create handler")
	return fakeManager, h
}

func checkErrorResponse(t *testing.T, expectedCode int, expectedErrMsg string, resp *httptest.ResponseRecorder) {
	require.Equal(t, expectedCode, resp.Result().StatusCode)

	headerArray, headerOK := resp.Result().Header["Content-Type"]
	require.True(t, headerOK)
	require.Len(t, headerArray, 1)
	require.Equal(t, "application/json", headerArray[0])

	decoder := json.NewDecoder(resp.Body)
	respErr := &types.ErrorResponse{}
	err := decoder.Decode(respErr)
	require.NoError(t, err, "body: %s", resp.Body.String())
	require.Contains(t, respErr.Error, expectedErrMsg)
}

func genJoinRequestFormData(t *testing.T, blockBytes []byte) *http.Request {
	joinBody := new(bytes.Buffer)
	writer := multipart.NewWriter(joinBody)
	part, err := writer.CreateFormFile(channelparticipation.FormDataConfigBlockKey, "join-config.block")
	require.NoError(t, err)
	part.Write(blockBytes)
	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, channelparticipation.URLBaseV1Channels, joinBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func validBlockBytes(channelID string) []byte {
	blockBytes := protoutil.MarshalOrPanic(blockWithGroups(map[string]*common.ConfigGroup{
		"Application": {},
	}, channelID))
	return blockBytes
}

func sysChanBlockBytes(channelID string) []byte {
	blockBytes := protoutil.MarshalOrPanic(blockWithGroups(map[string]*common.ConfigGroup{
		"Consortiums": {},
	}, channelID))
	return blockBytes
}
