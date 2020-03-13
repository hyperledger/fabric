/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestProtolatorComputeConfigUpdate(t *testing.T) {
	originalConfig := protoutil.MarshalOrPanic(&cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			ModPolicy: "foo",
		},
	})

	updatedConfig := protoutil.MarshalOrPanic(&cb.Config{
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
	updatedConfig := protoutil.MarshalOrPanic(&cb.Config{
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
	originalConfig := protoutil.MarshalOrPanic(&cb.Config{
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
