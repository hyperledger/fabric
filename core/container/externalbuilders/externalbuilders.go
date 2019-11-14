/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

var (
	DefaultEnvWhitelist = []string{"LD_LIBRARY_PATH", "LIBPATH", "PATH", "TMPDIR"}
	logger              = flogging.MustGetLogger("chaincode.externalbuilders")
)

const MetadataFile = "metadata.json"

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
	durablePath := filepath.Join(d.DurablePath, SanitizeCCIDPath(ccid))
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
				PackageID:   ccid,
				Builder:     builder,
				BldDir:      filepath.Join(durablePath, "bld"),
				TermTimeout: 5 * time.Second,
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
		return nil, nil
	}

	// Look for a cached instance.
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
		logger.Debugf("no external builder detected for %s", ccid)
		return nil, nil
	}

	if err := builder.Build(buildContext); err != nil {
		return nil, errors.WithMessage(err, "external builder failed to build")
	}

	if err := builder.Release(buildContext); err != nil {
		return nil, errors.WithMessage(err, "external builder failed to release")
	}

	durablePath := filepath.Join(d.DurablePath, SanitizeCCIDPath(ccid))

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
		PackageID:   ccid,
		Builder:     builder,
		BldDir:      durableBldDir,
		TermTimeout: 5 * time.Second,
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

func NewBuildContext(ccid string, md *persistence.ChaincodePackageMetadata, codePackage io.Reader) (bc *BuildContext, err error) {
	scratchDir, err := ioutil.TempDir("", "fabric-"+SanitizeCCIDPath(ccid))
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

func (bc *BuildContext) Cleanup() {
	os.RemoveAll(bc.ScratchDir)
}

var pkgIDreg = regexp.MustCompile("[<>:\"/\\\\|\\?\\*&]")

func SanitizeCCIDPath(ccid string) string {
	return pkgIDreg.ReplaceAllString(ccid, "-")
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

type Builder struct {
	EnvWhitelist []string
	Location     string
	Logger       *flogging.FabricLogger
	Name         string
}

func CreateBuilders(builderConfs []peer.ExternalBuilder) []*Builder {
	var builders []*Builder
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
	cmd := b.NewCommand(detect, buildContext.SourceDir, buildContext.MetadataDir)

	err := RunCommand(b.Logger, cmd)
	if err != nil {
		logger.Debugf("builder '%s' detect failed: %s", b.Name, err)
		return false
	}

	return true
}

func (b *Builder) Build(buildContext *BuildContext) error {
	build := filepath.Join(b.Location, "bin", "build")
	cmd := b.NewCommand(build, buildContext.SourceDir, buildContext.MetadataDir, buildContext.BldDir)

	err := RunCommand(b.Logger, cmd)
	if err != nil {
		return errors.Wrapf(err, "external builder '%s' failed", b.Name)
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

	cmd := b.NewCommand(release, buildContext.BldDir, buildContext.ReleaseDir)

	if err := RunCommand(b.Logger, cmd); err != nil {
		return errors.Wrapf(err, "builder '%s' release failed", b.Name)
	}

	return nil
}

// RunConfig is serialized to disk when launching
type RunConfig struct {
	CCID        string `json:"chaincode_id"`
	PeerAddress string `json:"peer_address"`
	ClientCert  string `json:"client_cert"` // PEM encoded client certifcate
	ClientKey   string `json:"client_key"`  // PEM encoded client key
	RootCert    string `json:"root_cert"`   // PEM encoded peer chaincode certificate
}

func (b *Builder) Run(ccid, bldDir string, peerConnection *ccintf.PeerConnection) (*Session, error) {
	lc := &RunConfig{
		PeerAddress: peerConnection.Address,
		CCID:        ccid,
	}

	if peerConnection.TLSConfig != nil {
		lc.ClientCert = string(peerConnection.TLSConfig.ClientCert)
		lc.ClientKey = string(peerConnection.TLSConfig.ClientKey)
		lc.RootCert = string(peerConnection.TLSConfig.RootCert)
	}

	launchDir, err := ioutil.TempDir("", "fabric-run")
	if err != nil {
		return nil, errors.WithMessage(err, "could not create temp run dir")
	}

	marshaledLC, err := json.Marshal(lc)
	if err != nil {
		return nil, errors.WithMessage(err, "could not marshal run config")
	}

	if err := ioutil.WriteFile(filepath.Join(launchDir, "chaincode.json"), marshaledLC, 0600); err != nil {
		return nil, errors.WithMessage(err, "could not write root cert")
	}

	run := filepath.Join(b.Location, "bin", "run")
	cmd := b.NewCommand(run, bldDir, launchDir)
	sess, err := Start(b.Logger, cmd)
	if err != nil {
		os.RemoveAll(launchDir)
		return nil, errors.Wrapf(err, "builder '%s' run failed to start", b.Name)
	}

	go func() {
		defer os.RemoveAll(launchDir)
		sess.Wait()
	}()

	return sess, nil
}

// NewCommand creates an exec.Cmd that is configured to prune the calling
// environment down to the environment variables specified in the external
// builder's EnvironmentWhitelist and the DefaultEnvWhitelist.
func (b *Builder) NewCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	whitelist := appendDefaultWhitelist(b.EnvWhitelist)
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
	sess, err := Start(logger, cmd)
	if err != nil {
		return err
	}
	return sess.Wait()
}
