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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
)

var ccproviderLogger = logging.MustGetLogger("ccprovider")

var chaincodeInstallPath string

//CCPackage encapsulates a chaincode package which can be
//    raw ChaincodeDeploymentSpec
//    SignedChaincodeDeploymentSpec
// Attempt to keep the interface at a level with minimal
// interface for possible generalization.
type CCPackage interface {
	//InitFromBuffer initialize the package from bytes
	InitFromBuffer(buf []byte) (*ChaincodeData, error)

	// InitFromFS gets the chaincode from the filesystem (includes the raw bytes too)
	InitFromFS(ccname string, ccversion string) ([]byte, *pb.ChaincodeDeploymentSpec, error)

	// PutChaincodeToFS writes the chaincode to the filesystem
	PutChaincodeToFS() error

	// GetDepSpec gets the ChaincodeDeploymentSpec from the package
	GetDepSpec() *pb.ChaincodeDeploymentSpec

	// GetDepSpecBytes gets the serialized ChaincodeDeploymentSpec from the package
	GetDepSpecBytes() []byte

	// ValidateCC validates and returns the chaincode deployment spec corresponding to
	// ChaincodeData. The validation is based on the metadata from ChaincodeData
	// One use of this method is to validate the chaincode before launching
	ValidateCC(ccdata *ChaincodeData) error

	// GetPackageObject gets the object as a proto.Message
	GetPackageObject() proto.Message

	// GetChaincodeData gets the ChaincodeData
	GetChaincodeData() *ChaincodeData

	// GetId gets the fingerprint of the chaincode based on package computation
	GetId() []byte
}

//SetChaincodesPath sets the chaincode path for this peer
func SetChaincodesPath(path string) {
	if s, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(path, 0755); err != nil {
				panic(fmt.Sprintf("Could not create chaincodes install path: %s", err))
			}
		} else {
			panic(fmt.Sprintf("Could not stat chaincodes install path: %s", err))
		}
	} else if !s.IsDir() {
		panic(fmt.Errorf("chaincode path exists but not a dir: %s", path))
	}

	chaincodeInstallPath = path
}

//GetChaincodePackage returns the chaincode package from the file system
func GetChaincodePackage(ccname string, ccversion string) ([]byte, error) {
	path := fmt.Sprintf("%s/%s.%s", chaincodeInstallPath, ccname, ccversion)
	var ccbytes []byte
	var err error
	if ccbytes, err = ioutil.ReadFile(path); err != nil {
		return nil, err
	}
	return ccbytes, nil
}

// GetChaincodeFromFS this is a wrapper for hiding package implementation.
func GetChaincodeFromFS(ccname string, ccversion string) (CCPackage, error) {
	//try raw CDS
	cccdspack := &CDSPackage{}
	_, _, err := cccdspack.InitFromFS(ccname, ccversion)
	if err != nil {
		//try signed CDS
		ccscdspack := &SignedCDSPackage{}
		_, _, err = ccscdspack.InitFromFS(ccname, ccversion)
		if err != nil {
			return nil, err
		}
		return ccscdspack, nil
	}
	return cccdspack, nil
}

// PutChaincodeIntoFS is a wrapper for putting raw ChaincodeDeploymentSpec
//using CDSPackage. This is only used in UTs
func PutChaincodeIntoFS(depSpec *pb.ChaincodeDeploymentSpec) error {
	buf, err := proto.Marshal(depSpec)
	if err != nil {
		return err
	}
	cccdspack := &CDSPackage{}
	if _, err := cccdspack.InitFromBuffer(buf); err != nil {
		return err
	}
	return cccdspack.PutChaincodeToFS()
}

// GetCCPackage tries each known package implementation one by one
// till the right package is found
func GetCCPackage(buf []byte) (CCPackage, error) {
	//try raw CDS
	cccdspack := &CDSPackage{}
	if _, err := cccdspack.InitFromBuffer(buf); err != nil {
		//try signed CDS
		ccscdspack := &SignedCDSPackage{}
		if _, err := ccscdspack.InitFromBuffer(buf); err != nil {
			return nil, err
		}
		return ccscdspack, nil
	}
	return cccdspack, nil
}

// GetInstalledChaincodes returns a map whose key is the chaincode id and
// value is the ChaincodeDeploymentSpec struct for that chaincodes that have
// been installed (but not necessarily instantiated) on the peer by searching
// the chaincode install path
func GetInstalledChaincodes() (*pb.ChaincodeQueryResponse, error) {
	files, err := ioutil.ReadDir(chaincodeInstallPath)
	if err != nil {
		return nil, err
	}

	// array to store info for all chaincode entries from LCCC
	var ccInfoArray []*pb.ChaincodeInfo

	for _, file := range files {
		// split at first period as chaincode versions can contain periods while
		// chaincode names cannot
		fileNameArray := strings.SplitN(file.Name(), ".", 2)

		// check that length is 2 as expected, otherwise skip to next cc file
		if len(fileNameArray) == 2 {
			ccname := fileNameArray[0]
			ccversion := fileNameArray[1]
			ccpack, err := GetChaincodeFromFS(ccname, ccversion)
			if err != nil {
				// either chaincode on filesystem has been tampered with or
				// a non-chaincode file has been found in the chaincodes directory
				ccproviderLogger.Errorf("Unreadable chaincode file found on filesystem: %s", file.Name())
				continue
			}

			cdsfs := ccpack.GetDepSpec()

			name := cdsfs.GetChaincodeSpec().GetChaincodeId().Name
			version := cdsfs.GetChaincodeSpec().GetChaincodeId().Version
			if name != ccname || version != ccversion {
				// chaincode name/version in the chaincode file name has been modified
				// by an external entity
				ccproviderLogger.Errorf("Chaincode file's name/version has been modified on the filesystem: %s", file.Name())
				continue
			}

			path := cdsfs.GetChaincodeSpec().ChaincodeId.Path
			// since this is just an installed chaincode these should be blank
			input, escc, vscc := "", "", ""

			ccInfo := &pb.ChaincodeInfo{Name: name, Version: version, Path: path, Input: input, Escc: escc, Vscc: vscc}

			// add this specific chaincode's metadata to the array of all chaincodes
			ccInfoArray = append(ccInfoArray, ccInfo)
		}
	}
	// add array with info about all instantiated chaincodes to the query
	// response proto
	cqr := &pb.ChaincodeQueryResponse{Chaincodes: ccInfoArray}

	return cqr, nil
}

//CCContext pass this around instead of string of args
type CCContext struct {
	//ChainID chain id
	ChainID string

	//Name chaincode name
	Name string

	//Version used to construct the chaincode image and register
	Version string

	//TxID is the transaction id for the proposal (if any)
	TxID string

	//Syscc is this a system chaincode
	Syscc bool

	//SignedProposal for this invoke (if any)
	//this is kept here for access control and in case we need to pass something
	//from this to the chaincode
	SignedProposal *pb.SignedProposal

	//Proposal for this invoke (if any)
	//this is kept here just in case we need to pass something
	//from this to the chaincode
	Proposal *pb.Proposal

	//this is not set but computed (note that this is not exported. use GetCanonicalName)
	canonicalName string
}

//NewCCContext just construct a new struct with whatever args
func NewCCContext(cid, name, version, txid string, syscc bool, signedProp *pb.SignedProposal, prop *pb.Proposal) *CCContext {
	//version CANNOT be empty. The chaincode namespace has to use version and chain name.
	//All system chaincodes share the same version given by utils.GetSysCCVersion. Note
	//that neither Chain Name or Version are stored in a chaincodes state on the ledger
	if version == "" {
		panic(fmt.Sprintf("---empty version---(chain=%s,chaincode=%s,version=%s,txid=%s,syscc=%t,proposal=%p", cid, name, version, txid, syscc, prop))
	}

	canName := name + ":" + version

	cccid := &CCContext{cid, name, version, txid, syscc, signedProp, prop, canName}

	ccproviderLogger.Debugf("NewCCCC (chain=%s,chaincode=%s,version=%s,txid=%s,syscc=%t,proposal=%p,canname=%s", cid, name, version, txid, syscc, prop, cccid.canonicalName)

	return cccid
}

//GetCanonicalName returns the canonical name associated with the proposal context
func (cccid *CCContext) GetCanonicalName() string {
	if cccid.canonicalName == "" {
		panic(fmt.Sprintf("cccid not constructed using NewCCContext(chain=%s,chaincode=%s,version=%s,txid=%s,syscc=%t)", cccid.ChainID, cccid.Name, cccid.Version, cccid.TxID, cccid.Syscc))
	}

	return cccid.canonicalName
}

//-------- ChaincodeData is stored on the LCCC -------

//ChaincodeData defines the datastructure for chaincodes to be serialized by proto
//Type provides an additional check by directing to use a specific package after instantiation
//Data is Type specifc (see CDSPackage and SignedCDSPackage)
type ChaincodeData struct {
	//Name of the chaincode
	Name string `protobuf:"bytes,1,opt,name=name"`

	//Version of the chaincode
	Version string `protobuf:"bytes,2,opt,name=version"`

	//Escc for the chaincode instance
	Escc string `protobuf:"bytes,3,opt,name=escc"`

	//Vscc for the chaincode instance
	Vscc string `protobuf:"bytes,4,opt,name=vscc"`

	//Policy endorsement policy for the chaincode instance
	Policy []byte `protobuf:"bytes,5,opt,name=policy,proto3"`

	//Data data specific to the package
	Data []byte `protobuf:"bytes,6,opt,name=data,proto3"`

	//Id of the chaincode that's the unique fingerprint for the CC
	//This is not currently used anywhere but serves as a good
	//eyecatcher
	Id []byte `protobuf:"bytes,7,opt,name=id,proto3"`
}

//implement functions needed from proto.Message for proto's mar/unmarshal functions

//Reset resets
func (cd *ChaincodeData) Reset() { *cd = ChaincodeData{} }

//String convers to string
func (cd *ChaincodeData) String() string { return proto.CompactTextString(cd) }

//ProtoMessage just exists to make proto happy
func (*ChaincodeData) ProtoMessage() {}

// ChaincodeProvider provides an abstraction layer that is
// used for different packages to interact with code in the
// chaincode package without importing it; more methods
// should be added below if necessary
type ChaincodeProvider interface {
	// GetContext returns a ledger context
	GetContext(ledger ledger.PeerLedger) (context.Context, error)
	// GetCCContext returns an opaque chaincode context
	GetCCContext(cid, name, version, txid string, syscc bool, signedProp *pb.SignedProposal, prop *pb.Proposal) interface{}
	// GetCCValidationInfoFromLCCC returns the VSCC and the policy listed by LCCC for the supplied chaincode
	GetCCValidationInfoFromLCCC(ctxt context.Context, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chainID string, chaincodeID string) (string, []byte, error)
	// ExecuteChaincode executes the chaincode given context and args
	ExecuteChaincode(ctxt context.Context, cccid interface{}, args [][]byte) (*pb.Response, *pb.ChaincodeEvent, error)
	// Execute executes the chaincode given context and spec (invocation or deploy)
	Execute(ctxt context.Context, cccid interface{}, spec interface{}) (*pb.Response, *pb.ChaincodeEvent, error)
	// ExecuteWithErrorFilder executes the chaincode given context and spec and returns payload
	ExecuteWithErrorFilter(ctxt context.Context, cccid interface{}, spec interface{}) ([]byte, *pb.ChaincodeEvent, error)
	// Stop stops the chaincode given context and deployment spec
	Stop(ctxt context.Context, cccid interface{}, spec *pb.ChaincodeDeploymentSpec) error
	// ReleaseContext releases the context returned previously by GetContext
	ReleaseContext()
}

var ccFactory ChaincodeProviderFactory

// ChaincodeProviderFactory defines a factory interface so
// that the actual implementation can be injected
type ChaincodeProviderFactory interface {
	NewChaincodeProvider() ChaincodeProvider
}

// RegisterChaincodeProviderFactory is to be called once to set
// the factory that will be used to obtain instances of ChaincodeProvider
func RegisterChaincodeProviderFactory(ccfact ChaincodeProviderFactory) {
	ccFactory = ccfact
}

// GetChaincodeProvider returns instances of ChaincodeProvider;
// the actual implementation is controlled by the factory that
// is registered via RegisterChaincodeProviderFactory
func GetChaincodeProvider() ChaincodeProvider {
	if ccFactory == nil {
		panic("The factory must be set first via RegisterChaincodeProviderFactory")
	}
	return ccFactory.NewChaincodeProvider()
}
