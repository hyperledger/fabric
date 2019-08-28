/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/internal/ccmetadata"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Platform for chaincodes written in Go
type Platform struct{}

// Name returns the name of this platform.
func (p *Platform) Name() string {
	return pb.ChaincodeSpec_GOLANG.String()
}

// ValidatePath is used to ensure that path provided points to something that
// looks like go chainccode.
//
// NOTE: this is only used at the _client_ side by the peer CLI.
func (p *Platform) ValidatePath(rawPath string) error {
	_, err := getCodeDescriptor(rawPath)
	if err != nil {
		return err
	}

	return nil
}

// ValidateCodePackage examines the chaincode archive to ensure it is valid.
//
// NOTE: this is only used at the _client_ side by the peer CLI.
func (p *Platform) ValidateCodePackage(code []byte) error {
	is := bytes.NewReader(code)
	gr, err := gzip.NewReader(is)
	if err != nil {
		return fmt.Errorf("failure opening codepackage gzip stream: %s", err)
	}

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Only allow regular files without execute bit
		if header.Mode&^0100666 != 0 {
			return fmt.Errorf("illegal file mode detected for file %s: %o", header.Name, header.Mode)
		}
	}

	return nil
}

// GetDeploymentPayload creates a gzip compressed tape archive that contains the
// required assets to build and run go chaincode.
//
// NOTE: this is only used at the _client_ side by the peer CLI.
func (p *Platform) GetDeploymentPayload(codepath string) ([]byte, error) {
	codeDescriptor, err := getCodeDescriptor(codepath)
	if err != nil {
		return nil, err
	}

	fileMap, err := findSource(codeDescriptor)
	if err != nil {
		return nil, err
	}

	var packageInfo []PackageInfo
	for _, dist := range distributions() {
		pi, err := dependencyPackageInfo(dist.goos, dist.goarch, codeDescriptor.Pkg)
		if err != nil {
			return nil, err
		}
		packageInfo = append(packageInfo, pi...)
	}

	for _, pkg := range packageInfo {
		for _, filename := range pkg.Files() {
			filePath := filepath.Join(pkg.Dir, filename)
			sd := SourceDescriptor{
				Name:       path.Join("src", pkg.ImportPath, filename),
				Path:       filePath,
				IsMetadata: false,
			}
			fileMap[sd.Name] = sd
		}
	}

	// --------------------------------------------------------------------------------------
	// Write out our tar package
	// --------------------------------------------------------------------------------------
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	for _, file := range fileMap.values() {
		// If the file is metadata rather than golang code, remove the leading go code path, for example:
		// original file.Name:  src/github.com/hyperledger/fabric/examples/chaincode/go/marbles02/META-INF/statedb/couchdb/indexes/indexOwner.json
		// updated file.Name:   META-INF/statedb/couchdb/indexes/indexOwner.json
		if file.IsMetadata {
			// TODO: handle this much earlier - metadata is only gathered from the original directory
			file.Name, err = filepath.Rel(filepath.Join("src", codeDescriptor.Pkg), file.Name)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to calculate relative path for %s", file.Name)
			}

			// Split the tar location (file.Name) into a tar package directory and filename
			_, filename := filepath.Split(file.Name)

			// Hidden files are not supported as metadata, therefore ignore them.
			// User often doesn't know that hidden files are there, and may not be able to delete them, therefore warn user rather than error out.
			if strings.HasPrefix(filename, ".") {
				continue
			}

			fileBytes, err := ioutil.ReadFile(file.Path)
			if err != nil {
				return nil, err
			}

			// Validate metadata file for inclusion in tar
			// Validation is based on the passed filename with path
			err = ccmetadata.ValidateMetadataFile(file.Name, fileBytes)
			if err != nil {
				return nil, err
			}
		}

		err = util.WriteFileToPackage(file.Path, file.Name, tw)
		if err != nil {
			return nil, fmt.Errorf("Error writing %s to tar: %s", file.Name, err)
		}
	}

	err = tw.Close()
	if err == nil {
		err = gw.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create tar for chaincode")
	}

	return payload.Bytes(), nil
}

func (p *Platform) GenerateDockerfile() (string, error) {
	var buf []string
	buf = append(buf, "FROM "+util.GetDockerfileFromConfig("chaincode.golang.runtime"))
	buf = append(buf, "ADD binpackage.tar /usr/local/bin")

	return strings.Join(buf, "\n"), nil
}

const staticLDFlagsOpts = "-ldflags \"-linkmode external -extldflags '-static'\""
const dynamicLDFlagsOpts = ""

func getLDFlagsOpts() string {
	if viper.GetBool("chaincode.golang.dynamicLink") {
		return dynamicLDFlagsOpts
	}
	return staticLDFlagsOpts
}

func (p *Platform) DockerBuildOptions(pkg string) (util.DockerBuildOptions, error) {
	ldFlagOpts := getLDFlagsOpts()
	return util.DockerBuildOptions{
		Cmd: fmt.Sprintf("GOPATH=/chaincode/input:$GOPATH go build  %s -o /chaincode/output/chaincode %s", ldFlagOpts, pkg),
	}, nil
}

type CodeDescriptor struct {
	Gopath string
	Pkg    string
}

func getGopath() (string, error) {
	output, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return "", err
	}

	pathElements := filepath.SplitList(strings.TrimSpace(string(output)))
	if len(pathElements) == 0 {
		return "", fmt.Errorf("GOPATH is not set")
	}

	return pathElements[0], nil
}

// getCodeDescriptor returns GOPATH and package information
func getCodeDescriptor(path string) (CodeDescriptor, error) {
	if path == "" {
		return CodeDescriptor{}, errors.New("cannot collect files from empty chaincode path")
	}

	gopath, err := getGopath()
	if err != nil {
		return CodeDescriptor{}, err
	}
	sourcePath := filepath.Join(gopath, "src", path)

	fi, err := os.Stat(sourcePath)
	if err != nil {
		return CodeDescriptor{}, errors.Wrap(err, "failed to get code")
	}
	if !fi.IsDir() {
		return CodeDescriptor{}, errors.Errorf("path is not a directory: %s", path)
	}

	return CodeDescriptor{Gopath: gopath, Pkg: path}, nil
}

type SourceDescriptor struct {
	Name       string
	Path       string
	IsMetadata bool
}

type Sources []SourceDescriptor

func (s Sources) Len() int           { return len(s) }
func (s Sources) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s Sources) Less(i, j int) bool { return s[i].Name < s[j].Name }

type SourceMap map[string]SourceDescriptor

func (s SourceMap) values() Sources {
	var sources Sources
	for _, src := range s {
		sources = append(sources, src)
	}

	sort.Sort(sources)
	return sources
}

func findSource(cd CodeDescriptor) (SourceMap, error) {
	sources := SourceMap{}

	tld := filepath.Join(cd.Gopath, "src", cd.Pkg)
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Allow import of the top level chaincode directory into chaincode code package
			if path == tld {
				return nil
			}

			// Allow import of META-INF metadata directories into chaincode code package tar.
			// META-INF directories contain chaincode metadata artifacts such as statedb index definitions
			if isMetadataDir(path, tld) {
				return nil
			}

			// Do not import any other directories into chaincode code package
			return filepath.SkipDir
		}

		name, err := filepath.Rel(cd.Gopath, path)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate relative path for %s", path)
		}

		sources[name] = SourceDescriptor{Name: name, Path: path, IsMetadata: isMetadataDir(path, tld)}
		return nil
	}

	if err := filepath.Walk(tld, walkFn); err != nil {
		return nil, errors.Wrap(err, "walk failed")
	}

	return sources, nil
}

// isMetadataDir checks to see if the current path is in the META-INF directory at the root of the chaincode directory
func isMetadataDir(path, tld string) bool {
	return strings.HasPrefix(path, filepath.Join(tld, "META-INF"))
}

// dist holds go "distribution" infomration.
type dist struct{ goos, goarch string }

// distributions returns the OS and ARCH that we calcluate deps for.
func distributions() []dist {
	// linux architecutures
	dists := map[dist]bool{
		{goos: "linux", goarch: "amd64"}: true,
		{goos: "linux", goarch: "s390x"}: true,
	}

	// add local OS and ARCH
	dists[dist{goos: runtime.GOOS, goarch: runtime.GOARCH}] = true

	var list []dist
	for d := range dists {
		list = append(list, d)
	}

	return list
}
