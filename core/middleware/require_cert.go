/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware

import (
	"net/http"
)

type requireCert struct {
	next http.Handler
}

// RequireCert is used to ensure that a verified TLS client certificate was
// used for authentication.
func RequireCert() Middleware {
	return func(next http.Handler) http.Handler {
		return &requireCert{next: next}
	}
}

func (r *requireCert) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.TLS == nil:
		fallthrough
	case len(req.TLS.VerifiedChains) == 0:
		fallthrough
	case len(req.TLS.VerifiedChains[0]) == 0:
		w.WriteHeader(http.StatusUnauthorized)
	default:
		r.next.ServeHTTP(w, req)
	}
}
