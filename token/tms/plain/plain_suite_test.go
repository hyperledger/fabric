/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"strconv"
	"testing"

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

func IntToHex(q int64) string {
	return "0x" + strconv.FormatInt(q, 16)
}
