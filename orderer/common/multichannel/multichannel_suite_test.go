/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMultichannel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Multichannel Suite")
}
