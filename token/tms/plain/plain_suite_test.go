/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPlain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plain Suite")
}

func ToHex(q uint64) string {
	return "0x" + strconv.FormatUint(q, 16)
}

func ToDecimal(q uint64) string {
	return strconv.FormatUint(q, 10)
}

func IntToHex(q int64) string {
	return "0x" + strconv.FormatInt(q, 16)
}

func buildTokenOwnerString(raw []byte) string {
	owner := &token.TokenOwner{Type: token.TokenOwner_MSP_IDENTIFIER, Raw: raw}
	ownerString, err := plain.GetTokenOwnerString(owner)
	Expect(err).NotTo(HaveOccurred())

	return ownerString
}
