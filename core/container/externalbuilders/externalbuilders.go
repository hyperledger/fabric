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
	PackageID string
	BldDir    string
	Builder   *Builder
}

func (i *Instance) Start(peerConnection *ccintf.PeerConnection) error {
	return i.Builder.Run(i.PackageID, i.BldDir, peerConnection)
}

func (i *Instance) Stop() error {
	return errors.Errorf("stop is not implemented for external builders yet")
}

func (i *Instance) Wait() (int, error) {
	// Unimplemented, so, wait forever
	select {}
}

type BuildInfo struct {
	BuilderName string `json:"builder_name"`
}

type Detector struct {
	DurablePath string
	Builders    []*Builder
}

func (d *Detector) Detect(buildContext *BuildContext) *Builder {
	for _, builder := range d.Builders {
		if builder.Detect(buildContext) {
			return builder
		}
	}
	return nil
}

// CachedBuild returns a build instance that was already built, or nil, or
// when an unexpected error is encountered, an error.
func (d *Detector) CachedBuild(ccid string) (*Instance, error) {
	durablePath := filepath.Join(d.DurablePath, ccid)

	_, err := os.Stat(durablePath)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, errors.WithMessage(err, "existing build detected, but something went wrong inspecting it")
	}

	buildInfoPath := filepath.Join(durablePath, "build-info.json")
	buildInfoData, err := ioutil.ReadFile(buildInfoPath)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not read '%s' for build info", buildInfoPath)
	}

	buildInfo := &BuildInfo{}
	err = json.Unmarshal(buildInfoData, buildInfo)
	if err != nil {
		return nil, errors.WithMessagef(err, "malformed build info at '%s'", buildInfoPath)
	}

	for _, builder := range d.Builders {
		if builder.Name == buildInfo.BuilderName {
			return &Instance{
				PackageID: ccid,
				Builder:   builder,
				BldDir:    filepath.Join(durablePath, "bld"),
			}, nil
		}
	}

	return nil, errors.Errorf("chaincode '%s' was already built with builder '%s', but that builder is no longer available", ccid, buildInfo.BuilderName)
}

func (d *Detector) Build(ccid string, md *persistence.ChaincodePackageMetadata, codeStream io.Reader) (*Instance, error) {
	if len(d.Builders) == 0 {
		// A small optimization, especially while the launcher feature is under development
		// let's not explode the build package out into the filesystem unless there are
		// external builders to run against it.
		return nil, errors.Errorf("no builders defined")
	}

	i, err := d.CachedBuild(ccid)
	if err != nil {
		return nil, errors.WithMessage(err, "existing build could not be restored")
	}

	if i != nil {
		return i, nil
	}

	buildContext, err := NewBuildContext(ccid, md, codeStream)
	if err != nil {
		return nil, errors.WithMessage(err, "could not create build context")
	}

	defer buildContext.Cleanup()

	builder := d.Detect(buildContext)
	if builder == nil {
		return nil, errors.Errorf("no builder found")
	}

	if err := builder.Build(buildContext); err != nil {
		return nil, errors.WithMessage(err, "external builder failed to build")
	}

	if err := builder.Release(buildContext); err != nil {
		return nil, errors.WithMessage(err, "external builder failed to release")
	}

	durablePath := filepath.Join(d.DurablePath, ccid)

	err = os.Mkdir(durablePath, 0700)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not create dir '%s' to persist build ouput", durablePath)
	}

	buildInfo, err := json.Marshal(&BuildInfo{
		BuilderName: builder.Name,
	})
	if err != nil {
		os.RemoveAll(durablePath)
		return nil, errors.WithMessage(err, "could not marshal for build-info.json")
	}

	err = ioutil.WriteFile(filepath.Join(durablePath, "build-info.json"), buildInfo, 0600)
	if err != nil {
		os.RemoveAll(durablePath)
		return nil, errors.WithMessage(err, "could not write build-info.json")
	}

	err = os.Rename(buildContext.ReleaseDir, filepath.Join(durablePath, "release"))
	if err != nil {
		os.RemoveAll(durablePath)
		return nil, errors.WithMessagef(err, "could not move build context release to persistent location '%s'", durablePath)
	}

	durableBldDir := filepath.Join(durablePath, "bld")
	err = os.Rename(buildContext.BldDir, durableBldDir)
	if err != nil {
		os.RemoveAll(durablePath)
		return nil, errors.WithMessagef(err, "could not move build context bld to persistent location '%s'", durablePath)
	}

	return &Instance{
		PackageID: ccid,
		Builder:   builder,
		BldDir:    durableBldDir,
	}, nil
}

type BuildContext struct {
	CCID        string
	Metadata    *persistence.ChaincodePackageMetadata
	ScratchDir  string
	SourceDir   string
	ReleaseDir  string
	MetadataDir string
	BldDir      string
}

var pkgIDreg = regexp.MustCompile("[^a-zA-Z0-9]+")

func NewBuildContext(ccid string, md *persistence.ChaincodePackageMetadata, codePackage io.Reader) (bc *BuildContext, err error) {
	scratchDir, err := ioutil.TempDir("", "fabric-"+pkgIDreg.ReplaceAllString(ccid, "-"))
	if err != nil {
		return nil, errors.WithMessage(err, "could not create temp dir")
	}

	defer func() {
		if err != nil {
			os.RemoveAll(scratchDir)
		}
	}()

	sourceDir := filepath.Join(scratchDir, "src")
	if err = os.Mkdir(sourceDir, 0700); err != nil {
		return nil, errors.WithMessage(err, "could not create source dir")
	}

	metadataDir := filepath.Join(scratchDir, "metadata")
	if err = os.Mkdir(metadataDir, 0700); err != nil {
		return nil, errors.WithMessage(err, "could not create metadata dir")
	}

	outputDir := filepath.Join(scratchDir, "bld")
	if err = os.Mkdir(outputDir, 0700); err != nil {
		return nil, errors.WithMessage(err, "could not create build dir")
	}

	releaseDir := filepath.Join(scratchDir, "release")
	if err = os.Mkdir(releaseDir, 0700); err != nil {
		return nil, errors.WithMessage(err, "could not create release dir")
	}

	err = Untar(codePackage, sourceDir)
	if err != nil {
		return nil, errors.WithMessage(err, "could not untar source package")
	}

	err = writeMetadataFile(ccid, md, metadataDir)
	if err != nil {
		return nil, errors.WithMessage(err, "could not write metadata file")
	}

	return &BuildContext{
		ScratchDir:  scratchDir,
		SourceDir:   sourceDir,
		MetadataDir: metadataDir,
		BldDir:      outputDir,
		ReleaseDir:  releaseDir,
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

func CreateBuilders(builderConfs []peer.ExternalBuilder) []*Builder {
	builders := []*Builder{}
	for _, builderConf := range builderConfs {
		builders = append(builders, &Builder{
			Location:     builderConf.Path,
			Name:         builderConf.Name,
			EnvWhitelist: builderConf.EnvironmentWhitelist,
			Logger:       logger.Named(builderConf.Name),
		})
	}
	return builders
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
		logger.Debugf("Detection for builder '%s' detect failed: %s", b.Name, err)
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
		buildContext.BldDir,
	)

	err := RunCommand(b.Logger, cmd)
	if err != nil {
		return errors.Wrapf(err, "builder '%s' build failed", b.Name)
	}

	return nil
}

func (b *Builder) Release(buildContext *BuildContext) error {
	release := filepath.Join(b.Location, "bin", "release")

	_, err := os.Stat(release)
	if os.IsNotExist(err) {
		b.Logger.Debugf("Skipping release step for '%s' as no release binary found", buildContext.CCID)
		return nil
	}

	if err != nil {
		return errors.WithMessagef(err, "could not stat release binary '%s'", release)
	}

	cmd := NewCommand(
		release,
		b.EnvWhitelist,
		buildContext.BldDir,
		buildContext.ReleaseDir,
	)

	if err := RunCommand(b.Logger, cmd); err != nil {
		return errors.Wrapf(err, "builder '%s' release failed", b.Name)
	}

	return nil
}

// RunConfig is serialized to disk when launching
type RunConfig struct {
	CCID        string `json:"chaincode_id"`
	PeerAddress string `json:"peer_address"`
	ClientCert  []byte `json:"client_cert"`
	ClientKey   []byte `json:"client_key"`
	RootCert    []byte `json:"root_cert"`
}

func (b *Builder) Run(ccid, bldDir string, peerConnection *ccintf.PeerConnection) error {
	lc := &RunConfig{
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
		return errors.WithMessage(err, "could not marshal run config")
	}

	if err := ioutil.WriteFile(filepath.Join(launchDir, "chaincode.json"), marshaledLC, 0600); err != nil {
		return errors.WithMessage(err, "could not write root cert")
	}

	run := filepath.Join(b.Location, "bin", "run")
	cmd := NewCommand(
		run,
		b.EnvWhitelist,
		bldDir,
		launchDir,
	)

	err = RunCommand(b.Logger, cmd)
	if err != nil {
		return errors.Wrapf(err, "builder '%s' run failed", b.Name)
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
