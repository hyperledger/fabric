/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"encoding/json"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
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
	URLBaseV1              = "/participation/v1/"
	URLBaseV1Channels      = URLBaseV1 + "channels"
	FormDataConfigBlockKey = "config-block"
	RemoveStorageQueryKey  = "removeStorage"

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
	JoinChannel(channelID string, configBlock *cb.Block, isAppChannel bool) (types.ChannelInfo, error)

	// RemoveChannel instructs the orderer to remove a channel.
	// Depending on the removeStorage parameter, the storage resources are either removed or archived.
	RemoveChannel(channelID string, removeStorage bool) error
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

	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveJoin).Methods(http.MethodPost).HeadersRegexp(
		"Content-Type", "multipart/form-data*")
	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveBadContentType).Methods(http.MethodPost)

	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveRemove).Methods(http.MethodDelete)
	handler.router.HandleFunc(urlWithChannelIDKey, handler.serveNotAllowed)

	handler.router.HandleFunc(URLBaseV1Channels, handler.serveListAll).Methods(http.MethodGet)
	handler.router.HandleFunc(URLBaseV1Channels, handler.serveNotAllowed)

	handler.router.HandleFunc(URLBaseV1, handler.redirectBaseV1).Methods(http.MethodGet)

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
	resp.Header().Set("Cache-Control", "no-store")
	h.sendResponseOK(resp, channelList)
}

// List a single channel
func (h *HTTPHandler) serveListOne(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json responses for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	channelID, err := h.extractChannelID(req, resp)
	if err != nil {
		return
	}

	infoFull, err := h.registrar.ChannelInfo(channelID)
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotFound, err)
		return
	}
	resp.Header().Set("Cache-Control", "no-store")
	h.sendResponseOK(resp, infoFull)
}

func (h *HTTPHandler) redirectBaseV1(resp http.ResponseWriter, req *http.Request) {
	http.Redirect(resp, req, URLBaseV1Channels, http.StatusFound)
}

// Join a channel.
// Expect multipart/form-data.
func (h *HTTPHandler) serveJoin(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json responses for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	channelID, err := h.extractChannelID(req, resp)
	if err != nil {
		return
	}

	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot parse Mime media type"))
		return
	}

	block := h.multipartFormDataBodyToBlock(params, req, resp)
	if block == nil {
		return
	}

	isAppChannel, err := ValidateJoinBlock(channelID, block)
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "invalid join block"))
		return
	}

	info, err := h.registrar.JoinChannel(channelID, block, isAppChannel)
	if err == nil {
		info.URL = path.Join(URLBaseV1Channels, info.Name)
		h.logger.Debugf("Successfully joined channel: %s", info)
		h.sendResponseCreated(resp, info.URL, info)
		return
	}

	h.sendJoinError(err, resp)
}

// Expect a multipart/form-data with a single part, of type file, with key FormDataConfigBlockKey.
func (h *HTTPHandler) multipartFormDataBodyToBlock(params map[string]string, req *http.Request, resp http.ResponseWriter) *cb.Block {
	boundary := params["boundary"]
	reader := multipart.NewReader(req.Body, boundary)
	form, err := reader.ReadForm(100 * 1024 * 1024)
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot read form from request body"))
		return nil
	}

	if _, exist := form.File[FormDataConfigBlockKey]; !exist {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Errorf("form does not contains part key: %s", FormDataConfigBlockKey))
		return nil
	}

	if len(form.File) != 1 || len(form.Value) != 0 {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.New("form contains too many parts"))
		return nil
	}

	fileHeader := form.File[FormDataConfigBlockKey][0]
	file, err := fileHeader.Open()
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrapf(err, "cannot open file part %s from request body", FormDataConfigBlockKey))
		return nil
	}

	blockBytes, err := ioutil.ReadAll(file)
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrapf(err, "cannot read file part %s from request body", FormDataConfigBlockKey))
		return nil
	}

	block := &cb.Block{}
	err = proto.Unmarshal(blockBytes, block)
	if err != nil {
		h.logger.Debugf("Failed to unmarshal blockBytes: %s", err)
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrapf(err, "cannot unmarshal file part %s into a block", FormDataConfigBlockKey))
		return nil
	}

	return block
}

func (h *HTTPHandler) extractChannelID(req *http.Request, resp http.ResponseWriter) (string, error) {
	channelID, ok := mux.Vars(req)[channelIDKey]
	if !ok {
		err := errors.New("missing channel ID")
		h.sendResponseJsonError(resp, http.StatusInternalServerError, err)
		return "", err
	}

	if err := configtx.ValidateChannelID(channelID); err != nil {
		err = errors.Wrap(err, "invalid channel ID")
		h.sendResponseJsonError(resp, http.StatusBadRequest, err)
		return "", err
	}
	return channelID, nil
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
// Expecting an optional query: "removeStorage=true" or "removeStorage=false".
func (h *HTTPHandler) serveRemove(resp http.ResponseWriter, req *http.Request) {
	_, err := negotiateContentType(req) // Only application/json responses for now
	if err != nil {
		h.sendResponseJsonError(resp, http.StatusNotAcceptable, err)
		return
	}

	channelID, err := h.extractChannelID(req, resp)
	if err != nil {
		return
	}

	removeStorage, err := h.extractRemoveStorageQuery(req, resp)
	if err != nil {
		return
	}

	err = h.registrar.RemoveChannel(channelID, removeStorage)
	if err == nil {
		h.logger.Debugf("Successfully removed channel: %s", channelID)
		resp.WriteHeader(http.StatusNoContent)
		return
	}

	h.logger.Debugf("Failed to remove channel: %s, err: %s", channelID, err)

	switch err {
	case types.ErrSystemChannelExists:
		h.sendResponseNotAllowed(resp, errors.Wrap(err, "cannot remove"), http.MethodGet)
	case types.ErrChannelNotExist:
		h.sendResponseJsonError(resp, http.StatusNotFound, errors.Wrap(err, "cannot remove"))
	default:
		h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot remove"))
	}
}

func (h *HTTPHandler) extractRemoveStorageQuery(req *http.Request, resp http.ResponseWriter) (bool, error) {
	removeStorage := h.config.RemoveStorage
	queryVal := req.URL.Query()
	if len(queryVal) > 1 {
		err := errors.New("cannot remove: too many query keys")
		h.sendResponseJsonError(resp, http.StatusBadRequest, err)
		return false, err
	}
	if values, ok := queryVal[RemoveStorageQueryKey]; ok {
		var err error
		if len(values) != 1 {
			err = errors.New("cannot remove: too many query parameters")
			h.sendResponseJsonError(resp, http.StatusBadRequest, err)
			return false, err
		}

		removeStorage, err = strconv.ParseBool(values[0])
		if err != nil {
			h.sendResponseJsonError(resp, http.StatusBadRequest, errors.Wrap(err, "cannot remove: invalid query parameter"))
			return false, err
		}
	} else if len(queryVal) > 0 {
		err := errors.New("cannot remove: invalid query key")
		h.sendResponseJsonError(resp, http.StatusBadRequest, err)
		return false, err
	}
	return removeStorage, nil
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
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(code)
	if err := encoder.Encode(&types.ErrorResponse{Error: err.Error()}); err != nil {
		h.logger.Errorf("failed to encode error, err: %s", err)
	}
}

func (h *HTTPHandler) sendResponseOK(resp http.ResponseWriter, content interface{}) {
	encoder := json.NewEncoder(resp)
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(http.StatusOK)
	if err := encoder.Encode(content); err != nil {
		h.logger.Errorf("failed to encode content, err: %s", err)
	}
}

func (h *HTTPHandler) sendResponseCreated(resp http.ResponseWriter, location string, content interface{}) {
	encoder := json.NewEncoder(resp)
	resp.Header().Set("Location", location)
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(http.StatusCreated)
	if err := encoder.Encode(content); err != nil {
		h.logger.Errorf("failed to encode content, err: %s", err)
	}
}

func (h *HTTPHandler) sendResponseNotAllowed(resp http.ResponseWriter, err error, allow ...string) {
	encoder := json.NewEncoder(resp)
	allowVal := strings.Join(allow, ", ")
	resp.Header().Set("Allow", allowVal)
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(http.StatusMethodNotAllowed)
	if err := encoder.Encode(&types.ErrorResponse{Error: err.Error()}); err != nil {
		h.logger.Errorf("failed to encode error, err: %s", err)
	}
}
