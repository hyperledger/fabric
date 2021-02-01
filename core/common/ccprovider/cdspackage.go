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
	"bytes"
	"fmt"
	"hash"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
)

//----- CDSData ------

// CDSData is data stored in the LSCC on instantiation of a CC
// for CDSPackage.  This needs to be serialized for ChaincodeData
// hence the protobuf format
type CDSData struct {
	// CodeHash hash of CodePackage from ChaincodeDeploymentSpec
	CodeHash []byte `protobuf:"bytes,1,opt,name=codehash,proto3"`

	// MetaDataHash hash of Name and Version from ChaincodeDeploymentSpec
	MetaDataHash []byte `protobuf:"bytes,2,opt,name=metadatahash,proto3"`
}

//----implement functions needed from proto.Message for proto's mar/unmarshal functions

// Reset resets
func (data *CDSData) Reset() { *data = CDSData{} }

// String converts to string
func (data *CDSData) String() string { return proto.CompactTextString(data) }

// ProtoMessage just exists to make proto happy
func (*CDSData) ProtoMessage() {}

// Equals data equals other
func (data *CDSData) Equals(other *CDSData) bool {
	return other != nil && bytes.Equal(data.CodeHash, other.CodeHash) && bytes.Equal(data.MetaDataHash, other.MetaDataHash)
}

// GetHasher interface defines a subset of bccsp which contains GetHash function.
type GetHasher interface {
	GetHash(opts bccsp.HashOpts) (h hash.Hash, err error)
}

//--------- CDSPackage ------------

// CDSPackage encapsulates ChaincodeDeploymentSpec.
type CDSPackage struct {
	buf       []byte
	depSpec   *pb.ChaincodeDeploymentSpec
	data      *CDSData
	datab     []byte
	id        []byte
	GetHasher GetHasher
}

// GetId gets the fingerprint of the chaincode based on package computation
func (ccpack *CDSPackage) GetId() []byte {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetId() should never be called
	if ccpack.id == nil {
		panic("GetId called on uninitialized package")
	}
	return ccpack.id
}

// GetDepSpec gets the ChaincodeDeploymentSpec from the package
func (ccpack *CDSPackage) GetDepSpec() *pb.ChaincodeDeploymentSpec {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetDepSpec() should never be called
	if ccpack.depSpec == nil {
		panic("GetDepSpec called on uninitialized package")
	}
	return ccpack.depSpec
}

// GetDepSpecBytes gets the serialized ChaincodeDeploymentSpec from the package
func (ccpack *CDSPackage) GetDepSpecBytes() []byte {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetDepSpecBytes() should never be called
	if ccpack.buf == nil {
		panic("GetDepSpecBytes called on uninitialized package")
	}
	return ccpack.buf
}

// GetPackageObject gets the ChaincodeDeploymentSpec as proto.Message
func (ccpack *CDSPackage) GetPackageObject() proto.Message {
	return ccpack.depSpec
}

// GetChaincodeData gets the ChaincodeData
func (ccpack *CDSPackage) GetChaincodeData() *ChaincodeData {
	// this has to be after creating a package and initializing it
	// If those steps fail, GetChaincodeData() should never be called
	if ccpack.depSpec == nil || ccpack.datab == nil || ccpack.id == nil {
		panic("GetChaincodeData called on uninitialized package")
	}
	return &ChaincodeData{Name: ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name, Version: ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version, Data: ccpack.datab, Id: ccpack.id}
}

func (ccpack *CDSPackage) getCDSData(cds *pb.ChaincodeDeploymentSpec) ([]byte, []byte, *CDSData, error) {
	// check for nil argument. It is an assertion that getCDSData
	// is never called on a package that did not go through/succeed
	// package initialization.
	if cds == nil {
		panic("nil cds")
	}

	if _, err := proto.Marshal(cds); err != nil {
		return nil, nil, nil, err
	}

	// compute hashes now
	// hash, err := factory.GetDefault().GetHash(&bccsp.SHAOpts{})
	hash, err := ccpack.GetHasher.GetHash(&bccsp.SHAOpts{})
	if err != nil {
		return nil, nil, nil, err
	}

	cdsdata := &CDSData{}

	// code hash
	hash.Write(cds.CodePackage)
	cdsdata.CodeHash = hash.Sum(nil)

	hash.Reset()

	// metadata hash
	hash.Write([]byte(cds.ChaincodeSpec.ChaincodeId.Name))
	hash.Write([]byte(cds.ChaincodeSpec.ChaincodeId.Version))

	cdsdata.MetaDataHash = hash.Sum(nil)

	b, err := proto.Marshal(cdsdata)
	if err != nil {
		return nil, nil, nil, err
	}

	hash.Reset()

	// compute the id
	hash.Write(cdsdata.CodeHash)
	hash.Write(cdsdata.MetaDataHash)

	id := hash.Sum(nil)

	return b, id, cdsdata, nil
}

// ValidateCC returns error if the chaincode is not found or if its not a
// ChaincodeDeploymentSpec
func (ccpack *CDSPackage) ValidateCC(ccdata *ChaincodeData) error {
	if ccpack.depSpec == nil {
		return fmt.Errorf("uninitialized package")
	}

	if ccpack.data == nil {
		return fmt.Errorf("nil data")
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

	otherdata := &CDSData{}
	err := proto.Unmarshal(ccdata.Data, otherdata)
	if err != nil {
		return err
	}

	if !ccpack.data.Equals(otherdata) {
		return fmt.Errorf("data mismatch")
	}

	return nil
}

// InitFromBuffer sets the buffer if valid and returns ChaincodeData
func (ccpack *CDSPackage) InitFromBuffer(buf []byte) (*ChaincodeData, error) {
	depSpec := &pb.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(buf, depSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment spec from bytes")
	}

	databytes, id, data, err := ccpack.getCDSData(depSpec)
	if err != nil {
		return nil, err
	}

	ccpack.buf = buf
	ccpack.depSpec = depSpec
	ccpack.data = data
	ccpack.datab = databytes
	ccpack.id = id

	return ccpack.GetChaincodeData(), nil
}

// InitFromFS returns the chaincode and its package from the file system
func (ccpack *CDSPackage) InitFromPath(ccNameVersion string, path string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	buf, err := GetChaincodePackageFromPath(ccNameVersion, path)
	if err != nil {
		return nil, nil, err
	}

	ccdata, err := ccpack.InitFromBuffer(buf)
	if err != nil {
		return nil, nil, err
	}

	if err := ccpack.ValidateCC(ccdata); err != nil {
		return nil, nil, err
	}

	return ccpack.buf, ccpack.depSpec, nil
}

// InitFromFS returns the chaincode and its package from the file system
func (ccpack *CDSPackage) InitFromFS(ccNameVersion string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	return ccpack.InitFromPath(ccNameVersion, chaincodeInstallPath)
}

// PutChaincodeToFS - serializes chaincode to a package on the file system
func (ccpack *CDSPackage) PutChaincodeToFS() error {
	if ccpack.buf == nil {
		return fmt.Errorf("uninitialized package")
	}

	if ccpack.id == nil {
		return fmt.Errorf("id cannot be nil if buf is not nil")
	}

	if ccpack.depSpec == nil {
		return fmt.Errorf("depspec cannot be nil if buf is not nil")
	}

	if ccpack.data == nil {
		return fmt.Errorf("nil data")
	}

	if ccpack.datab == nil {
		return fmt.Errorf("nil data bytes")
	}

	ccname := ccpack.depSpec.ChaincodeSpec.ChaincodeId.Name
	ccversion := ccpack.depSpec.ChaincodeSpec.ChaincodeId.Version

	// return error if chaincode exists
	path := fmt.Sprintf("%s/%s.%s", chaincodeInstallPath, ccname, ccversion)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("chaincode %s exists", path)
	}

	if err := ioutil.WriteFile(path, ccpack.buf, 0o644); err != nil {
		return err
	}

	return nil
}
