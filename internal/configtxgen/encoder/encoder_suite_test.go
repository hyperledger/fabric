/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder_test

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEncoder(t *testing.T) {
	factory.InitFactories(nil)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Encoder Suite")
}
