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

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
)

var ccproviderLogger = logging.MustGetLogger("ccprovider")

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

	canName := name + ":" + version + "/" + cid

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
