/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge_test

import (
	"testing"

	"github.com/hyperledger/fabric-amcl/amcl"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPlain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plain Suite")
}

// NewRandPanic is an utility test function that always panic when invoked
func NewRandPanic() *amcl.RAND {
	panic("new rand panic")
}
