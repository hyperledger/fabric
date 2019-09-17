/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

var (
	DefaultEnvWhitelist = []string{"GOCACHE", "HOME", "LD_LIBRARY_PATH", "LIBPATH", "PATH", "TMPDIR"}
	logger              = flogging.MustGetLogger("chaincode.externalbuilders")
)

const MetadataFile = "metadata.json"

type Instance struct {
	PackageID   string
	DurablePath string
	Builder     *Builder
}

func (i *Instance) Start(peerConnection *ccintf.PeerConnection) error {
	return i.Builder.Launch(i.PackageID, i.DurablePath, peerConnection)
}

func (i *Instance) Stop() error {
	return errors.Errorf("stop is not implemented for external builders yet")
}

func (i *Instance) Wait() (int, error) {
	// Unimplemented, so, wait forever
	select {}
}

type Detector struct {
	DurablePath string
	Builders    []peer.ExternalBuilder
}

func (d *Detector) Detect(buildContext *BuildContext) *Builder {
	for _, detectBuilder := range d.Builders {
		builder := &Builder{
			Location:     detectBuilder.Path,
			Logger:       logger.Named(filepath.Base(detectBuilder.Path)),
			Name:         detectBuilder.Name,
			EnvWhitelist: detectBuilder.EnvironmentWhitelist,
		}
		if builder.Detect(buildContext) {
			return builder
		}
	}

	return nil
}

func (d *Detector) Build(ccid string, md *persistence.ChaincodePackageMetadata, codeStream io.Reader) (*Instance, error) {
	if len(d.Builders) == 0 {
		// A small optimization, especially while the launcher feature is under development
		// let's not explode the build package out into the filesystem unless there are
		// external builders to run against it.
		return nil, errors.Errorf("no builders defined")
	}

	buildContext, err := NewBuildContext(string(ccid), md, codeStream)
	if err != nil {
		return nil, errors.WithMessage(err, "could not create build context")
	}

	defer buildContext.Cleanup()

	builder := d.Detect(buildContext)
	if builder == nil {
		buildContext.Cleanup()
		return nil, errors.Errorf("no builder found")
	}

	if err := builder.Build(buildContext); err != nil {
		buildContext.Cleanup()
		return nil, errors.WithMessage(err, "external builder failed to build")
	}

	durablePath := filepath.Join(d.DurablePath, ccid)

	// cleanup anything which is already persisted
	err = os.RemoveAll(durablePath)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not clean dir '%s' to persist build ouput", durablePath)
	}

	err = os.Mkdir(durablePath, 0700)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not create dir '%s' to persist build ouput", durablePath)
	}

	err = os.Rename(buildContext.OutputDir, filepath.Join(durablePath, "bld"))
	if err != nil {
		return nil, errors.WithMessagef(err, "could not move build context bld to persistent location '%s'", durablePath)
	}

	return &Instance{
		PackageID:   ccid,
		Builder:     builder,
		DurablePath: durablePath,
	}, nil
}

type BuildContext struct {
	CCID        string
	Metadata    *persistence.ChaincodePackageMetadata
	ScratchDir  string
	SourceDir   string
	MetadataDir string
	OutputDir   string
}

var pkgIDreg = regexp.MustCompile("[^a-zA-Z0-9]+")

func NewBuildContext(ccid string, md *persistence.ChaincodePackageMetadata, codePackage io.Reader) (*BuildContext, error) {
	scratchDir, err := ioutil.TempDir("", "fabric-"+pkgIDreg.ReplaceAllString(ccid, "-"))
	if err != nil {
		return nil, errors.WithMessage(err, "could not create temp dir")
	}

	sourceDir := filepath.Join(scratchDir, "src")
	err = os.Mkdir(sourceDir, 0700)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not create source dir")
	}

	metadataDir := filepath.Join(scratchDir, "metadata")
	err = os.Mkdir(metadataDir, 0700)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not create metadata dir")
	}

	outputDir := filepath.Join(scratchDir, "bld")
	err = os.Mkdir(outputDir, 0700)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not create build dir")
	}

	err = Untar(codePackage, sourceDir)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not untar source package")
	}

	err = writeMetadataFile(ccid, md, metadataDir)
	if err != nil {
		os.RemoveAll(scratchDir)
		return nil, errors.WithMessage(err, "could not write metadata file")
	}

	return &BuildContext{
		ScratchDir:  scratchDir,
		SourceDir:   sourceDir,
		MetadataDir: metadataDir,
		OutputDir:   outputDir,
		Metadata:    md,
		CCID:        ccid,
	}, nil
}

type buildMetadata struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	PackageID string `json:"package_id"`
}

func writeMetadataFile(ccid string, md *persistence.ChaincodePackageMetadata, dst string) error {
	buildMetadata := &buildMetadata{
		Path:      md.Path,
		Type:      md.Type,
		PackageID: ccid,
	}
	mdBytes, err := json.Marshal(buildMetadata)
	if err != nil {
		return errors.Wrap(err, "failed to marshal build metadata into JSON")
	}

	return ioutil.WriteFile(filepath.Join(dst, MetadataFile), mdBytes, 0700)
}

func (bc *BuildContext) Cleanup() {
	os.RemoveAll(bc.ScratchDir)
}

type Builder struct {
	EnvWhitelist []string
	Location     string
	Logger       *flogging.FabricLogger
	Name         string
}

func (b *Builder) Detect(buildContext *BuildContext) bool {
	detect := filepath.Join(b.Location, "bin", "detect")
	cmd := NewCommand(
		detect,
		b.EnvWhitelist,
		buildContext.SourceDir,
		buildContext.MetadataDir,
	)

	err := RunCommand(b.Logger, cmd)
	if err != nil {
		logger.Debugf("Detection for builder '%s' failed: %s", b.Name, err)
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
		b.EnvWhitelist,
		buildContext.SourceDir,
		buildContext.MetadataDir,
		buildContext.OutputDir,
	)

	err := RunCommand(b.Logger, cmd)
	if err != nil {
		return errors.Wrapf(err, "builder '%s' failed", b.Name)
	}

	return nil
}

// LaunchConfig is serialized to disk when launching
type LaunchConfig struct {
	CCID        string `json:"chaincode_id"`
	PeerAddress string `json:"peer_address"`
	ClientCert  []byte `json:"client_cert"`
	ClientKey   []byte `json:"client_key"`
	RootCert    []byte `json:"root_cert"`
}

func (b *Builder) Launch(ccid, durablePath string, peerConnection *ccintf.PeerConnection) error {
	lc := &LaunchConfig{
		PeerAddress: peerConnection.Address,
		CCID:        ccid,
	}

	if peerConnection.TLSConfig != nil {
		lc.ClientCert = peerConnection.TLSConfig.ClientCert
		lc.ClientKey = peerConnection.TLSConfig.ClientKey
		lc.RootCert = peerConnection.TLSConfig.RootCert
	}

	launchDir, err := ioutil.TempDir("", "fabric-run")
	if err != nil {
		return errors.WithMessage(err, "could not create temp run dir")
	}

	marshaledLC, err := json.Marshal(lc)
	if err != nil {
		return errors.WithMessage(err, "could not marshal launch config")
	}

	if err := ioutil.WriteFile(filepath.Join(launchDir, "chaincode.json"), marshaledLC, 0600); err != nil {
		return errors.WithMessage(err, "could not write root cert")
	}

	launch := filepath.Join(b.Location, "bin", "launch")
	cmd := NewCommand(
		launch,
		b.EnvWhitelist,
		durablePath,
		launchDir,
	)

	err = RunCommand(b.Logger, cmd)
	if err != nil {
		return errors.Wrapf(err, "builder '%s' failed", b.Name)
	}

	return nil
}

// NewCommand creates an exec.Cmd that is configured to prune the calling
// environment down to LD_LIBRARY_PATH, LIBPATH, PATH, and TMPDIR environment
// variables.
func NewCommand(name string, envWhiteList []string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	whitelist := appendDefaultWhitelist(envWhiteList)
	for _, key := range whitelist {
		if val, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
		}
	}
	return cmd
}

func appendDefaultWhitelist(envWhitelist []string) []string {
	for _, variable := range DefaultEnvWhitelist {
		if !contains(envWhitelist, variable) {
			envWhitelist = append(envWhitelist, variable)
		}
	}
	return envWhitelist
}

func contains(envWhiteList []string, key string) bool {
	for _, variable := range envWhiteList {
		if key == variable {
			return true
		}
	}
	return false
}

func RunCommand(logger *flogging.FabricLogger, cmd *exec.Cmd) error {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	is := bufio.NewReader(stderr)
	for done := false; !done; {
		// read output line by line
		line, err := is.ReadString('\n')
		switch err {
		case nil:
			logger.Info(strings.TrimSuffix(line, "\n"))
		case io.EOF:
			if len(line) > 0 {
				logger.Info(line)
			}
			done = true
		default:
			logger.Error("error reading command output", err)
			return err
		}
	}

	return cmd.Wait()
}
