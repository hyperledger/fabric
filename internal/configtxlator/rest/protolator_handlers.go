/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/hyperledger/fabric-config/protolator"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func getMsgType(r *http.Request) (proto.Message, error) {
	vars := mux.Vars(r)
	msgName := vars["msgName"] // Will not arrive is unset

	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(msgName))
	if err != nil {
		return nil, errors.Wrapf(err, "message type not found")
	}

	msgType := reflect.TypeOf(proto.MessageV1(mt.Zero().Interface()))

	if msgType == nil {
		return nil, fmt.Errorf("message name not found")
	}
	return reflect.New(msgType.Elem()).Interface().(proto.Message), nil
}

func Decode(w http.ResponseWriter, r *http.Request) {
	msg, err := getMsgType(r)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, err)
		return
	}

	buf, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	err = proto.Unmarshal(buf, msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	var buffer bytes.Buffer
	err = protolator.DeepMarshalJSON(&buffer, msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	buffer.WriteTo(w)
}

func Encode(w http.ResponseWriter, r *http.Request) {
	msg, err := getMsgType(r)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, err)
		return
	}

	err = protolator.DeepUnmarshalJSON(r.Body, msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}
