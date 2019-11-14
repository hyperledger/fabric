/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type VersionInfoHandler struct {
	Logger      Logger
	VersionInfo *VersionInfo
}

type VersionInfo struct {
	CommitSHA string `json:"CommitSHA,omitempty"`
	Version   string `json:"Version,omitempty"`
}

func (m *VersionInfoHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		m.SendResponse(resp, http.StatusOK, m.VersionInfo)
	default:
		err := fmt.Errorf("invalid request method: %s", req.Method)
		m.SendResponse(resp, http.StatusBadRequest, err)
	}
}

type errorResponse struct {
	Error string `json:"Error"`
}

func (m *VersionInfoHandler) SendResponse(resp http.ResponseWriter, code int, payload interface{}) {
	if err, ok := payload.(error); ok {
		payload = &errorResponse{Error: err.Error()}
	}
	js, err := json.Marshal(payload)
	if err != nil {
		m.Logger.Errorf("failed to encode payload: %s", err)
		resp.WriteHeader(http.StatusInternalServerError)
	} else {
		resp.WriteHeader(code)
		resp.Header().Set("Content-Type", "application/json")
		resp.Write(js)
	}
}
