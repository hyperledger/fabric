/*
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
)

var logger = flogging.MustGetLogger("chaincode.platform.node")

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

// Name returns the name of this platform
func (p *Platform) Name() string {
	return pb.ChaincodeSpec_NODE.String()
}

// ValidateSpec validates Go chaincodes
func (p *Platform) ValidatePath(rawPath string) error {
	path, err := url.Parse(rawPath)
	if err != nil || path == nil {
		return fmt.Errorf("invalid path: %s", err)
	}

	// Treat empty scheme as a local filesystem path
	if path.Scheme == "" {
		pathToCheck, err := filepath.Abs(rawPath)
		if err != nil {
			return fmt.Errorf("error obtaining absolute path of the chaincode: %s", err)
		}

		exists, err := pathExists(pathToCheck)
		if err != nil {
			return fmt.Errorf("error validating chaincode path: %s", err)
		}
		if !exists {
			return fmt.Errorf("path to chaincode does not exist: %s", rawPath)
		}
	}
	return nil
}

func (p *Platform) ValidateCodePackage(code []byte) error {
	// FAB-2122: Scan the provided tarball to ensure it only contains source-code under
	// the src folder.
	//
	// It should be noted that we cannot catch every threat with these techniques.  Therefore,
	// the container itself needs to be the last line of defense and be configured to be
	// resilient in enforcing constraints. However, we should still do our best to keep as much
	// garbage out of the system as possible.
	re := regexp.MustCompile(`^(/)?(src|META-INF)/.*`)
	is := bytes.NewReader(code)
	gr, err := gzip.NewReader(is)
	if err != nil {
		return fmt.Errorf("failure opening codepackage gzip stream: %s", err)
	}
	tr := tar.NewReader(gr)

	foundPackageJson := false
	for {
		header, err := tr.Next()
		if err != nil {
			// We only get here if there are no more entries to scan
			break
		}

		// --------------------------------------------------------------------------------------
		// Check name for conforming path
		// --------------------------------------------------------------------------------------
		if !re.MatchString(header.Name) {
			return fmt.Errorf("illegal file detected in payload: \"%s\"", header.Name)
		}
		if header.Name == "src/package.json" {
			foundPackageJson = true
		}
		// --------------------------------------------------------------------------------------
		// Check that file mode makes sense
		// --------------------------------------------------------------------------------------
		// Acceptable flags:
		//      ISREG      == 0100000
		//      -rw-rw-rw- == 0666
		//
		// Anything else is suspect in this context and will be rejected
		// --------------------------------------------------------------------------------------
		if header.Mode&^0o100666 != 0 {
			return fmt.Errorf("illegal file mode detected for file %s: %o", header.Name, header.Mode)
		}
	}
	if !foundPackageJson {
		return fmt.Errorf("no package.json found at the root of the chaincode package")
	}

	return nil
}

// Generates a deployment payload by putting source files in src/$file entries in .tar.gz format
func (p *Platform) GetDeploymentPayload(path string) ([]byte, error) {
	var err error

	// --------------------------------------------------------------------------------------
	// Write out our tar package
	// --------------------------------------------------------------------------------------
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	folder := path
	if folder == "" {
		return nil, errors.New("ChaincodeSpec's path cannot be empty")
	}

	// trim trailing slash if it exists
	if folder[len(folder)-1] == '/' {
		folder = folder[:len(folder)-1]
	}

	logger.Debugf("Packaging node.js project from path %s", folder)

	if err = util.WriteFolderToTarPackage(tw, folder, []string{"node_modules"}, nil, nil); err != nil {
		logger.Errorf("Error writing folder to tar package %s", err)
		return nil, fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	// Write the tar file out
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	tw.Close()
	gw.Close()

	return payload.Bytes(), nil
}

func (p *Platform) GenerateDockerfile() (string, error) {
	var buf []string

	buf = append(buf, "FROM "+util.GetDockerImageFromConfig("chaincode.node.runtime"))
	buf = append(buf, "ADD binpackage.tar /usr/local/src")

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

var buildScript = `
set -e
if [ -x /chaincode/build.sh ]; then
	/chaincode/build.sh
else
	cp -R /chaincode/input/src/. /chaincode/output && cd /chaincode/output && npm install --production
fi
`

func (p *Platform) DockerBuildOptions(path string) (util.DockerBuildOptions, error) {
	return util.DockerBuildOptions{
		Image: util.GetDockerImageFromConfig("chaincode.node.runtime"),
		Cmd:   buildScript,
	}, nil
}
