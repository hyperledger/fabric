/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRootScope(t *testing.T) {
	s := NewRootScope()
	assert.NotNil(t, s)
}

func TestNewRootScopeConcurrent(t *testing.T) {
	var s1 Scope
	var s2 Scope
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		s1 = NewRootScope()
		wg.Done()
	}()
	go func() {
		s2 = NewRootScope()
		wg.Done()
	}()
	wg.Wait()
	assert.Exactly(t, &s1, &s2)
}

func TestClose(t *testing.T) {
	NewRootScope()
	assert.NotPanics(t, func() {
		Close()
	})
}
