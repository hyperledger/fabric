/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package disabled_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDisabled(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Disabled Suite")
}
