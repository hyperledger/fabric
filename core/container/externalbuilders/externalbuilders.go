/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/ccprovider"

	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("chaincode.externalbuilders")

type BuildContext struct {
	CCCI       *ccprovider.ChaincodeContainerInfo
	ScratchDir string
	SourceDir  string
	OutputDir  string
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

	outputDir := filepath.Join(scratchDir, "bld")
	err = os.Mkdir(outputDir, 0700)
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
		OutputDir:  outputDir,
		CCCI:       ccci,
	}, nil
}

func (bc *BuildContext) Cleanup() {
	os.RemoveAll(bc.ScratchDir)
}

type Builder struct {
	Location string
}

func (b *Builder) Detect(buildContext *BuildContext) bool {
	detect := filepath.Join(b.Location, "bin", "detect")
	cmd := exec.Cmd{
		Path: detect,
		Args: []string{
			"detect", // the first arg is the binary name we are invoking as
			"--package-id", string(buildContext.CCCI.PackageID),
			// XXX long term, do we want to include path and type?
			// the whole idea of detect was to determine these things, I thought
			// same goes for other builder functions like Build
			"--path", buildContext.CCCI.Path,
			"--type", buildContext.CCCI.Type,
			"--source", buildContext.SourceDir,
		},

		// TODO wire the stderr of the process back to the peer logs
		// same goes for other builder functions like Build
	}

	err := cmd.Run()
	if err != nil {
		logger.Debugf("Detection for builder '%s' failed: %s", b.Name(), err)
		// XXX, we probably also want to differentiate between a 'not detected'
		// and a 'I failed nastily', but, again, good enough for now
		return false
	}

	return true
}

func (b *Builder) Build(buildContext *BuildContext) error {
	build := filepath.Join(b.Location, "bin", "build")
	cmd := exec.Cmd{
		Path: build,
		Args: []string{
			"build", // the first arg is the binary name we are invoking as
			"--package-id", string(buildContext.CCCI.PackageID),
			"--path", buildContext.CCCI.Path,
			"--type", buildContext.CCCI.Type,
			"--source", buildContext.SourceDir,
			"--output", buildContext.OutputDir,
		},
	}

	err := cmd.Run()
	if err != nil {
		return errors.Errorf("builder '%s' failed: %s", b.Name(), err)
	}

	return nil
}

// Name is a convenience method and uses the base name of the builder
// for identification purposes.
func (b *Builder) Name() string {
	return filepath.Base(b.Location)
}
