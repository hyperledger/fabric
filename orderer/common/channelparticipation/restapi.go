/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
)

const URLBaseV1 = "/participation/v1/"
const URLBaseV1Channels = URLBaseV1 + "channels"

//go:generate counterfeiter -o mocks/channel_management.go -fake-name ChannelManagement . ChannelManagement

type ChannelManagement interface {
	// ListAll returns the names of all channels (including the system channel), and the name of the system channel
	// (empty if does not exist). The URL field is empty, and is to be completed by the caller.
	ListAllChannels() ([]ChannelInfoShort, string)

	// ListChannel provides extended status information about a channel.
	// The URL field is empty, and is to be completed by the caller.
	ListChannel(channelID string) (*ChannelInfoFull, error)

	// TODO skeleton
}

// HTTPHandler handles all the HTTP requests to the channel participation API.
type HTTPHandler struct {
	logger    *flogging.FabricLogger
	config    localconfig.ChannelParticipation
	registrar ChannelManagement
	// TODO skeleton
}

func NewHTTPHandler(config localconfig.ChannelParticipation, registrar ChannelManagement) *HTTPHandler {
	return &HTTPHandler{
		logger:    flogging.MustGetLogger("orderer.commmon.channelparticipation"),
		config:    config,
		registrar: registrar,
	}
}

func (h *HTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if !h.config.Enabled {
		err := errors.New("channel participation API is disabled")
		h.sendResponseJsonError(resp, http.StatusServiceUnavailable, err)
		return
	}

	switch req.Method {
	case http.MethodGet:
		_, err := negotiateContentType(req) // Only application/json for now
		if err != nil {
			h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
			return
		}

		channelID, all, err := detectResource(req.URL.Path)
		if err != nil {
			h.sendResponseJsonError(resp, http.StatusBadRequest, err)
		}

		// Asking for a list of all channels
		if all {
			var channels *ListAllChannels = &ListAllChannels{}
			channels.Channels, channels.SystemChannel = h.registrar.ListAllChannels()
			channels.Size = len(channels.Channels)
			for i, info := range channels.Channels {
				channels.Channels[i].URL = URLBaseV1Channels + `/` + info.Name
			}
			h.sendResponseOK(resp, channels)
			return
		}

		// Asking for a single channel
		if err = configtx.ValidateChannelID(channelID); err != nil {
			h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "invalid channel ID"))
			return
		}
		infoFull, err := h.registrar.ListChannel(channelID)
		if err != nil {
			h.sendResponseJsonError(resp, http.StatusNotFound, err)
			return
		}
		h.sendResponseOK(resp, infoFull)

	case http.MethodPost, http.MethodDelete:
		//TODO skeleton
		err := errors.Errorf("not implemented yet: %s %s", req.Method, req.URL.String())
		h.sendResponseJsonError(resp, http.StatusNotImplemented, err)

	default:
		err := errors.Errorf("invalid request method: %s", req.Method)
		h.sendResponseJsonError(resp, http.StatusBadRequest, err)
	}
}

func negotiateContentType(req *http.Request) (string, error) {
	acceptReq := req.Header.Get("Accept")
	if len(acceptReq) == 0 {
		return "application/json", nil
	}

	options := strings.Split(acceptReq, ",")
	for _, opt := range options {
		if strings.Contains(opt, "application/json") ||
			strings.Contains(opt, "application/*") ||
			strings.Contains(opt, "*/*") {
			return "application/json", nil
		}
	}

	return "", errors.New("response Content-Type is application/json only")
}

func detectResource(path string) (string, bool, error) {
	nakedPath := strings.TrimSuffix(path, "/")

	if !strings.HasPrefix(path, URLBaseV1Channels) {
		return "", false, errors.Errorf("invalid path: %s", path)
	}
	channel := strings.TrimPrefix(nakedPath, URLBaseV1Channels)

	if len(channel) == 0 {
		return "", true, nil
	}

	return channel[1:], false, nil
}

func (h *HTTPHandler) sendResponseJsonError(resp http.ResponseWriter, code int, err error) {
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(code)
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(&ErrorResponse{Error: err.Error()}); err != nil {
		h.logger.Errorf("failed to encode error, err: %s", err)
	}
}

func (h *HTTPHandler) sendResponseOK(resp http.ResponseWriter, content interface{}) {
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(http.StatusOK)
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(content); err != nil {
		h.logger.Errorf("failed to encode content, err: %s", err)
	}
}
