/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/common/crypto"
)

//go:generate counterfeiter -o mock/local_signer.go --fake-name LocalSigner . localSigner
type localSigner interface {
	crypto.LocalSigner
}

func TestEncoder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Encoder Suite")
}
