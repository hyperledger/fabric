/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

//go:generate counterfeiter -o mock/instantiated_cc_store.go --fake-name InstantiatedChaincodeStore . instantiatedChaincodeStore
type instantiatedChaincodeStore interface {
	lifecycle.InstantiatedChaincodeStore
}

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}
