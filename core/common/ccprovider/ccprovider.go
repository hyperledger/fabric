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

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
)

var ccproviderLogger = logging.MustGetLogger("ccprovider")

var chaincodeInstallPath string

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

//GetChaincodeFromFS returns the chaincode and its package from the file system
func GetChaincodeFromFS(ccname string, ccversion string) ([]byte, *pb.ChaincodeDeploymentSpec, error) {
	//NOTE- this is the only place from where we get code from file system
	//this API needs to be modified to take other params for security.
	//this implementation needs to be enhanced to do those security checks
	ccbytes, err := GetChaincodePackage(ccname, ccversion)
	if err != nil {
		return nil, nil, err
	}

	cdsfs := &pb.ChaincodeDeploymentSpec{}
	err = proto.Unmarshal(ccbytes, cdsfs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal fs deployment spec for %s, %s", ccname, ccversion)
	}

	return ccbytes, cdsfs, nil
}

//PutChaincodeIntoFS - serializes chaincode to a package on the file system
func PutChaincodeIntoFS(depSpec *pb.ChaincodeDeploymentSpec) error {
	//NOTE- this is  only place from where we put code into file system
	//this API needs to be modified to take other params for security.
	//this implementation needs to be enhanced to do those security checks
	ccname := depSpec.ChaincodeSpec.ChaincodeId.Name
	ccversion := depSpec.ChaincodeSpec.ChaincodeId.Version

	b, err := proto.Marshal(depSpec)
	if err != nil {
		return fmt.Errorf("failed to marshal fs deployment spec for %s, %s", ccname, ccversion)
	}

	path := fmt.Sprintf("%s/%s.%s", chaincodeInstallPath, ccname, ccversion)
	if err = ioutil.WriteFile(path, b, 0644); err != nil {
		return err
	}

	return nil
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

	//Proposal for this invoke (if any)
	//this is kept here just in case we need to pass something
	//from this to the chaincode
	Proposal *pb.Proposal

	//this is not set but computed (note that this is not exported. use GetCanonicalName)
	canonicalName string
}

//NewCCContext just construct a new struct with whatever args
func NewCCContext(cid, name, version, txid string, syscc bool, prop *pb.Proposal) *CCContext {
	//version CANNOT be empty. The chaincode namespace has to use version and chain name.
	//All system chaincodes share the same version given by utils.GetSysCCVersion. Note
	//that neither Chain Name or Version are stored in a chaincodes state on the ledger
	if version == "" {
		panic(fmt.Sprintf("---empty version---(chain=%s,chaincode=%s,version=%s,txid=%s,syscc=%t,proposal=%p", cid, name, version, txid, syscc, prop))
	}

	canName := name + ":" + version

	cccid := &CCContext{cid, name, version, txid, syscc, prop, canName}

	ccproviderLogger.Infof("NewCCCC (chain=%s,chaincode=%s,version=%s,txid=%s,syscc=%t,proposal=%p,canname=%s", cid, name, version, txid, syscc, prop, cccid.canonicalName)

	return cccid
}

//GetCanonicalName returns the canonical name associated with the proposal context
func (cccid *CCContext) GetCanonicalName() string {
	if cccid.canonicalName == "" {
		panic(fmt.Sprintf("cccid not constructed using NewCCContext(chain=%s,chaincode=%s,version=%s,txid=%s,syscc=%t)", cccid.ChainID, cccid.Name, cccid.Version, cccid.TxID, cccid.Syscc))
	}

	return cccid.canonicalName
}

//ChaincodeData defines the datastructure for chaincodes to be serialized by proto
type ChaincodeData struct {
	Name    string `protobuf:"bytes,1,opt,name=name"`
	Version string `protobuf:"bytes,2,opt,name=version"`
	DepSpec []byte `protobuf:"bytes,3,opt,name=depSpec,proto3"`
	Escc    string `protobuf:"bytes,4,opt,name=escc"`
	Vscc    string `protobuf:"bytes,5,opt,name=vscc"`
	Policy  []byte `protobuf:"bytes,6,opt,name=policy"`
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
	GetCCContext(cid, name, version, txid string, syscc bool, prop *pb.Proposal) interface{}
	// GetCCValidationInfoFromLCCC returns the VSCC and the policy listed by LCCC for the supplied chaincode
	GetCCValidationInfoFromLCCC(ctxt context.Context, txid string, prop *pb.Proposal, chainID string, chaincodeID string) (string, []byte, error)
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
