/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestExternalbuilders(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "External Builder Suite")
}
