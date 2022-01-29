/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	. "github.com/onsi/ginkgo/v2"
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

//go:generate mockery -dir . -name MetadataProvider -case underscore -output mock/ -outpkg mock

func TestPersistence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Persistence Suite")
}
