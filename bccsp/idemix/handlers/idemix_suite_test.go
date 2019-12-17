/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers_test

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
//go:generate counterfeiter -o mock/ecp.go -fake-name Ecp . Ecp
//go:generate counterfeiter -o mock/credrequest.go -fake-name CredRequest . CredRequest
//go:generate counterfeiter -o mock/credential.go -fake-name Credential . Credential
//go:generate counterfeiter -o mock/revocation.go -fake-name Revocation . Revocation
//go:generate counterfeiter -o mock/signature_scheme.go -fake-name SignatureScheme . SignatureScheme
//go:generate counterfeiter -o mock/nymsignature_scheme.go -fake-name NymSignatureScheme . NymSignatureScheme

func TestPlain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plain Suite")
}
