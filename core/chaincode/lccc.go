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

package chaincode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
)

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

//The life cycle system chaincode manages chaincodes deployed
//on this peer. It manages chaincodes via Invoke proposals.
//     "Args":["deploy",<ChaincodeDeploymentSpec>]
//     "Args":["upgrade",<ChaincodeDeploymentSpec>]
//     "Args":["stop",<ChaincodeInvocationSpec>]
//     "Args":["start",<ChaincodeInvocationSpec>]

var logger = logging.MustGetLogger("lccc")

const (
	//CHAINCODETABLE prefix for chaincode tables
	CHAINCODETABLE = "chaincodes"

	//chaincode lifecyle commands

	//DEPLOY deploy command
	DEPLOY = "deploy"

	//UPGRADE upgrade chaincode
	UPGRADE = "upgrade"

	//GETCCINFO get chaincode
	GETCCINFO = "getid"

	//GETDEPSPEC get ChaincodeDeploymentSpec
	GETDEPSPEC = "getdepspec"

	//GETCCDATA get ChaincodeData
	GETCCDATA = "getccdata"

	//characters used in chaincodenamespace
	specialChars = "/:[]${}"

	// chaincode version when deploy
	startVersion = "0"
)

//---------- the LCCC -----------------

// LifeCycleSysCC implements chaincode lifecycle and policies aroud it
type LifeCycleSysCC struct {
}

//----------------errors---------------

//AlreadyRegisteredErr Already registered error
type AlreadyRegisteredErr string

func (f AlreadyRegisteredErr) Error() string {
	return fmt.Sprintf("%s already registered", string(f))
}

//InvalidFunctionErr invalid function error
type InvalidFunctionErr string

func (f InvalidFunctionErr) Error() string {
	return fmt.Sprintf("invalid function to lccc %s", string(f))
}

//InvalidArgsLenErr invalid arguments length error
type InvalidArgsLenErr int

func (i InvalidArgsLenErr) Error() string {
	return fmt.Sprintf("invalid number of argument to lccc %d", int(i))
}

//InvalidArgsErr invalid arguments error
type InvalidArgsErr int

func (i InvalidArgsErr) Error() string {
	return fmt.Sprintf("invalid argument (%d) to lccc", int(i))
}

//TXExistsErr transaction exists error
type TXExistsErr string

func (t TXExistsErr) Error() string {
	return fmt.Sprintf("transaction exists %s", string(t))
}

//TXNotFoundErr transaction not found error
type TXNotFoundErr string

func (t TXNotFoundErr) Error() string {
	return fmt.Sprintf("transaction not found %s", string(t))
}

//InvalidDeploymentSpecErr invalide chaincode deployment spec error
type InvalidDeploymentSpecErr string

func (f InvalidDeploymentSpecErr) Error() string {
	return fmt.Sprintf("Invalid deployment spec : %s", string(f))
}

//ExistsErr chaincode exists error
type ExistsErr string

func (t ExistsErr) Error() string {
	return fmt.Sprintf("Chaincode exists %s", string(t))
}

//NotFoundErr chaincode not registered with LCCC error
type NotFoundErr string

func (t NotFoundErr) Error() string {
	return fmt.Sprintf("chaincode not found %s", string(t))
}

//InvalidChainNameErr invalid chain name error
type InvalidChainNameErr string

func (f InvalidChainNameErr) Error() string {
	return fmt.Sprintf("invalid chain name %s", string(f))
}

//InvalidChaincodeNameErr invalid chaincode name error
type InvalidChaincodeNameErr string

func (f InvalidChaincodeNameErr) Error() string {
	return fmt.Sprintf("invalid chain code name %s", string(f))
}

//MarshallErr error marshaling/unmarshalling
type MarshallErr string

func (m MarshallErr) Error() string {
	return fmt.Sprintf("error while marshalling %s", string(m))
}

//-------------- helper functions ------------------
//create the chaincode on the given chain
func (lccc *LifeCycleSysCC) createChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string, cccode []byte) (*ChaincodeData, error) {
	return lccc.putChaincodeData(stub, chainname, ccname, startVersion, cccode)
}

//upgrade the chaincode on the given chain
func (lccc *LifeCycleSysCC) upgradeChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string, version string, cccode []byte) (*ChaincodeData, error) {
	return lccc.putChaincodeData(stub, chainname, ccname, version, cccode)
}

//create the chaincode on the given chain
func (lccc *LifeCycleSysCC) putChaincodeData(stub shim.ChaincodeStubInterface, chainname string, ccname string, version string, cccode []byte) (*ChaincodeData, error) {
	cd := &ChaincodeData{Name: ccname, Version: version, DepSpec: cccode}
	cdbytes, err := proto.Marshal(cd)
	if err != nil {
		return nil, err
	}

	if cdbytes == nil {
		return nil, MarshallErr(ccname)
	}

	err = stub.PutState(ccname, cdbytes)

	return cd, err
}

//checks for existence of chaincode on the given chain
func (lccc *LifeCycleSysCC) getChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string) (*ChaincodeData, []byte, error) {
	cdbytes, err := stub.GetState(ccname)
	if err != nil {
		return nil, nil, err
	}

	if cdbytes != nil {
		cd := &ChaincodeData{}
		err = proto.Unmarshal(cdbytes, cd)
		if err != nil {
			return nil, nil, MarshallErr(ccname)
		}

		return cd, cdbytes, nil
	}

	return nil, nil, NotFoundErr(ccname)
}

//getChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func (lccc *LifeCycleSysCC) getChaincodeDeploymentSpec(code []byte) (*pb.ChaincodeDeploymentSpec, error) {
	cds := &pb.ChaincodeDeploymentSpec{}

	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, InvalidDeploymentSpecErr(err.Error())
	}

	return cds, nil
}

//do access control
func (lccc *LifeCycleSysCC) acl(stub shim.ChaincodeStubInterface, chainname string, cds *pb.ChaincodeDeploymentSpec) error {
	return nil
}

//check validity of chain name
func (lccc *LifeCycleSysCC) isValidChainName(chainname string) bool {
	//TODO we probably need more checks
	if chainname == "" {
		return false
	}
	return true
}

//check validity of chaincode name
func (lccc *LifeCycleSysCC) isValidChaincodeName(chaincodename string) bool {
	//TODO we probably need more checks
	if chaincodename == "" {
		return false
	}

	//do not allow special characters in chaincode name
	if strings.ContainsAny(chaincodename, specialChars) {
		return false
	}

	return true
}

//deploy the chaincode on to the chain
func (lccc *LifeCycleSysCC) deploy(stub shim.ChaincodeStubInterface, chainname string, version string, cds *pb.ChaincodeDeploymentSpec) error {
	//if unit testing, just return..we cannot do the actual deploy
	if _, ismock := stub.(*shim.MockStub); ismock {
		//we got this far just stop short of actual deploy for test purposes
		return nil
	}

	ctxt := context.Background()

	//TODO - we are in the LCCC chaincode simulating an "invoke" to deploy another
	//chaincode. Deploying the chaincode and calling its "init" involves ledger access
	//for the called chaincode - ie, it needs to undergo state simulatio as well.
	//How do we handle the simulation for the called called chaincode ?
	//    1) don't allow state initialization on deploy
	//    2) combine both LCCC and the called chaincodes into one RW set
	//    3) just drop the second
	lgr := peer.GetLedger(chainname)

	var dummytxsim ledger.TxSimulator
	var err error

	if dummytxsim, err = lgr.NewTxSimulator(); err != nil {
		return fmt.Errorf("Could not get simulator for %s", chainname)
	}

	ctxt = context.WithValue(ctxt, TXSimulatorKey, dummytxsim)

	//TODO-if/when we use this deploy() func make sure to use the
	//TXID of the calling proposal
	txid := util.GenerateUUID()

	//deploy does not need a version
	cccid := NewCCContext(chainname, cds.ChaincodeSpec.ChaincodeID.Name, startVersion, txid, false, nil)

	_, err = theChaincodeSupport.Deploy(ctxt, cccid, cds)
	if err != nil {
		return fmt.Errorf("Failed to deploy chaincode spec(%s)", err)
	}

	//launch and wait for ready
	_, _, err = theChaincodeSupport.Launch(ctxt, cccid, cds)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	//stop now that we are done
	theChaincodeSupport.Stop(ctxt, cccid, cds)

	return nil
}

//this implements "deploy" Invoke transaction
func (lccc *LifeCycleSysCC) executeDeploy(stub shim.ChaincodeStubInterface, chainname string, code []byte) error {
	cds, err := lccc.getChaincodeDeploymentSpec(code)

	if err != nil {
		return err
	}

	if !lccc.isValidChaincodeName(cds.ChaincodeSpec.ChaincodeID.Name) {
		return InvalidChaincodeNameErr(cds.ChaincodeSpec.ChaincodeID.Name)
	}

	if err = lccc.acl(stub, chainname, cds); err != nil {
		return err
	}

	cd, _, err := lccc.getChaincode(stub, chainname, cds.ChaincodeSpec.ChaincodeID.Name)
	if cd != nil {
		return ExistsErr(cds.ChaincodeSpec.ChaincodeID.Name)
	}

	/**TODO - this is done in the endorser service for now so we can
		 * collect all state changes under one TXSim. Revisit this ...
	         * maybe this *is* the right solution
		 *if err = lccc.deploy(stub, chainname, version, cds); err != nil {
		 *	return err
		 *}
		 **/

	_, err = lccc.createChaincode(stub, chainname, cds.ChaincodeSpec.ChaincodeID.Name, code)

	return err
}

//this implements "upgrade" Invoke transaction
func (lccc *LifeCycleSysCC) executeUpgrade(stub shim.ChaincodeStubInterface, chainName string, code []byte) ([]byte, error) {
	cds, err := lccc.getChaincodeDeploymentSpec(code)
	if err != nil {
		return nil, err
	}

	if err = lccc.acl(stub, chainName, cds); err != nil {
		return nil, err
	}

	chaincodeName := cds.ChaincodeSpec.ChaincodeID.Name
	if !lccc.isValidChaincodeName(chaincodeName) {
		return nil, InvalidChaincodeNameErr(chaincodeName)
	}

	// check for existence of chaincode
	cd, _, err := lccc.getChaincode(stub, chainName, chaincodeName)
	if cd == nil {
		return nil, NotFoundErr(chainName)
	}

	// if (ever) we allow users to specify a "deployed" version, this will have to change so
	// accept that. We might then require that users provide an "upgrade" version too. In
	// that case we'd replace the increment below with a uniqueness check. For now we will
	// assume Version is internal and is a number.
	//
	// NOTE - system chaincodes use the fabric's version and hence are not numbers.
	// As they cannot be "upgraded" via LCCC, that's not an issue. But they do help illustrate
	// the kind of issues we will have if we allow users to specify version.
	v, err := strconv.ParseInt(cd.Version, 10, 32)

	//This should never happen as long we don't expose version as version is computed internally
	//so panic till we find a need to relax
	if err != nil {
		panic(fmt.Sprintf("invalid version %s/%s [err: %s]", chaincodeName, chainName, err))
	}

	// replace the ChaincodeDeploymentSpec using the next version
	newVersion := fmt.Sprintf("%d", (v + 1))
	newCD, err := lccc.upgradeChaincode(stub, chainName, chaincodeName, newVersion, code)
	if err != nil {
		return nil, err
	}

	return []byte(newCD.Version), nil
}

//-------------- the chaincode stub interface implementation ----------

//Init does nothing
func (lccc *LifeCycleSysCC) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	return nil, nil
}

// Invoke implements lifecycle functions "deploy", "start", "stop", "upgrade".
// Deploy's arguments -  {[]byte("deploy"), []byte(<chainname>), <unmarshalled pb.ChaincodeDeploymentSpec>}
//
// Invoke also implements some query-like functions
// Get chaincode arguments -  {[]byte("getid"), []byte(<chainname>), []byte(<chaincodename>)}
func (lccc *LifeCycleSysCC) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	args := stub.GetArgs()
	if len(args) < 1 {
		return nil, InvalidArgsLenErr(len(args))
	}

	function := string(args[0])

	switch function {
	case DEPLOY:
		if len(args) != 3 {
			return nil, InvalidArgsLenErr(len(args))
		}

		//chain the chaincode shoud be associated with. It
		//should be created with a register call
		chainname := string(args[1])

		if !lccc.isValidChainName(chainname) {
			return nil, InvalidChainNameErr(chainname)
		}

		//bytes corresponding to deployment spec
		code := args[2]

		err := lccc.executeDeploy(stub, chainname, code)

		return nil, err
	case UPGRADE:
		if len(args) != 3 {
			return nil, InvalidArgsLenErr(len(args))
		}

		chainname := string(args[1])
		if !lccc.isValidChainName(chainname) {
			return nil, InvalidChainNameErr(chainname)
		}

		code := args[2]
		return lccc.executeUpgrade(stub, chainname, code)
	case GETCCINFO, GETDEPSPEC, GETCCDATA:
		if len(args) != 3 {
			return nil, InvalidArgsLenErr(len(args))
		}

		chain := string(args[1])
		ccname := string(args[2])
		//get chaincode given <chain, name>

		cd, cdbytes, _ := lccc.getChaincode(stub, chain, ccname)
		if cd == nil || cdbytes == nil {
			logger.Debug("ChaincodeID [%s/%s] does not exist", chain, ccname)
			return nil, TXNotFoundErr(ccname + "/" + chain)
		}

		if function == GETCCINFO {
			return []byte(cd.Name), nil
		} else if function == GETCCDATA {
			return cdbytes, nil
		}
		return cd.DepSpec, nil
	}

	return nil, InvalidFunctionErr(function)
}
