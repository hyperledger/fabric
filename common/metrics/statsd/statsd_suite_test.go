/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statsd_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestStatsd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Statsd Suite")
}
