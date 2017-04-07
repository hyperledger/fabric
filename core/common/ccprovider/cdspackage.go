/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package ccprovider

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"

	pb "github.com/hyperledger/fabric/protos/peer"
)

//CDSPackage encapsulates ChaincodeDeploymentSpec.
type CDSPackage struct {
	buf     []byte
	depSpec *pb.ChaincodeDeploymentSpec
}

// GetDepSpec gets the ChaincodeDeploymentSpec from the package
func (ccpack *CDSPackage) GetDepSpec() *pb.ChaincodeDeploymentSpec {
	return ccpack.depSpec
}

// ValidateCC returns error if the chaincode is not found or if its not a
// ChaincodeDeploymentSpec
func (ccpack *CDSPackage) ValidateCC(ccdata *ChaincodeData) (*pb.ChaincodeDeploymentSpec, error) {
	if ccpack.depSpec == nil {
		return nil, fmt.Errorf("uninitialized package")
	}
	if ccdata.Name != ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name || ccdata.Version != ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version {
		return nil, fmt.Errorf("invalid chaincode data %v (%v)", ccdata, ccpack.depSpec.ChaincodeSpec.ChaincodeId)
	}
	//for now just return chaincode. When we introduce Hash we will do more checks
	return ccpack.depSpec, nil
}

//InitFromBuffer sets the buffer if valid and returns ChaincodeData
func (ccpack *CDSPackage) InitFromBuffer(buf []byte) (*ChaincodeData, error) {
	//incase ccpack is reused
	ccpack.buf = nil
	ccpack.depSpec = nil

	depSpec := &pb.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(buf, depSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal fs deployment spec from bytes")
	}
	ccpack.buf = buf
	ccpack.depSpec = depSpec
	return &ChaincodeData{Name: depSpec.ChaincodeSpec.ChaincodeId.Name, Version: depSpec.ChaincodeSpec.ChaincodeId.Version}, nil
}

//InitFromFS returns the chaincode and its package from the file system
func (ccpack *CDSPackage) InitFromFS(ccname string, ccversion string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	//incase ccpack is reused
	ccpack.buf = nil
	ccpack.depSpec = nil

	buf, err := GetChaincodePackage(ccname, ccversion)
	if err != nil {
		return nil, nil, err
	}

	depSpec := &pb.ChaincodeDeploymentSpec{}
	err = proto.Unmarshal(buf, depSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal fs deployment spec for %s, %s", ccname, ccversion)
	}

	ccpack.buf = buf
	ccpack.depSpec = depSpec

	return buf, depSpec, nil
}

//PutChaincodeToFS - serializes chaincode to a package on the file system
func (ccpack *CDSPackage) PutChaincodeToFS() error {
	if ccpack.buf == nil {
		return fmt.Errorf("uninitialized package")
	}

	if ccpack.depSpec == nil {
		return fmt.Errorf("depspec cannot be nil if buf is not nil")
	}

	ccname := ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name
	ccversion := ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version

	//return error if chaincode exists
	path := fmt.Sprintf("%s/%s.%s", chaincodeInstallPath, ccname, ccversion)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("chaincode %s exists", path)
	}

	if err := ioutil.WriteFile(path, ccpack.buf, 0644); err != nil {
		return err
	}

	return nil
}
