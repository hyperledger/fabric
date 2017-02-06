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
	"fmt"
	"io/ioutil"
	"strings"

	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Platform for the CAR type
type Platform struct {
}

// ValidateSpec validates the chaincode specification for CAR types to satisfy
// the platform interface.  This chaincode type currently doesn't
// require anything specific so we just implicitly approve any spec
func (carPlatform *Platform) ValidateSpec(spec *pb.ChaincodeSpec) error {
	return nil
}

func (carPlatform *Platform) GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error) {

	return ioutil.ReadFile(spec.ChaincodeID.Path)
}

func (carPlatform *Platform) GenerateDockerfile(cds *pb.ChaincodeDeploymentSpec) (string, error) {

	var buf []string

	spec := cds.ChaincodeSpec

	//let the executable's name be chaincode ID's name
	buf = append(buf, cutil.GetDockerfileFromConfig("chaincode.car.Dockerfile"))
	buf = append(buf, "COPY codepackage.car /tmp/codepackage.car")
	// invoking directly for maximum JRE compatiblity
	buf = append(buf, fmt.Sprintf("RUN java -jar /usr/local/bin/chaintool buildcar /tmp/codepackage.car -o $GOPATH/bin/%s && rm /tmp/codepackage.car", spec.ChaincodeID.Name))

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

func (carPlatform *Platform) GenerateDockerBuild(cds *pb.ChaincodeDeploymentSpec, tw *tar.Writer) error {

	return cutil.WriteBytesToPackage("codepackage.car", cds.CodePackage, tw)
}
