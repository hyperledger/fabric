/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/gorilla/mux"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/pkg/errors"
)

const (
	URLBaseV1           = "/participation/v1/"
	URLBaseV1Channels   = URLBaseV1 + "channels"
	channelIDKey        = "channelID"
	urlWithChannelIDKey = URLBaseV1Channels + "/{" + channelIDKey + "}"
)

//go:generate counterfeiter -o mocks/channel_management.go -fake-name ChannelManagement . ChannelManagement

type ChannelManagement interface {
	// ChannelList returns a slice of ChannelInfoShort containing all application channels (excluding the system
	// channel), and ChannelInfoShort of the system channel (nil if does not exist).
	// The URL fields are empty, and are to be completed by the caller.
	ChannelList() types.ChannelList

	// ChannelInfo provides extended status information about a channel.
	// The URL field is empty, and is to be completed by the caller.
	ChannelInfo(channelID string) (types.ChannelInfo, error)

	// TODO skeleton
}

// HTTPHandler handles all the HTTP requests to the channel participation API.
type HTTPHandler struct {
	logger    *flogging.FabricLogger
	config    localconfig.ChannelParticipation
	registrar ChannelManagement
	router    *mux.Router
	// TODO skeleton
}

func NewHTTPHandler(config localconfig.ChannelParticipation, registrar ChannelManagement) *HTTPHandler {
	handler := &HTTPHandler{
		logger:    flogging.MustGetLogger("orderer.commmon.channelparticipation"),
		config:    config,
		registrar: registrar,
		router:    mux.NewRouter(),
	}

	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveListOne).Methods(http.MethodGet)
	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveJoin).Methods(http.MethodPost).Headers(
		"Content-Type", "application/json") // TODO support application/octet-stream & multipart/form-data
	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveBadContentType).Methods(http.MethodPost)
	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveRemove).Methods(http.MethodDelete)
	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveNotAllowed)

	handler.router.HandleFunc(URLBaseV1Channels, handler.serveListAll).Methods("GET")
	handler.router.HandleFunc(URLBaseV1Channels, handler.serveNotAllowed)

	handler.router.Handle(URLBaseV1, nil) //TODO redirect to URLBaseV1Channels

	return handler
}

func (h *HTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if !h.config.Enabled {
		err := errors.New("channel participation API is disabled")
		h.sendResponseJsonError(resp, http.StatusServiceUnavailable, err)
		return
	}

	h.router.ServeHTTP(resp, req)
}

// List all channels
func (h *HTTPHandler) serveListAll(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}
	channelList := h.registrar.ChannelList()
	if channelList.SystemChannel != nil && channelList.SystemChannel.Name != "" {
		channelList.SystemChannel.URL = path.Join(URLBaseV1Channels, channelList.SystemChannel.Name)
	}
	for i, info := range channelList.Channels {
		channelList.Channels[i].URL = path.Join(URLBaseV1Channels, info.Name)
	}
	h.sendResponseOK(resp, channelList)
}

// List a single channel
func (h *HTTPHandler) serveListOne(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	channelID, ok := mux.Vars(req)[channelIDKey]
	if !ok {
		h.sendResponseJsonError(resp, http.StatusInternalServerError, errors.Wrap(err, "missing channel ID"))
		return
	}

	if err = configtx.ValidateChannelID(channelID); err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "invalid channel ID"))
		return
	}
	infoFull, err := h.registrar.ChannelInfo(channelID)
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotFound, err)
		return
	}
	h.sendResponseOK(resp, infoFull)
}

// Join a channel
func (h *HTTPHandler) serveJoin(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	//TODO
	err = errors.Errorf("not implemented yet: %s %s", req.Method, req.URL.String())
	h.sendResponseJsonError(resp, http.StatusNotImplemented, err)
}

// Remove a channel
func (h *HTTPHandler) serveRemove(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	//TODO
	err = errors.Errorf("not implemented yet: %s %s", req.Method, req.URL.String())
	h.sendResponseJsonError(resp, http.StatusNotImplemented, err)
}

func (h *HTTPHandler) serveBadContentType(resp http.ResponseWriter, req *http.Request) {
	err := errors.Errorf("unsupported Content-Type: %s", req.Header.Values("Content-Type"))
	h.sendResponseJsonError(resp, http.StatusBadRequest, err)
}

func (h *HTTPHandler) serveNotAllowed(resp http.ResponseWriter, req *http.Request) {
	err := errors.Errorf("invalid request method: %s", req.Method)
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(http.StatusMethodNotAllowed)
	if _, ok := mux.Vars(req)[channelIDKey]; ok {
		resp.Header().Set("Allow", "GET, POST, DELETE")
	} else {
		resp.Header().Set("Allow", "GET")
	}
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(&types.ErrorResponse{Error: err.Error()}); err != nil {
		h.logger.Errorf("failed to encode error, err: %s", err)
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

func (h *HTTPHandler) sendResponseJsonError(resp http.ResponseWriter, code int, err error) {
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(code)
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(&types.ErrorResponse{Error: err.Error()}); err != nil {
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
