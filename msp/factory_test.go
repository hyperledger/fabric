/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewInvalidOpts(t *testing.T) {
	i, err := New(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid msp.NewOpts instance. It must be either *BCCSPNewOpts or *IdemixNewOpts. It was [<nil>]")
	assert.Nil(t, i)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: -1}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid *BCCSPNewOpts. Version not recognized [-1]")
	assert.Nil(t, i)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: -1}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [-1]")
	assert.Nil(t, i)
}

func TestNew(t *testing.T) {
	i, err := New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_0}})
	assert.NoError(t, err)
	assert.NotNil(t, i)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_0}})
	assert.NoError(t, err)
	assert.NotNil(t, i)
}
