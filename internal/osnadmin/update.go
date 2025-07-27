/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package osnadmin

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"
)

// Update an OSN to existing channel.
func Update(osnURL string, blockBytes []byte, caCertPool *x509.CertPool, tlsClientCert tls.Certificate, timeShift time.Duration) (*http.Response, error) {
	url := fmt.Sprintf("%s/participation/v1/channels", osnURL)
	req, err := createUpdateRequest(url, blockBytes)
	if err != nil {
		return nil, err
	}

	return httpDoTimeShift(req, caCertPool, tlsClientCert, timeShift)
}

func createUpdateRequest(url string, envelopeBytes []byte) (*http.Request, error) {
	updateBody := new(bytes.Buffer)
	writer := multipart.NewWriter(updateBody)
	part, err := writer.CreateFormFile("config-update-envelope", "config-update.envelope")
	if err != nil {
		return nil, err
	}
	part.Write(envelopeBytes)
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPut, url, updateBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req, nil
}
