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
	"errors"
	"fmt"
	"os"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	"google.golang.org/protobuf/proto"
)

// -------- SignedCDSPackage ---------

// SignedCDSPackage encapsulates SignedChaincodeDeploymentSpec.
type SignedCDSPackage struct {
	buf       []byte
	depSpec   *pb.ChaincodeDeploymentSpec
	sDepSpec  *pb.SignedChaincodeDeploymentSpec
	env       *common.Envelope
	data      *SignedCDSData
	datab     []byte
	id        []byte
	GetHasher GetHasher
}

// GetId gets the fingerprint of the chaincode based on package computation
func (ccpack *SignedCDSPackage) GetId() []byte {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetId() should never be called
	if ccpack.id == nil {
		panic("GetId called on uninitialized package")
	}
	return ccpack.id
}

// GetDepSpec gets the ChaincodeDeploymentSpec from the package
func (ccpack *SignedCDSPackage) GetDepSpec() *pb.ChaincodeDeploymentSpec {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetDepSpec() should never be called
	if ccpack.depSpec == nil {
		panic("GetDepSpec called on uninitialized package")
	}
	return ccpack.depSpec
}

// GetInstantiationPolicy gets the instantiation policy from the package
func (ccpack *SignedCDSPackage) GetInstantiationPolicy() []byte {
	if ccpack.sDepSpec == nil {
		panic("GetInstantiationPolicy called on uninitialized package")
	}
	return ccpack.sDepSpec.InstantiationPolicy
}

// GetDepSpecBytes gets the serialized ChaincodeDeploymentSpec from the package
func (ccpack *SignedCDSPackage) GetDepSpecBytes() []byte {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetDepSpecBytes() should never be called
	if ccpack.sDepSpec == nil || ccpack.sDepSpec.ChaincodeDeploymentSpec == nil {
		panic("GetDepSpecBytes called on uninitialized package")
	}
	return ccpack.sDepSpec.ChaincodeDeploymentSpec
}

// GetPackageObject gets the ChaincodeDeploymentSpec as proto.Message
func (ccpack *SignedCDSPackage) GetPackageObject() proto.Message {
	return ccpack.env
}

// GetChaincodeData gets the ChaincodeData
func (ccpack *SignedCDSPackage) GetChaincodeData() *ChaincodeData {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetChaincodeData() should never be called
	if ccpack.depSpec == nil || ccpack.datab == nil || ccpack.id == nil {
		panic("GetChaincodeData called on uninitialized package")
	}

	var instPolicy []byte
	if ccpack.sDepSpec != nil {
		instPolicy = ccpack.sDepSpec.InstantiationPolicy
	}

	return &ChaincodeData{
		Name:                ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name,
		Version:             ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version,
		Data:                ccpack.datab,
		Id:                  ccpack.id,
		InstantiationPolicy: instPolicy,
	}
}

func (ccpack *SignedCDSPackage) getCDSData(scds *pb.SignedChaincodeDeploymentSpec) ([]byte, []byte, *SignedCDSData, error) {
	// check for nil argument. It is an assertion that getCDSData
	// is never called on a package that did not go through/succeed
	// package initialization.
	if scds == nil {
		panic("nil cds")
	}

	cds := &pb.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(scds.ChaincodeDeploymentSpec, cds)
	if err != nil {
		return nil, nil, nil, err
	}

	if err = factory.InitFactories(nil); err != nil {
		return nil, nil, nil, fmt.Errorf("Internal error, BCCSP could not be initialized : %s", err)
	}

	// get the hash object
	hash, err := ccpack.GetHasher.GetHash(&bccsp.SHAOpts{})
	if err != nil {
		return nil, nil, nil, err
	}

	scdsdata := &SignedCDSData{}

	// get the code hash
	hash.Write(cds.CodePackage)
	scdsdata.CodeHash = hash.Sum(nil)

	hash.Reset()

	// get the metadata hash
	hash.Write([]byte(cds.ChaincodeSpec.ChaincodeId.Name))
	hash.Write([]byte(cds.ChaincodeSpec.ChaincodeId.Version))

	scdsdata.MetaDataHash = hash.Sum(nil)

	hash.Reset()

	// get the signature hashes
	if scds.InstantiationPolicy == nil {
		return nil, nil, nil, fmt.Errorf("instantiation policy cannot be nil for chaincode (%s:%s)", cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version)
	}

	hash.Write(scds.InstantiationPolicy)
	for _, o := range scds.OwnerEndorsements {
		hash.Write(o.Endorser)
	}
	scdsdata.SignatureHash = hash.Sum(nil)

	// marshall data
	b, err := proto.Marshal(scdsdata)
	if err != nil {
		return nil, nil, nil, err
	}

	hash.Reset()

	// compute the id
	hash.Write(scdsdata.CodeHash)
	hash.Write(scdsdata.MetaDataHash)
	hash.Write(scdsdata.SignatureHash)

	id := hash.Sum(nil)

	return b, id, scdsdata, nil
}

// ValidateCC returns error if the chaincode is not found or if its not a
// ChaincodeDeploymentSpec
func (ccpack *SignedCDSPackage) ValidateCC(ccdata *ChaincodeData) error {
	if ccpack.sDepSpec == nil {
		return errors.New("uninitialized package")
	}

	if ccpack.sDepSpec.ChaincodeDeploymentSpec == nil {
		return errors.New("signed chaincode deployment spec cannot be nil in a package")
	}

	if ccpack.depSpec == nil {
		return errors.New("chaincode deployment spec cannot be nil in a package")
	}

	// This is a hack. LSCC expects a specific LSCC error when names are invalid so it
	// has its own validation code. We can't use that error because of import cycles.
	// Unfortunately, we also need to check if what have makes some sort of sense as
	// protobuf will gladly deserialize garbage and there are paths where we assume that
	// a successful unmarshal means everything works but, if it fails, we try to unmarshal
	// into something different.
	if !isPrintable(ccdata.Name) {
		return fmt.Errorf("invalid chaincode name: %q", ccdata.Name)
	}

	if ccdata.Name != ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name || ccdata.Version != ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version {
		return fmt.Errorf("invalid chaincode data %v (%v)", ccdata, ccpack.depSpec.ChaincodeSpec.ChaincodeId)
	}

	otherdata := &SignedCDSData{}
	err := proto.Unmarshal(ccdata.Data, otherdata)
	if err != nil {
		return err
	}

	if !proto.Equal(ccpack.data, otherdata) {
		return errors.New("data mismatch")
	}

	return nil
}

// InitFromBuffer sets the buffer if valid and returns ChaincodeData
func (ccpack *SignedCDSPackage) InitFromBuffer(buf []byte) (*ChaincodeData, error) {
	env := &common.Envelope{}
	err := proto.Unmarshal(buf, env)
	if err != nil {
		return nil, errors.New("failed to unmarshal envelope from bytes")
	}
	cHdr, sDepSpec, err := ccpackage.ExtractSignedCCDepSpec(env)
	if err != nil {
		return nil, err
	}

	if cHdr.Type != int32(common.HeaderType_CHAINCODE_PACKAGE) {
		return nil, errors.New("invalid type of envelope for chaincode package")
	}

	depSpec := &pb.ChaincodeDeploymentSpec{}
	err = proto.Unmarshal(sDepSpec.ChaincodeDeploymentSpec, depSpec)
	if err != nil {
		return nil, errors.New("error getting deployment spec")
	}

	databytes, id, data, err := ccpack.getCDSData(sDepSpec)
	if err != nil {
		return nil, err
	}

	ccpack.buf = buf
	ccpack.sDepSpec = sDepSpec
	ccpack.depSpec = depSpec
	ccpack.env = env
	ccpack.data = data
	ccpack.datab = databytes
	ccpack.id = id

	return ccpack.GetChaincodeData(), nil
}

// InitFromFS returns the chaincode and its package from the file system
func (ccpack *SignedCDSPackage) InitFromFS(ccNameVersion string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	return ccpack.InitFromPath(ccNameVersion, chaincodeInstallPath)
}

// InitFromPath returns the chaincode and its package from the file system
func (ccpack *SignedCDSPackage) InitFromPath(ccNameVersion string, path string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	buf, err := GetChaincodePackageFromPath(ccNameVersion, path)
	if err != nil {
		return nil, nil, err
	}

	if _, err = ccpack.InitFromBuffer(buf); err != nil {
		return nil, nil, err
	}

	return ccpack.buf, ccpack.depSpec, nil
}

// PutChaincodeToFS - serializes chaincode to a package on the file system
func (ccpack *SignedCDSPackage) PutChaincodeToFS() error {
	if ccpack.buf == nil {
		return errors.New("uninitialized package")
	}

	if ccpack.id == nil {
		return errors.New("id cannot be nil if buf is not nil")
	}

	if ccpack.sDepSpec == nil || ccpack.depSpec == nil {
		return errors.New("depspec cannot be nil if buf is not nil")
	}

	if ccpack.env == nil {
		return errors.New("env cannot be nil if buf and depspec are not nil")
	}

	if ccpack.data == nil {
		return errors.New("nil data")
	}

	if ccpack.datab == nil {
		return errors.New("nil data bytes")
	}

	ccname := ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name
	ccversion := ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version

	// return error if chaincode exists
	path := fmt.Sprintf("%s/%s.%s", chaincodeInstallPath, ccname, ccversion)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("chaincode %s exists", path)
	}

	if err := os.WriteFile(path, ccpack.buf, 0o644); err != nil {
		return err
	}

	return nil
}
