/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDeliver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deliver Suite")
}
