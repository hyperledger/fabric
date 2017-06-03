/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rest

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

var (
	testProto = &cb.Block{
		Header: &cb.BlockHeader{
			PreviousHash: []byte("foo"),
		},
		Data: &cb.BlockData{
			Data: [][]byte{
				utils.MarshalOrPanic(&cb.Envelope{
					Signature: []byte("bar"),
				}),
			},
		},
	}

	testOutput = `{"data":{"data":[{"signature":"YmFy"}]},"header":{"previous_hash":"Zm9v"}}`
)

func TestProtolatorDecode(t *testing.T) {
	data, err := proto.Marshal(testProto)
	assert.NoError(t, err)

	url := fmt.Sprintf("/protolator/decode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader(data))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Remove all the whitespace
	compactJSON := strings.Replace(strings.Replace(strings.Replace(rec.Body.String(), "\n", "", -1), "\t", "", -1), " ", "", -1)

	assert.Equal(t, testOutput, compactJSON)
}

func TestProtolatorEncode(t *testing.T) {

	url := fmt.Sprintf("/protolator/encode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte(testOutput)))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	outputMsg := &cb.Block{}

	err := proto.Unmarshal(rec.Body.Bytes(), outputMsg)
	assert.NoError(t, err)
	assert.Equal(t, testProto, outputMsg)
}

func TestProtolatorDecodeNonExistantProto(t *testing.T) {
	req, _ := http.NewRequest("POST", "/protolator/decode/NonExistantMsg", bytes.NewReader([]byte{}))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProtolatorEncodeNonExistantProto(t *testing.T) {
	req, _ := http.NewRequest("POST", "/protolator/encode/NonExistantMsg", bytes.NewReader([]byte{}))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProtolatorDecodeBadData(t *testing.T) {
	url := fmt.Sprintf("/protolator/decode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte("Garbage")))

	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProtolatorEncodeBadData(t *testing.T) {
	url := fmt.Sprintf("/protolator/encode/%s", proto.MessageName(testProto))

	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte("Garbage")))

	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
