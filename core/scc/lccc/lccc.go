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

package lccc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
)

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
	// sccprovider is the interface with which we call
	// methods of the system chaincode package without
	// import cycles
	sccprovider sysccprovider.SystemChaincodeProvider
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
func (lccc *LifeCycleSysCC) createChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string, cccode []byte, policy []byte, escc []byte, vscc []byte) (*ccprovider.ChaincodeData, error) {
	return lccc.putChaincodeData(stub, chainname, ccname, startVersion, cccode, policy, escc, vscc)
}

//upgrade the chaincode on the given chain
func (lccc *LifeCycleSysCC) upgradeChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string, version string, cccode []byte, policy []byte, escc []byte, vscc []byte) (*ccprovider.ChaincodeData, error) {
	return lccc.putChaincodeData(stub, chainname, ccname, version, cccode, policy, escc, vscc)
}

//create the chaincode on the given chain
func (lccc *LifeCycleSysCC) putChaincodeData(stub shim.ChaincodeStubInterface, chainname string, ccname string, version string, cccode []byte, policy []byte, escc []byte, vscc []byte) (*ccprovider.ChaincodeData, error) {
	// check that escc and vscc are real system chaincodes
	if !lccc.sccprovider.IsSysCC(string(escc)) {
		return nil, fmt.Errorf("%s is not a valid endorsement system chaincode", string(escc))
	}
	if !lccc.sccprovider.IsSysCC(string(vscc)) {
		return nil, fmt.Errorf("%s is not a valid validation system chaincode", string(vscc))
	}

	cd := &ccprovider.ChaincodeData{Name: ccname, Version: version, DepSpec: cccode, Policy: policy, Escc: string(escc), Vscc: string(vscc)}
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
func (lccc *LifeCycleSysCC) getChaincode(stub shim.ChaincodeStubInterface, chainname string, ccname string) (*ccprovider.ChaincodeData, []byte, error) {
	cdbytes, err := stub.GetState(ccname)
	if err != nil {
		return nil, nil, err
	}

	if cdbytes != nil {
		cd := &ccprovider.ChaincodeData{}
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

//this implements "deploy" Invoke transaction
func (lccc *LifeCycleSysCC) executeDeploy(stub shim.ChaincodeStubInterface, chainname string, code []byte, policy []byte, escc []byte, vscc []byte) error {
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

	_, err = lccc.createChaincode(stub, chainname, cds.ChaincodeSpec.ChaincodeID.Name, code, policy, escc, vscc)

	return err
}

//this implements "upgrade" Invoke transaction
func (lccc *LifeCycleSysCC) executeUpgrade(stub shim.ChaincodeStubInterface, chainName string, code []byte, policy []byte, escc []byte, vscc []byte) ([]byte, error) {
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
	newCD, err := lccc.upgradeChaincode(stub, chainName, chaincodeName, newVersion, code, policy, escc, vscc)
	if err != nil {
		return nil, err
	}

	return []byte(newCD.Version), nil
}

//-------------- the chaincode stub interface implementation ----------

//Init only initializes the system chaincode provider
func (lccc *LifeCycleSysCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	lccc.sccprovider = sysccprovider.GetSystemChaincodeProvider()
	return shim.Success(nil)
}

// Invoke implements lifecycle functions "deploy", "start", "stop", "upgrade".
// Deploy's arguments -  {[]byte("deploy"), []byte(<chainname>), <unmarshalled pb.ChaincodeDeploymentSpec>}
//
// Invoke also implements some query-like functions
// Get chaincode arguments -  {[]byte("getid"), []byte(<chainname>), []byte(<chaincodename>)}
func (lccc *LifeCycleSysCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) < 1 {
		return shim.Error(InvalidArgsLenErr(len(args)).Error())
	}

	function := string(args[0])

	switch function {
	case DEPLOY:
		if len(args) < 3 || len(args) > 6 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		//chain the chaincode shoud be associated with. It
		//should be created with a register call
		chainname := string(args[1])

		if !lccc.isValidChainName(chainname) {
			return shim.Error(InvalidChainNameErr(chainname).Error())
		}

		//bytes corresponding to deployment spec
		code := args[2]

		// optional arguments here (they can each be nil and may or may not be present)
		// args[3] is a marshalled SignaturePolicyEnvelope representing the endorsement policy
		// args[4] is the name of escc
		// args[5] is the name of vscc
		var policy []byte
		if len(args) > 3 && args[3] != nil {
			policy = args[3]
		} else {
			// FIXME: temporary workaround until all SDKs provide a policy
			policy = utils.MarshalOrPanic(cauthdsl.SignedByMspMember("DEFAULT"))
		}

		var escc []byte
		if len(args) > 4 && args[4] != nil {
			escc = args[4]
		} else {
			escc = []byte("escc")
		}

		var vscc []byte
		if len(args) > 5 && args[5] != nil {
			vscc = args[5]
		} else {
			vscc = []byte("vscc")
		}

		err := lccc.executeDeploy(stub, chainname, code, policy, escc, vscc)
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success(nil)
	case UPGRADE:
		if len(args) < 3 || len(args) > 6 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		chainname := string(args[1])
		if !lccc.isValidChainName(chainname) {
			return shim.Error(InvalidChainNameErr(chainname).Error())
		}

		code := args[2]

		// optional arguments here (they can each be nil and may or may not be present)
		// args[3] is a marshalled SignaturePolicyEnvelope representing the endorsement policy
		// args[4] is the name of escc
		// args[5] is the name of vscc
		var policy []byte
		if len(args) > 3 && args[3] != nil {
			policy = args[3]
		} else {
			// FIXME: temporary workaround until all SDKs provide a policy
			policy = utils.MarshalOrPanic(cauthdsl.SignedByMspMember("DEFAULT"))
		}

		var escc []byte
		if len(args) > 4 && args[4] != nil {
			escc = args[4]
		} else {
			escc = []byte("escc")
		}

		var vscc []byte
		if len(args) > 5 && args[5] != nil {
			vscc = args[5]
		} else {
			vscc = []byte("vscc")
		}

		verBytes, err := lccc.executeUpgrade(stub, chainname, code, policy, escc, vscc)
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success(verBytes)
	case GETCCINFO, GETDEPSPEC, GETCCDATA:
		if len(args) != 3 {
			return shim.Error(InvalidArgsLenErr(len(args)).Error())
		}

		chain := string(args[1])
		ccname := string(args[2])
		//get chaincode given <chain, name>

		cd, cdbytes, _ := lccc.getChaincode(stub, chain, ccname)
		if cd == nil || cdbytes == nil {
			logger.Debugf("ChaincodeID: %s does not exist on channel: %s ", ccname, chain)
			return shim.Error(TXNotFoundErr(ccname + "/" + chain).Error())
		}

		if function == GETCCINFO {
			return shim.Success([]byte(cd.Name))
		} else if function == GETCCDATA {
			return shim.Success(cdbytes)
		}
		return shim.Success(cd.DepSpec)
	}

	return shim.Error(InvalidFunctionErr(function).Error())
}
