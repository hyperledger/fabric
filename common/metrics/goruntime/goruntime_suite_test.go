/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package goruntime_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRuntime(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runtime Suite")
}
