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

package car

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

func httpDownload(path string) ([]byte, error) {

	// The file is remote, so we need to download it first
	var err error

	resp, err := http.Get(path)
	if err != nil {
		return nil, fmt.Errorf("Error with HTTP GET: %s", err)
	}
	defer resp.Body.Close()

	buf := bytes.NewBuffer(nil)

	// FIXME: Validate maximum size constraints are not violated
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error downloading bytes: %s", err)
	}

	return buf.Bytes(), nil
}

var httpSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

func (carPlatform *Platform) GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error) {

	path := spec.ChaincodeID.Path
	url, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("Error decoding %s: %s", path, err)
	}

	if _, ok := httpSchemes[url.Scheme]; ok {
		return httpDownload(path)
	} else {
		// All we know is its _not_ an HTTP scheme.  Assume the path is a local file
		// and see if it works.
		return ioutil.ReadFile(url.Path)
	}
}

func (carPlatform *Platform) GenerateDockerBuild(cds *pb.ChaincodeDeploymentSpec, tw *tar.Writer) (string, error) {

	var buf []string
	var err error

	spec := cds.ChaincodeSpec

	//let the executable's name be chaincode ID's name
	buf = append(buf, cutil.GetDockerfileFromConfig("chaincode.car.Dockerfile"))
	buf = append(buf, "COPY codepackage.car /tmp/codepackage.car")
	// invoking directly for maximum JRE compatiblity
	buf = append(buf, fmt.Sprintf("RUN java -jar /usr/local/bin/chaintool buildcar /tmp/codepackage.car -o $GOPATH/bin/%s && rm /tmp/codepackage.car", spec.ChaincodeID.Name))

	dockerFileContents := strings.Join(buf, "\n")

	err = cutil.WriteBytesToPackage("codepackage.car", cds.CodePackage, tw)
	if err != nil {
		return "", err
	}

	return dockerFileContents, nil
}
