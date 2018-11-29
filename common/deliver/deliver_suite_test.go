/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/deliver"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/filtered_response_sender.go -fake-name FilteredResponseSender . filteredResponseSender
type filteredResponseSender interface {
	deliver.ResponseSender
	deliver.Filtered
}

func TestDeliver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deliver Suite")
}
