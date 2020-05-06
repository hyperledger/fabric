/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"encoding/json"
	"net/http"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
)

const URLBaseV1 = "/participation/v1/"

type ErrorResponse struct {
	Error string `json:"error"`
}

//go:generate counterfeiter -o mocks/channel_management.go -fake-name ChannelManagement . ChannelManagement

type ChannelManagement interface {
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
	case http.MethodGet, http.MethodPost, http.MethodDelete:
		//TODO skeleton
		err := errors.Errorf("not implemented yet: %s %s", req.Method, req.URL.String())
		h.sendResponseJsonError(resp, http.StatusNotImplemented, err)

	default:
		err := errors.Errorf("invalid request method: %s", req.Method)
		h.sendResponseJsonError(resp, http.StatusBadRequest, err)
	}
}

func (h *HTTPHandler) sendResponseJsonError(resp http.ResponseWriter, code int, err error) {
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(code)
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(&ErrorResponse{Error: err.Error()}); err != nil {
		h.logger.Errorw("failed to encode payload", "error", err)
	}
}
