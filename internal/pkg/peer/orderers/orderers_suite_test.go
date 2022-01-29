/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderers_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOrderers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orderers Suite")
}
