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
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/hyperledger/fabric/internal/ccmetadata"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Platform for chaincodes written in Go
type Platform struct{}

// Returns whether the given file or directory exists or not
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
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

// Name returns the name of this platform
func (p *Platform) Name() string {
	return pb.ChaincodeSpec_GOLANG.String()
}

// ValidatePath validates Go chaincodes
func (p *Platform) ValidatePath(rawPath string) error {
	_, err := getCodeDescriptor(rawPath)
	if err != nil {
		return err
	}

	return nil
}

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

// Generates a deployment payload for GOLANG as a series of src/$pkg entries in .tar.gz format
func (p *Platform) GetDeploymentPayload(codepath string) ([]byte, error) {
	codeDescriptor, err := getCodeDescriptor(codepath)
	if err != nil {
		return nil, err
	}

	fileMap, err := findSource(codeDescriptor)
	if err != nil {
		return nil, err
	}

	packageInfo, err := dependencyPackageInfo(codeDescriptor.Pkg)
	if err != nil {
		return nil, err
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
