/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package container_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/vm_provider.go --fake-name VMProvider . VMProvider
//go:generate counterfeiter -o mock/vm.go --fake-name VM . VM
//go:generate counterfeiter -o mock/vm_req.go --fake-name VMCReq . VMCReq
//go:generate counterfeiter -o mock/builder.go --fake-name Builder . Builder

func TestContainer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Container Suite")
}
