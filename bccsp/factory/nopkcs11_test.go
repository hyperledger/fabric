//go:build !pkcs11
// +build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitFactories(t *testing.T) {
	err := initFactories(&FactoryOpts{
		Default: "SW",
		SW:      &SwOpts{},
	})
	require.EqualError(t, err, "Failed initializing BCCSP: Could not initialize BCCSP SW [Failed initializing configuration at [0,]: Hash Family not supported []]")

	err = initFactories(&FactoryOpts{
		Default: "PKCS11",
	})
	require.EqualError(t, err, "Could not find default `PKCS11` BCCSP")
}
