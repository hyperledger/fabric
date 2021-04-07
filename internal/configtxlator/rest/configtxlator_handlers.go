/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/configtxlator/update"
)

func fieldBytes(fieldName string, r *http.Request) ([]byte, error) {
	fieldFile, _, err := r.FormFile(fieldName)
	if err != nil {
		return nil, err
	}
	defer fieldFile.Close()

	return ioutil.ReadAll(fieldFile)
}

func fieldConfigProto(fieldName string, r *http.Request) (*cb.Config, error) {
	fieldBytes, err := fieldBytes(fieldName, r)
	if err != nil {
		return nil, fmt.Errorf("error reading field bytes: %s", err)
	}

	config := &cb.Config{}
	err = proto.Unmarshal(fieldBytes, config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling field bytes: %s", err)
	}

	return config, nil
}

func ComputeUpdateFromConfigs(w http.ResponseWriter, r *http.Request) {
	originalConfig, err := fieldConfigProto("original", r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error with field 'original': %s\n", err)
		return
	}

	updatedConfig, err := fieldConfigProto("updated", r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error with field 'updated': %s\n", err)
		return
	}

	configUpdate, err := update.Compute(originalConfig, updatedConfig)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error computing update: %s\n", err)
		return
	}

	configUpdate.ChannelId = r.FormValue("channel")

	encoded, err := proto.Marshal(configUpdate)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error marshaling config update: %s\n", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(encoded)
}
