/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"

	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("chaincode.externalbuilders")

type Instance struct {
	BuildContext *BuildContext
	Builder      *Builder
}

func (i *Instance) Start(peerConnection *ccintf.PeerConnection) error {
	return i.Builder.Launch(i.BuildContext, peerConnection)
}

func (i *Instance) Stop() error {
	return errors.Errorf("stop is not implemented for external builders yet")
}

func (i *Instance) Wait() (int, error) {
	// Unimplemented, so, wait forever
	select {}
}

type Detector struct {
	Builders []string
}

func (d *Detector) Detect(buildContext *BuildContext) *Builder {
	for _, builderLocation := range d.Builders {
		builder := &Builder{
			Location: builderLocation,
		}
		if builder.Detect(buildContext) {
			return builder
		}
	}

	return nil
}

func (d *Detector) Build(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) (container.Instance, error) {
	if len(d.Builders) == 0 {
		// A small optimization, especially while the launcher feature is under development
		// let's not explode the build package out into the filesystem unless there are
		// external builders to run against it.
		return nil, errors.Errorf("no builders defined")
	}

	buildContext, err := NewBuildContext(ccci, bytes.NewBuffer(codePackage))
	if err != nil {
		return nil, errors.WithMessage(err, "could not create build context")
	}

	builder := d.Detect(buildContext)
	if builder == nil {
		buildContext.Cleanup()
		return nil, errors.Errorf("no builder found")
	}

	if err := builder.Build(buildContext); err != nil {
		buildContext.Cleanup()
		return nil, errors.WithMessage(err, "external builder failed to build")
	}

	return &Instance{
		BuildContext: buildContext,
		Builder:      builder,
	}, nil
}

type BuildContext struct {
	CCCI       *ccprovider.ChaincodeContainerInfo
	ScratchDir string
	SourceDir  string
	OutputDir  string
	LaunchDir  string
}

var pkgIDreg = regexp.MustCompile("[^a-zA-Z0-9]+")

func NewBuildContext(ccci *ccprovider.ChaincodeContainerInfo, codePackage io.Reader) (*BuildContext, error) {
	scratchDir, err := ioutil.TempDir("", "fabric-"+pkgIDreg.ReplaceAllString(string(ccci.PackageID), "-"))
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

	launchDir := filepath.Join(scratchDir, "run")
	err = os.MkdirAll(launchDir, 0700)
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
		LaunchDir:  launchDir,
		CCCI:       ccci,
	}, nil
}

func (bc *BuildContext) Cleanup() {
	os.RemoveAll(bc.ScratchDir)
}

type Builder struct {
	Location string
	Logger   *flogging.FabricLogger
}

func (b *Builder) Detect(buildContext *BuildContext) bool {
	detect := filepath.Join(b.Location, "bin", "detect")
	cmd := NewCommand(
		detect,
		"--package-id", string(buildContext.CCCI.PackageID),
		"--path", buildContext.CCCI.Path,
		"--type", buildContext.CCCI.Type,
		"--source", buildContext.SourceDir,
	)

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
	cmd := NewCommand(
		build,
		"--package-id", string(buildContext.CCCI.PackageID),
		"--path", buildContext.CCCI.Path,
		"--type", buildContext.CCCI.Type,
		"--source", buildContext.SourceDir,
		"--output", buildContext.OutputDir,
	)

	err := cmd.Run()
	if err != nil {
		return errors.Errorf("builder '%s' failed: %s", b.Name(), err)
	}

	return nil
}

// LaunchConfig is serialized to disk when launching
type LaunchConfig struct {
	PeerAddress string `json:"PeerAddress"`
	ClientCert  []byte `json:"ClientCert"`
	ClientKey   []byte `json:"ClientKey"`
	RootCert    []byte `json:"RootCert"`
}

func (b *Builder) Launch(buildContext *BuildContext, peerConnection *ccintf.PeerConnection) error {
	lc := &LaunchConfig{
		PeerAddress: peerConnection.Address,
	}

	if peerConnection.TLSConfig != nil {
		lc.ClientCert = peerConnection.TLSConfig.ClientCert
		lc.ClientKey = peerConnection.TLSConfig.ClientKey
		lc.RootCert = peerConnection.TLSConfig.RootCert
	}

	marshaledLC, err := json.Marshal(lc)
	if err != nil {
		return errors.WithMessage(err, "could not marshal launch config")
	}

	if err := ioutil.WriteFile(filepath.Join(buildContext.LaunchDir, "chaincode.json"), marshaledLC, 0600); err != nil {
		return errors.WithMessage(err, "could not write root cert")
	}

	launch := filepath.Join(b.Location, "bin", "launch")
	cmd := NewCommand(
		launch,
		"--package-id", string(buildContext.CCCI.PackageID),
		"--path", buildContext.CCCI.Path,
		"--type", buildContext.CCCI.Type,
		"--source", buildContext.SourceDir,
		"--output", buildContext.OutputDir,
		"--artifacts", buildContext.LaunchDir,
	)

	err = cmd.Run()
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

// NewCommand creates an exec.Cmd that is configured to prune the calling
// environment down to LD_LIBRARY_PATH, LIBPATH, PATH, and TMPDIR environment
// variables.
func NewCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	for _, key := range []string{"LD_LIBRARY_PATH", "LIBPATH", "PATH", "TMPDIR"} {
		if val, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
		}
	}
	return cmd
}
