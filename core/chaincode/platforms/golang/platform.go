/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package golang

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Platform for chaincodes written in Go
type Platform struct {
}

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

func decodeUrl(spec *pb.ChaincodeSpec) (string, error) {
	var urlLocation string
	if strings.HasPrefix(spec.ChaincodeID.Path, "http://") {
		urlLocation = spec.ChaincodeID.Path[7:]
	} else if strings.HasPrefix(spec.ChaincodeID.Path, "https://") {
		urlLocation = spec.ChaincodeID.Path[8:]
	} else {
		urlLocation = spec.ChaincodeID.Path
	}

	if urlLocation == "" {
		return "", errors.New("ChaincodeSpec's path/URL cannot be empty")
	}

	if strings.LastIndex(urlLocation, "/") == len(urlLocation)-1 {
		urlLocation = urlLocation[:len(urlLocation)-1]
	}

	return urlLocation, nil
}

// ValidateSpec validates Go chaincodes
func (goPlatform *Platform) ValidateSpec(spec *pb.ChaincodeSpec) error {
	path, err := url.Parse(spec.ChaincodeID.Path)
	if err != nil || path == nil {
		return fmt.Errorf("invalid path: %s", err)
	}

	//we have no real good way of checking existence of remote urls except by downloading and testin
	//which we do later anyway. But we *can* - and *should* - test for existence of local paths.
	//Treat empty scheme as a local filesystem path
	if path.Scheme == "" {
		gopath := os.Getenv("GOPATH")
		// Only take the first element of GOPATH
		gopath = filepath.SplitList(gopath)[0]
		pathToCheck := filepath.Join(gopath, "src", spec.ChaincodeID.Path)
		exists, err := pathExists(pathToCheck)
		if err != nil {
			return fmt.Errorf("Error validating chaincode path: %s", err)
		}
		if !exists {
			return fmt.Errorf("Path to chaincode does not exist: %s", spec.ChaincodeID.Path)
		}
	}
	return nil
}

// WritePackage writes the Go chaincode package
func (goPlatform *Platform) GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error) {

	var err error

	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tw := tar.NewWriter(gw)

	//ignore the generated hash. Just use the tw
	//The hash could be used in a future enhancement
	//to check, warn of duplicate installs etc.
	_, err = collectChaincodeFiles(spec, tw)
	if err != nil {
		return nil, err
	}

	err = writeChaincodePackage(spec, tw)

	tw.Close()
	gw.Close()

	if err != nil {
		return nil, err
	}

	payload := inputbuf.Bytes()

	return payload, nil
}

func (goPlatform *Platform) GenerateDockerfile(cds *pb.ChaincodeDeploymentSpec) (string, error) {

	var buf []string
	var err error

	spec := cds.ChaincodeSpec

	urlLocation, err := decodeUrl(spec)
	if err != nil {
		return "", fmt.Errorf("could not decode url: %s", err)
	}

	toks := strings.Split(urlLocation, "/")
	if toks == nil || len(toks) == 0 {
		return "", fmt.Errorf("cannot get path components from %s", urlLocation)
	}

	chaincodeGoName := toks[len(toks)-1]
	if chaincodeGoName == "" {
		return "", fmt.Errorf("could not get chaincode name from path %s", urlLocation)
	}

	buf = append(buf, cutil.GetDockerfileFromConfig("chaincode.golang.Dockerfile"))
	buf = append(buf, "ADD codepackage.tgz $GOPATH")
	//let the executable's name be chaincode ID's name
	buf = append(buf, fmt.Sprintf("RUN go install %s && mv $GOPATH/bin/%s $GOPATH/bin/%s", urlLocation, chaincodeGoName, spec.ChaincodeID.Name))

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

func (goPlatform *Platform) GenerateDockerBuild(cds *pb.ChaincodeDeploymentSpec, tw *tar.Writer) error {
	return cutil.WriteBytesToPackage("codepackage.tgz", cds.CodePackage, tw)
}
