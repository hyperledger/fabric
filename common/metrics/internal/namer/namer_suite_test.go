/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package namer_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestNamer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Namer Suite")
}
