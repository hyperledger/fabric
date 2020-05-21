/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
	cb "github.com/hyperledger/fabric-protos-go/common"
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

	// JoinChannel instructs the orderer to create a channel and join it with the provided config block.
	JoinChannel(channelID string, configBlock *cb.Block) (types.ChannelInfo, error)
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
	_, err := negotiateContentType(req) // Only application/json responses for now
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
	_, err := negotiateContentType(req) // Only application/json responses for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	channelID, ok := h.extractChannelID(req, resp)
	if !ok {
		return
	}

	infoFull, err := h.registrar.ChannelInfo(channelID)
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotFound, err)
		return
	}
	h.sendResponseOK(resp, infoFull)
}

// Join a channel. Expect application/json content.
func (h *HTTPHandler) serveJoin(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json responses for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	channelID, ok := h.extractChannelID(req, resp)
	if !ok {
		return
	}

	block, ok := h.jsonBodyToBlock(req, resp) //TODO add support for application/octet-stream & multipart/form-data
	if !ok {
		return
	}

	info, err := h.registrar.JoinChannel(channelID, block)
	if err == nil {
		info.URL = path.Join(URLBaseV1Channels, info.Name)
		h.logger.Debugf("Successfully joined channel: %s", info)
		h.sendResponseCreated(resp, info.URL, info)
		return
	}

	h.sendJoinError(err, resp)
}

func (h *HTTPHandler) jsonBodyToBlock(req *http.Request, resp http.ResponseWriter) (*cb.Block, bool) {
	block := &cb.Block{}

	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		h.logger.Debugf("Failed to read request body: %s", err)
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot read request body"))
		return nil, false
	}

	body := types.JoinBody{}
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		h.logger.Debugf("Failed to json.Unmarshal request body: %s", err)
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot json.Unmarshal request body"))
		return nil, false
	}

	err = proto.Unmarshal(body.ConfigBlock, block)
	if err != nil {
		h.logger.Debugf("Failed to unmarshal ConfigBlock field: %s", err)
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot unmarshal ConfigBlock field"))
		return nil, false
	}

	return block, true
}

func (h *HTTPHandler) extractChannelID(req *http.Request, resp http.ResponseWriter) (string, bool) {
	channelID, ok := mux.Vars(req)[channelIDKey]
	if !ok {
		h.sendResponseJsonError(resp, http.StatusInternalServerError, errors.New("missing channel ID"))
		return "", false
	}

	if err := configtx.ValidateChannelID(channelID); err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "invalid channel ID"))
		return "", false
	}
	return channelID, true
}

func (h *HTTPHandler) sendJoinError(err error, resp http.ResponseWriter) {
	h.logger.Debugf("Failed to JoinChannel: %s", err)
	switch err {
	case types.ErrSystemChannelExists:
		// The client is trying to join an app-channel, but the system channel exists: only GET is allowed on app channels.
		h.sendResponseNotAllowed(resp, errors.Wrap(err, "cannot join"), http.MethodGet)
	case types.ErrChannelAlreadyExists:
		// The client is trying to join an app-channel that exists, but the system channel does not;
		// The client is trying to join the system-channel, and it exists. GET & DELETE are allowed on the channel.
		h.sendResponseNotAllowed(resp, errors.Wrap(err, "cannot join"), http.MethodGet, http.MethodDelete)
	case types.ErrAppChannelsAlreadyExists:
		// The client is trying to join the system-channel that does not exist, but app channels exist.
		h.sendResponseJsonError(resp, http.StatusForbidden, errors.Wrap(err, "cannot join"))
	default:
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot join"))
	}
}

// Remove a channel
func (h *HTTPHandler) serveRemove(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json responses for now
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

	if _, ok := mux.Vars(req)[channelIDKey]; ok {
		h.sendResponseNotAllowed(resp, err, http.MethodGet, http.MethodPost, http.MethodDelete)
		return
	}

	h.sendResponseNotAllowed(resp, err, http.MethodGet)
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

func (h *HTTPHandler) sendResponseCreated(resp http.ResponseWriter, location string, content interface{}) {
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(http.StatusCreated)
	resp.Header().Set("Location", location)
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(content); err != nil {
		h.logger.Errorf("failed to encode content, err: %s", err)
	}
}

func (h *HTTPHandler) sendResponseNotAllowed(resp http.ResponseWriter, err error, allow ...string) {
	encoder := json.NewEncoder(resp)
	resp.WriteHeader(http.StatusMethodNotAllowed)
	allowVal := strings.Join(allow, ", ")
	resp.Header().Set("Allow", allowVal)
	resp.Header().Set("Content-Type", "application/json")
	if err := encoder.Encode(&types.ErrorResponse{Error: err.Error()}); err != nil {
		h.logger.Errorf("failed to encode error, err: %s", err)
	}
}
