/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/ioreadwriter.go -fake-name IOReadWriter . ioReadWriter
type ioReadWriter interface {
	persistence.IOReadWriter
}

//go:generate counterfeiter -o mock/osfileinfo.go -fake-name OSFileInfo . osFileInfo
type osFileInfo interface {
	os.FileInfo
}

//go:generate counterfeiter -o mock/store_package_provider.go -fake-name StorePackageProvider . storePackageProvider
type storePackageProvider interface {
	persistence.StorePackageProvider
}

//go:generate counterfeiter -o mock/legacy_package_provider.go -fake-name LegacyPackageProvider . legacyPackageProvider
type legacyPackageProvider interface {
	persistence.LegacyPackageProvider
}

//go:generate counterfeiter -o mock/package_parser.go -fake-name PackageParser . packageParser
type packageParser interface {
	persistence.PackageParser
}

func TestPersistence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Persistence Suite")
}
