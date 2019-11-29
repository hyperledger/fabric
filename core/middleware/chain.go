/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware

import (
	"net/http"
)

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

	for i := len(c.mw) - 1; i >= 0; i-- {
		h = c.mw[i](h)
	}
	return h
}
