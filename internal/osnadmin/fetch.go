/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package osnadmin

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"
)

// Fetch get block from OSN.
func Fetch(osnURL, channelID string, blockID string, caCertPool *x509.CertPool, tlsClientCert tls.Certificate, timeShift time.Duration) (*http.Response, error) {
	url := fmt.Sprintf("%s/participation/v1/channels/%s/blocks/%s", osnURL, channelID, blockID)

	return httpGetTimeShift(url, caCertPool, tlsClientCert, timeShift)
}
