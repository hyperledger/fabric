/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEncoder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Encoder Suite")
}
