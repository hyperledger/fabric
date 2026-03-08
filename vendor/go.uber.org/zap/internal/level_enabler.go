<<<<<<<< HEAD:vendor/go.uber.org/multierr/error_post_go120.go
// Copyright (c) 2017-2023 Uber Technologies, Inc.
========
// Copyright (c) 2022 Uber Technologies, Inc.
>>>>>>>> main:vendor/go.uber.org/zap/internal/level_enabler.go
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

<<<<<<<< HEAD:vendor/go.uber.org/multierr/error_post_go120.go
//go:build go1.20
// +build go1.20

package multierr

// Unwrap returns a list of errors wrapped by this multierr.
func (merr *multiError) Unwrap() []error {
	return merr.Errors()
}

type multipleErrors interface {
	Unwrap() []error
}

func extractErrors(err error) []error {
	if err == nil {
		return nil
	}

	// check if the given err is an Unwrapable error that
	// implements multipleErrors interface.
	eg, ok := err.(multipleErrors)
	if !ok {
		return []error{err}
	}

	return append(([]error)(nil), eg.Unwrap()...)
========
// Package internal and its subpackages hold types and functionality
// that are not part of Zap's public API.
package internal

import "go.uber.org/zap/zapcore"

// LeveledEnabler is an interface satisfied by LevelEnablers that are able to
// report their own level.
//
// This interface is defined to use more conveniently in tests and non-zapcore
// packages.
// This cannot be imported from zapcore because of the cyclic dependency.
type LeveledEnabler interface {
	zapcore.LevelEnabler

	Level() zapcore.Level
>>>>>>>> main:vendor/go.uber.org/zap/internal/level_enabler.go
}
