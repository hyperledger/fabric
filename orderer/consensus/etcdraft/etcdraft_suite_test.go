/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package etcdraft_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEtcdraft(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcdraft Suite")
}
