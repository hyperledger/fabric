/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc_test

import (
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/scc/lscc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

//go:generate counterfeiter -o mock/sysccprovider.go --fake-name SystemChaincodeProvider . systemChaincodeProvider
type systemChaincodeProvider interface {
	sysccprovider.SystemChaincodeProvider
}

//go:generate counterfeiter -o mock/query_executor.go --fake-name QueryExecutor . queryExecutor
type queryExecutor interface {
	ledger.QueryExecutor
}

//go:generate counterfeiter -o mock/fs_support.go --fake-name FileSystemSupport . fileSystemSupport
type fileSystemSupport interface {
	lscc.FilesystemSupport
}

//go:generate counterfeiter -o mock/cc_package.go --fake-name CCPackage . ccPackage
type ccPackage interface {
	ccprovider.CCPackage
}

func TestLscc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lscc Suite")
}
