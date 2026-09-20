/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware

import (
	"net/http"
	"slices"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
)

var logger = flogging.MustGetLogger("middleware")

type Middleware func(http.Handler) http.Handler

// A Chain is a middleware chain use for http request processing.
type Chain struct {
	mw []Middleware
}

// NewChain creates a new Middleware chain. The chain will call the Middleware
// in the order provided.
func NewChain(middlewares ...Middleware) Chain {
	return Chain{
		mw: append([]Middleware{}, middlewares...),
	}
}

// Handler returns an http.Handler for this chain.
func (c Chain) Handler(h http.Handler) http.Handler {
	if h == nil {
		h = http.DefaultServeMux
	}

	for _, v := range slices.Backward(c.mw) {
		h = v(h)
	}
	return h
}
