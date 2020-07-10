/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var (
	testProto = &cb.Block{
		Header: &cb.BlockHeader{
			PreviousHash: []byte("foo"),
		},
		Data: &cb.BlockData{
			Data: [][]byte{
				protoutil.MarshalOrPanic(&cb.Envelope{
					Payload: protoutil.MarshalOrPanic(&cb.Payload{
						Header: &cb.Header{
							ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
								Type: int32(cb.HeaderType_CONFIG),
							}),
						},
					}),
					Signature: []byte("bar"),
				}),
			},
		},
	}

	testOutput = `{"data":{"data":[{"payload":{"data":null,"header":{"channel_header":{"channel_id":"","epoch":"0","extension":null,"timestamp":null,"tls_cert_hash":null,"tx_id":"","type":1,"version":0},"signature_header":null}},"signature":"YmFy"}]},"header":{"data_hash":null,"number":"0","previous_hash":"Zm9v"},"metadata":null}`
)

func TestProtolatorDecode(t *testing.T) {
	data, err := proto.Marshal(testProto)
	require.NoError(t, err)

	url := fmt.Sprintf("/protolator/decode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader(data))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Remove all the whitespace
	compactJSON := strings.Replace(strings.Replace(strings.Replace(rec.Body.String(), "\n", "", -1), "\t", "", -1), " ", "", -1)

	require.Equal(t, testOutput, compactJSON)
}

func TestProtolatorEncode(t *testing.T) {

	url := fmt.Sprintf("/protolator/encode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte(testOutput)))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	outputMsg := &cb.Block{}

	err := proto.Unmarshal(rec.Body.Bytes(), outputMsg)
	require.NoError(t, err)
	require.True(t, proto.Equal(testProto, outputMsg))
}

func TestProtolatorDecodeNonExistantProto(t *testing.T) {
	req, _ := http.NewRequest("POST", "/protolator/decode/NonExistantMsg", bytes.NewReader([]byte{}))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProtolatorEncodeNonExistantProto(t *testing.T) {
	req, _ := http.NewRequest("POST", "/protolator/encode/NonExistantMsg", bytes.NewReader([]byte{}))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProtolatorDecodeBadData(t *testing.T) {
	url := fmt.Sprintf("/protolator/decode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte("Garbage")))

	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProtolatorEncodeBadData(t *testing.T) {
	url := fmt.Sprintf("/protolator/encode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte("Garbage")))

	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
