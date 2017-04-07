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

	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

//SignedCDSPackage encapsulates SignedChaincodeDeploymentSpec.
type SignedCDSPackage struct {
	buf      []byte
	depSpec  *pb.ChaincodeDeploymentSpec
	sDepSpec *pb.SignedChaincodeDeploymentSpec
	env      *common.Envelope
}

// GetDepSpec gets the ChaincodeDeploymentSpec from the package
func (ccpack *SignedCDSPackage) GetDepSpec() *pb.ChaincodeDeploymentSpec {
	return ccpack.depSpec
}

// GetPackageObject gets the ChaincodeDeploymentSpec as proto.Message
func (ccpack *SignedCDSPackage) GetPackageObject() proto.Message {
	return ccpack.env
}

// ValidateCC returns error if the chaincode is not found or if its not a
// ChaincodeDeploymentSpec
func (ccpack *SignedCDSPackage) ValidateCC(ccdata *ChaincodeData) (*pb.ChaincodeDeploymentSpec, error) {
	if ccpack.sDepSpec == nil {
		return nil, fmt.Errorf("uninitialized package")
	}

	if ccpack.sDepSpec.ChaincodeDeploymentSpec == nil {
		return nil, fmt.Errorf("signed chaincode deployment spec cannot be nil in a package")
	}

	if ccpack.depSpec == nil {
		return nil, fmt.Errorf("chaincode deployment spec cannot be nil in a package")
	}

	if ccdata.Name != ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name || ccdata.Version != ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version {
		return nil, fmt.Errorf("invalid chaincode data %v (%v)", ccdata, ccpack.depSpec.ChaincodeSpec.ChaincodeId)
	}

	//for now just return chaincode. When we introduce Hash we will do more checks
	return ccpack.depSpec, nil
}

//InitFromBuffer sets the buffer if valid and returns ChaincodeData
func (ccpack *SignedCDSPackage) InitFromBuffer(buf []byte) (*ChaincodeData, error) {
	//incase ccpack is reused
	ccpack.buf = nil
	ccpack.sDepSpec = nil
	ccpack.depSpec = nil
	ccpack.env = nil

	env := &common.Envelope{}
	err := proto.Unmarshal(buf, env)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope from bytes")
	}
	cHdr, sDepSpec, err := ccpackage.ExtractSignedCCDepSpec(env)
	if err != nil {
		return nil, err
	}
	if cHdr.Type != int32(common.HeaderType_CHAINCODE_PACKAGE) {
		return nil, fmt.Errorf("invalid type of envelope for chaincode package")
	}

	depSpec := &pb.ChaincodeDeploymentSpec{}
	err = proto.Unmarshal(sDepSpec.ChaincodeDeploymentSpec, depSpec)
	if err != nil {
		return nil, fmt.Errorf("error getting deployment spec")
	}

	ccpack.buf = buf
	ccpack.sDepSpec = sDepSpec
	ccpack.depSpec = depSpec
	ccpack.env = env

	return &ChaincodeData{Name: depSpec.ChaincodeSpec.ChaincodeId.Name, Version: depSpec.ChaincodeSpec.ChaincodeId.Version}, nil
}

//InitFromFS returns the chaincode and its package from the file system
func (ccpack *SignedCDSPackage) InitFromFS(ccname string, ccversion string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	//incase ccpack is reused
	ccpack.buf = nil
	ccpack.sDepSpec = nil
	ccpack.depSpec = nil
	ccpack.env = nil

	buf, err := GetChaincodePackage(ccname, ccversion)
	if err != nil {
		return nil, nil, err
	}

	if _, err = ccpack.InitFromBuffer(buf); err != nil {
		return nil, nil, err
	}

	return ccpack.buf, ccpack.depSpec, nil
}

//PutChaincodeToFS - serializes chaincode to a package on the file system
func (ccpack *SignedCDSPackage) PutChaincodeToFS() error {
	if ccpack.buf == nil {
		return fmt.Errorf("uninitialized package")
	}

	if ccpack.sDepSpec == nil || ccpack.depSpec == nil {
		return fmt.Errorf("depspec cannot be nil if buf is not nil")
	}

	if ccpack.env == nil {
		return fmt.Errorf("env cannot be nil if buf and depspec are not nil")
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
