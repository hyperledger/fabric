/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package packaging_test

import (
	"testing"

	"github.com/hyperledger/fabric/internal/peer/packaging"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/platform.go --fake-name Platform . platform
type platform interface {
	packaging.Platform
}

func TestPackaging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platforms Suite")
}
