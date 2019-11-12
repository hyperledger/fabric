/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hyperledger/fabric/common/flogging"
)

type VersionInfoHandler struct {
	CommitSHA string `json:"CommitSHA,omitempty"`
	Version   string `json:"Version,omitempty"`
}

func (m *VersionInfoHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		m.sendResponse(resp, http.StatusOK, m)
	default:
		err := fmt.Errorf("invalid request method: %s", req.Method)
		m.sendResponse(resp, http.StatusBadRequest, err)
	}
}

type errorResponse struct {
	Error string `json:"Error"`
}

func (m *VersionInfoHandler) sendResponse(resp http.ResponseWriter, code int, payload interface{}) {
	if err, ok := payload.(error); ok {
		payload = &errorResponse{Error: err.Error()}
	}
	js, err := json.Marshal(payload)
	if err != nil {
		logger := flogging.MustGetLogger("operations.runner")
		logger.Errorw("failed to encode payload", "error", err)
		resp.WriteHeader(http.StatusInternalServerError)
	} else {
		resp.WriteHeader(code)
		resp.Header().Set("Content-Type", "application/json")
		resp.Write(js)
	}
}
