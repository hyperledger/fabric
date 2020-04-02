/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder_test

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEncoder(t *testing.T) {
	err := factory.InitFactories(nil)
	assert.NoError(t, err, "Could not initialize BCCSP Factories")
	RegisterFailHandler(Fail)
	RunSpecs(t, "Encoder Suite")
}
