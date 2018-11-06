/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/issuer.go -fake-name Issuer . Issuer
//go:generate counterfeiter -o mock/issuer_secret_key.go -fake-name IssuerSecretKey . IssuerSecretKey
//go:generate counterfeiter -o mock/issuer_public_key.go -fake-name IssuerPublicKey . IssuerPublicKey
//go:generate counterfeiter -o mock/user.go -fake-name User . User
//go:generate counterfeiter -o mock/big.go -fake-name Big . Big

func TestPlain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plain Suite")
}
