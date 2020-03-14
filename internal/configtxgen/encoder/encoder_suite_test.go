/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder_test

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEncoder(t *testing.T) {
	// Init the BCCSP
	err := factory.InitFactories(nil)
	if err != nil {
		panic(fmt.Errorf("Could not initialize BCCSP Factories [%s]", err))
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Encoder Suite")
}
