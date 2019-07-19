/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/core/common/ccprovider"

	"github.com/pkg/errors"
)

type BuildContext struct {
	ScratchDir string
	SourceDir  string
}

func NewBuildContext(ccci *ccprovider.ChaincodeContainerInfo, codePackage io.Reader) (*BuildContext, error) {
	// TODO, investigate if any other characters need sanitizing (we cannot have colons in the go path, for instance)
	scratchDir, err := ioutil.TempDir("", "fabric-"+strings.ReplaceAll(string(ccci.PackageID), string(os.PathListSeparator), "-"))
	if err != nil {
		return nil, errors.WithMessage(err, "could not create temp dir")
	}

	sourceDir := filepath.Join(scratchDir, "src")
	err = os.Mkdir(sourceDir, 0700)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not create source dir")
	}

	err = Untar(codePackage, sourceDir)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not untar source package")
	}

	return &BuildContext{
		ScratchDir: scratchDir,
		SourceDir:  sourceDir,
	}, nil
}

func (bc *BuildContext) Cleanup() {
	os.RemoveAll(bc.ScratchDir)
}
