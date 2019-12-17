/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"sync/atomic"
)

// TLSCertificates aggregates server and client TLS certificates
type TLSCertificates struct {
	TLSServerCert atomic.Value // *tls.Certificate server certificate of the peer
	TLSClientCert atomic.Value // *tls.Certificate client certificate of the peer
}
