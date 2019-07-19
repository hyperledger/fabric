/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/container/externalbuilders"
)

var _ = Describe("Tar", func() {
	Describe("ValidPath()", func() {
		It("validates that a path is relative and a child", func() {
			Expect(externalbuilders.ValidPath("a/simple/path")).To(BeTrue())
			Expect(externalbuilders.ValidPath("../path/to/parent")).To(BeFalse())
			Expect(externalbuilders.ValidPath("a/path/../with/intermediates")).To(BeTrue())
			Expect(externalbuilders.ValidPath("a/path/../../../with/toomanyintermediates")).To(BeFalse())
			Expect(externalbuilders.ValidPath("a/path/with/trailing/../..")).To(BeTrue())
			Expect(externalbuilders.ValidPath("a/path/with/toomanytrailing/../../../../..")).To(BeFalse())
			Expect(externalbuilders.ValidPath("/an/absolute/path")).To(BeFalse())
		})
	})

})
