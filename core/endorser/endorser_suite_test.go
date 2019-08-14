/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEndorser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endorser Suite")
}
