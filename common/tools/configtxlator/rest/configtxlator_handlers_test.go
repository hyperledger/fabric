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
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hyperledger/fabric/common/tools/configtxlator/sanitycheck"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestProtolatorComputeConfigUpdate(t *testing.T) {
	originalConfig := utils.MarshalOrPanic(&cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			ModPolicy: "foo",
		},
	})

	updatedConfig := utils.MarshalOrPanic(&cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			ModPolicy: "bar",
		},
	})

	buffer := &bytes.Buffer{}
	mpw := multipart.NewWriter(buffer)

	ffw, err := mpw.CreateFormFile("original", "foo")
	assert.NoError(t, err)
	_, err = bytes.NewReader(originalConfig).WriteTo(ffw)
	assert.NoError(t, err)

	ffw, err = mpw.CreateFormFile("updated", "bar")
	assert.NoError(t, err)
	_, err = bytes.NewReader(updatedConfig).WriteTo(ffw)
	assert.NoError(t, err)

	err = mpw.Close()
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "/configtxlator/compute/update-from-configs", buffer)
	assert.NoError(t, err)

	req.Header.Set("Content-Type", mpw.FormDataContentType())
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestProtolatorMissingOriginal(t *testing.T) {
	updatedConfig := utils.MarshalOrPanic(&cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			ModPolicy: "bar",
		},
	})

	buffer := &bytes.Buffer{}
	mpw := multipart.NewWriter(buffer)

	ffw, err := mpw.CreateFormFile("updated", "bar")
	assert.NoError(t, err)
	_, err = bytes.NewReader(updatedConfig).WriteTo(ffw)
	assert.NoError(t, err)

	err = mpw.Close()
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "/configtxlator/compute/update-from-configs", buffer)
	assert.NoError(t, err)

	req.Header.Set("Content-Type", mpw.FormDataContentType())
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProtolatorMissingUpdated(t *testing.T) {
	originalConfig := utils.MarshalOrPanic(&cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			ModPolicy: "bar",
		},
	})

	buffer := &bytes.Buffer{}
	mpw := multipart.NewWriter(buffer)

	ffw, err := mpw.CreateFormFile("original", "bar")
	assert.NoError(t, err)
	_, err = bytes.NewReader(originalConfig).WriteTo(ffw)
	assert.NoError(t, err)

	err = mpw.Close()
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "/configtxlator/compute/update-from-configs", buffer)
	assert.NoError(t, err)

	req.Header.Set("Content-Type", mpw.FormDataContentType())
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProtolatorCorruptProtos(t *testing.T) {
	originalConfig := []byte("Garbage")
	updatedConfig := []byte("MoreGarbage")

	buffer := &bytes.Buffer{}
	mpw := multipart.NewWriter(buffer)

	ffw, err := mpw.CreateFormFile("original", "bar")
	assert.NoError(t, err)
	_, err = bytes.NewReader(originalConfig).WriteTo(ffw)
	assert.NoError(t, err)

	ffw, err = mpw.CreateFormFile("updated", "bar")
	assert.NoError(t, err)
	_, err = bytes.NewReader(updatedConfig).WriteTo(ffw)
	assert.NoError(t, err)

	err = mpw.Close()
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "/configtxlator/compute/update-from-configs", buffer)
	assert.NoError(t, err)

	req.Header.Set("Content-Type", mpw.FormDataContentType())
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConfigtxlatorSanityCheckConfig(t *testing.T) {
	req, _ := http.NewRequest("POST", "/configtxlator/config/verify", bytes.NewReader(utils.MarshalOrPanic(&cb.Config{})))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	outputMsg := &sanitycheck.Messages{}

	err := json.Unmarshal(rec.Body.Bytes(), outputMsg)
	assert.NoError(t, err)
}

func TestConfigtxlatorSanityCheckMalformedConfig(t *testing.T) {
	req, _ := http.NewRequest("POST", "/configtxlator/config/verify", bytes.NewReader([]byte("Garbage")))
	rec := httptest.NewRecorder()
	r := NewRouter()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
